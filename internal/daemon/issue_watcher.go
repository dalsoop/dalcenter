package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	defaultIssuePollInterval = 5 * time.Minute
	maxTrackedIssues         = 200
)

// ghIssue represents a GitHub issue from `gh issue list --json`.
type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Labels    []ghLabel `json:"labels"`
	CreatedAt time.Time `json:"createdAt"`
	Author    ghAuthor  `json:"author"`
	URL       string    `json:"url"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghAuthor struct {
	Login string `json:"login"`
}

// trackedIssue records a dispatched issue and its processing state.
type trackedIssue struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	Author     string    `json:"author"`
	Labels     []string  `json:"labels,omitempty"`
	DetectedAt time.Time `json:"detected_at"`
	TaskID     string    `json:"task_id,omitempty"` // task ID dispatched to leader
	Status     string    `json:"status"`            // "dispatched", "skipped", "error", "closed"
	Error      string    `json:"error,omitempty"`

	// Delegation tracking for dalroot reminders
	DelegatedAt    *time.Time `json:"delegated_at,omitempty"`
	DelegatedTo    string     `json:"delegated_to,omitempty"`
	ReminderCount  int        `json:"reminder_count,omitempty"`
	LastRemindedAt *time.Time `json:"last_reminded_at,omitempty"`
}

// ghPR represents a GitHub pull request from `gh pr list --json`.
type ghPR struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	MergedAt  time.Time `json:"mergedAt"`
	URL       string    `json:"url"`
	Author    ghAuthor  `json:"author"`
	HeadRef   string    `json:"headRefName"`
}

// ghComment represents a GitHub issue comment.
type ghComment struct {
	Body      string   `json:"body"`
	Author    ghAuthor `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

// reminderBackoff returns the next reminder interval based on reminder count.
// Schedule: 5m → 15m → 30m (capped).
func reminderBackoff(count int) time.Duration {
	switch {
	case count <= 0:
		return 5 * time.Minute
	case count == 1:
		return 15 * time.Minute
	default:
		return 30 * time.Minute
	}
}

// issueStore tracks which issues have been seen and dispatched.
type issueStore struct {
	mu       sync.RWMutex
	issues   map[int]*trackedIssue // issue number -> tracked issue
	filePath string
}

func newIssueStore(path string) *issueStore {
	s := &issueStore{issues: make(map[int]*trackedIssue), filePath: path}
	var items []*trackedIssue
	if err := loadJSON(path, &items); err == nil {
		for _, issue := range items {
			s.issues[issue.Number] = issue
		}
	}
	return s
}

func (s *issueStore) Seen(number int) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.issues[number]
	return ok
}

func (s *issueStore) Track(issue *trackedIssue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.issues[issue.Number] = issue
	s.save()
}

func (s *issueStore) List() []*trackedIssue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*trackedIssue, 0, len(s.issues))
	for _, issue := range s.issues {
		result = append(result, issue)
	}
	return result
}

func (s *issueStore) save() {
	if s.filePath == "" {
		return
	}
	items := make([]*trackedIssue, 0, len(s.issues))
	for _, issue := range s.issues {
		items = append(items, issue)
	}
	// Evict oldest if over limit
	if len(items) > maxTrackedIssues {
		oldest := items[0]
		for _, item := range items[1:] {
			if item.DetectedAt.Before(oldest.DetectedAt) {
				oldest = item
			}
		}
		delete(s.issues, oldest.Number)
		items = make([]*trackedIssue, 0, len(s.issues))
		for _, issue := range s.issues {
			items = append(items, issue)
		}
	}
	persistJSON(s.filePath, items, nil)
}

// startIssueWatcher periodically polls GitHub issues and dispatches new ones to the leader.
func (d *Daemon) startIssueWatcher(ctx context.Context, repo string, interval time.Duration) {
	if repo == "" {
		log.Printf("[issue-watcher] DALCENTER_GITHUB_REPO not set, skipping")
		return
	}

	// Verify gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		log.Printf("[issue-watcher] gh CLI not found, skipping: %v", err)
		return
	}

	if interval <= 0 {
		interval = defaultIssuePollInterval
	}

	log.Printf("[issue-watcher] started (interval=%s, repo=%s)", interval, repo)

	// Initial poll after short delay to let daemon finish startup
	initialDelay := 30 * time.Second
	select {
	case <-ctx.Done():
		return
	case <-time.After(initialDelay):
	}

	d.pollGitHubIssues(repo)
	d.pollIssueCloses(repo)
	d.pollMergedPRs(repo)
	d.pollDalrootDelegations(repo)
	d.pollDalrootReminders()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[issue-watcher] stopped")
			return
		case <-ticker.C:
			d.pollGitHubIssues(repo)
			d.pollIssueCloses(repo)
			d.pollMergedPRs(repo)
			d.pollDalrootDelegations(repo)
			d.pollDalrootReminders()
		}
	}
}

// pollGitHubIssues fetches open issues and dispatches new ones to the leader.
func (d *Daemon) pollGitHubIssues(repo string) {
	issues, err := fetchGitHubIssues(repo)
	if err != nil {
		log.Printf("[issue-watcher] fetch failed: %v", err)
		return
	}

	var newCount int
	for _, issue := range issues {
		if d.issues.Seen(issue.Number) {
			continue
		}

		// Skip pull requests (gh issue list may include them)
		if isPullRequest(issue) {
			continue
		}

		newCount++
		tracked := &trackedIssue{
			Number:     issue.Number,
			Title:      issue.Title,
			URL:        issue.URL,
			Author:     issue.Author.Login,
			DetectedAt: time.Now().UTC(),
		}
		for _, l := range issue.Labels {
			tracked.Labels = append(tracked.Labels, l.Name)
		}

		// Dispatch to leader
		taskID, err := d.dispatchIssueToLeader(issue)
		if err != nil {
			log.Printf("[issue-watcher] dispatch #%d failed: %v", issue.Number, err)
			tracked.Status = "error"
			tracked.Error = err.Error()
		} else {
			tracked.Status = "dispatched"
			tracked.TaskID = taskID
			log.Printf("[issue-watcher] dispatched #%d → leader (task=%s)", issue.Number, taskID)
		}

		d.issues.Track(tracked)
	}

	if newCount > 0 {
		log.Printf("[issue-watcher] poll: %d new issues dispatched", newCount)
	}
}

// fetchGitHubIssues calls `gh issue list` to get open issues.
func fetchGitHubIssues(repo string) ([]ghIssue, error) {
	cmd := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--state", "open",
		"--limit", "30",
		"--json", "number,title,body,state,labels,createdAt,author,url",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh issue list: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh issue list: %w", err)
	}

	var issues []ghIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse issues: %w", err)
	}
	return issues, nil
}

// isPullRequest checks if a gh issue is actually a PR (gh may include PRs).
func isPullRequest(issue ghIssue) bool {
	return strings.Contains(issue.URL, "/pull/")
}

// dispatchIssueToLeader sends a task to the running leader container.
func (d *Daemon) dispatchIssueToLeader(issue ghIssue) (string, error) {
	// Find leader container
	d.mu.RLock()
	var leader *Container
	for _, c := range d.containers {
		if c.Role == "leader" && c.Status == "running" {
			leader = c
			break
		}
	}
	d.mu.RUnlock()

	if leader == nil {
		return "", fmt.Errorf("no running leader container")
	}

	// Build task prompt for leader
	labels := make([]string, 0, len(issue.Labels))
	for _, l := range issue.Labels {
		labels = append(labels, l.Name)
	}

	body := issue.Body
	if len(body) > 2000 {
		body = body[:2000] + "\n...(truncated)"
	}

	prompt := fmt.Sprintf(`새 GitHub 이슈가 등록되었습니다. 분석하고 적절한 member에게 작업을 할당하세요.

## Issue #%d: %s
- Author: %s
- Labels: %s
- URL: %s

### Body
%s

## 지시사항
1. 이슈를 분석하여 작업 범위를 파악하세요
2. 적절한 member dal에게 assign하세요 (dalcli wake <member> --issue %d)
3. member가 깨어나면 dalcli assign <member> "이슈 #%d 작업 지시 내용"으로 작업을 전달하세요
4. 작업 완료 후 PR이 올라오면 dalroot에게 알려주세요`,
		issue.Number, issue.Title,
		issue.Author.Login,
		strings.Join(labels, ", "),
		issue.URL,
		body,
		issue.Number, issue.Number,
	)

	// Dispatch as async task to leader
	tr := d.tasks.New(leader.DalName, prompt)
	go d.execTaskInContainer(leader, tr)

	// Dispatch webhook notification
	dispatchWebhook(WebhookEvent{
		Event:     "issue_detected",
		Dal:       leader.DalName,
		Task:      fmt.Sprintf("Issue #%d: %s", issue.Number, issue.Title),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})

	return tr.ID, nil
}

// Get returns a tracked issue by number, or nil if not found.
func (s *issueStore) Get(number int) *trackedIssue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.issues[number]
}

// Delegated returns all tracked issues that have been delegated but not yet resolved.
func (s *issueStore) Delegated() []*trackedIssue {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*trackedIssue
	for _, issue := range s.issues {
		if issue.DelegatedAt != nil && issue.Status == "dispatched" {
			result = append(result, issue)
		}
	}
	return result
}

// handleIssues returns tracked GitHub issues.
// GET /api/issues
func (d *Daemon) handleIssues(w http.ResponseWriter, r *http.Request) {
	issues := d.issues.List()
	respondJSON(w, http.StatusOK, map[string]any{
		"issues": issues,
		"total":  len(issues),
	})
}

// fetchClosedGitHubIssues calls `gh issue list --state closed` to get recently closed issues.
func fetchClosedGitHubIssues(repo string, limit int) ([]ghIssue, error) {
	if limit <= 0 {
		limit = 30
	}
	cmd := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--state", "closed",
		"--limit", fmt.Sprintf("%d", limit),
		"--json", "number,title,body,state,labels,createdAt,author,url",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh issue list --state closed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh issue list --state closed: %w", err)
	}
	var issues []ghIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse closed issues: %w", err)
	}
	return issues, nil
}

// fetchMergedPRs calls `gh pr list --state merged` to get recently merged PRs.
func fetchMergedPRs(repo string, limit int) ([]ghPR, error) {
	if limit <= 0 {
		limit = 10
	}
	cmd := exec.Command("gh", "pr", "list",
		"--repo", repo,
		"--state", "merged",
		"--limit", fmt.Sprintf("%d", limit),
		"--json", "number,title,state,mergedAt,url,author,headRefName",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh pr list --state merged: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh pr list --state merged: %w", err)
	}
	var prs []ghPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse merged PRs: %w", err)
	}
	return prs, nil
}

// fetchIssueComments calls `gh issue view` to get comments on an issue.
func fetchIssueComments(repo string, issueNumber int) ([]ghComment, error) {
	cmd := exec.Command("gh", "issue", "view",
		fmt.Sprintf("%d", issueNumber),
		"--repo", repo,
		"--json", "comments",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh issue view #%d comments: %s", issueNumber, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh issue view #%d comments: %w", issueNumber, err)
	}
	var result struct {
		Comments []ghComment `json:"comments"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse issue #%d comments: %w", issueNumber, err)
	}
	return result.Comments, nil
}

// hasDalrootDelegation checks if any comment delegates to dalroot.
func hasDalrootDelegation(comments []ghComment) bool {
	for _, c := range comments {
		lower := strings.ToLower(c.Body)
		if strings.Contains(lower, "@dalroot") || strings.Contains(lower, "dalroot") {
			return true
		}
	}
	return false
}

// notifyDalroot sends a notification to dalroot via bridge with @dalroot mention.
func (d *Daemon) notifyDalroot(msg string) {
	fullMsg := "@dalroot " + msg
	if d.bridgeURL != "" {
		if err := d.bridgePost(fullMsg, "dalcenter"); err != nil {
			log.Printf("[issue-watcher] dalroot bridge notify failed: %v", err)
		}
	}

	dispatchWebhook(WebhookEvent{
		Event:     "dalroot_notify",
		Dal:       "dalcenter",
		Task:      msg,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// pollIssueCloses detects issues that were open but are now closed.
func (d *Daemon) pollIssueCloses(repo string) {
	closed, err := fetchClosedGitHubIssues(repo, 30)
	if err != nil {
		log.Printf("[issue-watcher] fetch closed issues failed: %v", err)
		return
	}

	for _, issue := range closed {
		tracked := d.issues.Get(issue.Number)
		if tracked == nil || tracked.Status == "closed" {
			continue
		}
		// Issue was tracked and is now closed
		tracked.Status = "closed"
		d.issues.Track(tracked)

		msg := fmt.Sprintf("[@dalroot] #%d close: %s (by %s)", issue.Number, issue.Title, issue.Author.Login)
		d.notifyDalroot(msg)
		log.Printf("[issue-watcher] issue #%d closed, notified dalroot", issue.Number)
	}
}

// pollMergedPRs detects recently merged PRs and notifies dalroot.
func (d *Daemon) pollMergedPRs(repo string) {
	prs, err := fetchMergedPRs(repo, 10)
	if err != nil {
		log.Printf("[issue-watcher] fetch merged PRs failed: %v", err)
		return
	}

	for _, pr := range prs {
		// Use negative numbers for PR tracking to avoid collision with issue numbers
		trackKey := -pr.Number
		if d.issues.Seen(trackKey) {
			continue
		}

		// Only notify for PRs merged within the last poll interval (+ buffer)
		if time.Since(pr.MergedAt) > 2*defaultIssuePollInterval {
			continue
		}

		// Track this PR so we don't notify again
		d.issues.Track(&trackedIssue{
			Number:     trackKey,
			Title:      pr.Title,
			URL:        pr.URL,
			Author:     pr.Author.Login,
			DetectedAt: time.Now().UTC(),
			Status:     "closed",
		})

		// Extract issue number from branch name (e.g., "issue-123-xxx")
		issueRef := extractIssueFromBranch(pr.HeadRef)

		var msg string
		if issueRef != "" {
			msg = fmt.Sprintf("[@dalroot] #%s close: %s (PR #%d merged)", issueRef, pr.Title, pr.Number)
		} else {
			msg = fmt.Sprintf("[@dalroot] PR #%d merged: %s (by %s)", pr.Number, pr.Title, pr.Author.Login)
		}
		d.notifyDalroot(msg)
		log.Printf("[issue-watcher] PR #%d merged, notified dalroot", pr.Number)
	}
}

// extractIssueFromBranch extracts an issue number from a branch name like "issue-123-description".
func extractIssueFromBranch(branch string) string {
	parts := strings.Split(branch, "-")
	if len(parts) >= 2 && (parts[0] == "issue" || parts[0] == "fix" || parts[0] == "feat") {
		// Check if second part is a number
		for _, c := range parts[1] {
			if c < '0' || c > '9' {
				return ""
			}
		}
		return parts[1]
	}
	return ""
}

// pollDalrootDelegations checks tracked open issues for comments delegating to dalroot.
func (d *Daemon) pollDalrootDelegations(repo string) {
	for _, tracked := range d.issues.List() {
		if tracked.Status != "dispatched" || tracked.DelegatedAt != nil {
			continue
		}

		comments, err := fetchIssueComments(repo, tracked.Number)
		if err != nil {
			log.Printf("[issue-watcher] fetch comments for #%d failed: %v", tracked.Number, err)
			continue
		}

		if hasDalrootDelegation(comments) {
			now := time.Now().UTC()
			tracked.DelegatedAt = &now
			tracked.DelegatedTo = "dalroot"
			d.issues.Track(tracked)

			msg := fmt.Sprintf("[@dalroot] #%d 완료: %s (by %s)", tracked.Number, tracked.Title, tracked.Author)
			d.notifyDalroot(msg)
			log.Printf("[issue-watcher] issue #%d delegated to dalroot, notified", tracked.Number)
		}
	}
}

// pollDalrootReminders checks delegated issues and sends backoff reminders.
func (d *Daemon) pollDalrootReminders() {
	now := time.Now().UTC()
	for _, tracked := range d.issues.Delegated() {
		if tracked.DelegatedAt == nil {
			continue
		}

		interval := reminderBackoff(tracked.ReminderCount)
		var lastNotify time.Time
		if tracked.LastRemindedAt != nil {
			lastNotify = *tracked.LastRemindedAt
		} else {
			lastNotify = *tracked.DelegatedAt
		}

		if now.Sub(lastNotify) < interval {
			continue
		}

		elapsed := now.Sub(*tracked.DelegatedAt).Truncate(time.Minute)
		msg := fmt.Sprintf("[@dalroot] 리마인드: #%d 호스트 작업 미처리 (%s 경과)", tracked.Number, elapsed)
		d.notifyDalroot(msg)

		tracked.ReminderCount++
		tracked.LastRemindedAt = &now
		d.issues.Track(tracked)
		log.Printf("[issue-watcher] reminder #%d for issue #%d (%s elapsed)", tracked.ReminderCount, tracked.Number, elapsed)
	}
}
