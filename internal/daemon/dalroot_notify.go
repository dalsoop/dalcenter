package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// dalrootReminder tracks pending dalroot-delegated issues with backoff.
type dalrootReminder struct {
	IssueNumber int       `json:"issue_number"`
	Title       string    `json:"title"`
	DalName     string    `json:"dal_name"`
	CreatedAt   time.Time `json:"created_at"`
	LastNotify  time.Time `json:"last_notify"`
	NotifyCount int       `json:"notify_count"`
}

// dalrootReminderStore manages pending reminders.
type dalrootReminderStore struct {
	mu        sync.RWMutex
	reminders map[int]*dalrootReminder // issue number → reminder
}

func newDalrootReminderStore() *dalrootReminderStore {
	return &dalrootReminderStore{reminders: make(map[int]*dalrootReminder)}
}

func (s *dalrootReminderStore) Add(issueNum int, title, dalName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.reminders[issueNum]; exists {
		return
	}
	now := time.Now().UTC()
	s.reminders[issueNum] = &dalrootReminder{
		IssueNumber: issueNum,
		Title:       title,
		DalName:     dalName,
		CreatedAt:   now,
		LastNotify:  now,
	}
}

func (s *dalrootReminderStore) Remove(issueNum int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.reminders, issueNum)
}

func (s *dalrootReminderStore) List() []*dalrootReminder {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*dalrootReminder, 0, len(s.reminders))
	for _, r := range s.reminders {
		result = append(result, r)
	}
	return result
}

// reminderBackoff returns the interval for the next reminder based on count.
// Backoff schedule: 5m → 15m → 30m (capped).
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

// notifyDalroot sends a formatted notification to dalroot via mmPost or bridge.
func (d *Daemon) notifyDalroot(msg string) {
	log.Printf("[dalroot-notify] %s", msg)

	// Try mmPost first (direct Mattermost API)
	if err := d.mmPost(msg); err == nil {
		return
	}

	// Fallback to bridge
	if d.bridgeURL != "" {
		if err := d.bridgePost(msg, "dalcenter"); err != nil {
			log.Printf("[dalroot-notify] bridge post failed: %v", err)
		}
		return
	}

	log.Printf("[dalroot-notify] no notification channel available")
}

// pollClosedIssues fetches recently closed issues and detects OPEN→CLOSED transitions.
func (d *Daemon) pollClosedIssues(repo string) {
	issues, err := fetchClosedIssues(repo)
	if err != nil {
		log.Printf("[issue-watcher] fetch closed failed: %v", err)
		return
	}

	for _, issue := range issues {
		if isPullRequest(issue) {
			continue
		}
		tracked := d.issues.Get(issue.Number)
		if tracked == nil {
			// Not previously tracked — record as closed
			tracked = &trackedIssue{
				Number:     issue.Number,
				Title:      issue.Title,
				URL:        issue.URL,
				Author:     issue.Author.Login,
				DetectedAt: time.Now().UTC(),
				Status:     "skipped",
				LastState:  "CLOSED",
			}
			d.issues.Track(tracked)
			continue
		}
		if tracked.LastState == "OPEN" {
			tracked.LastState = "CLOSED"
			d.issues.Track(tracked)
			d.notifyDalroot(fmt.Sprintf("[@dalroot] #%d close: %s (by %s)", issue.Number, issue.Title, issue.Author.Login))
			d.reminders.Remove(issue.Number)
			log.Printf("[issue-watcher] state change: #%d OPEN→CLOSED", issue.Number)
		}
	}
}

// fetchClosedIssues calls `gh issue list --state closed` for recently closed issues.
func fetchClosedIssues(repo string) ([]ghIssue, error) {
	cmd := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--state", "closed",
		"--limit", "20",
		"--json", "number,title,body,state,labels,createdAt,author,url",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh issue list closed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh issue list closed: %w", err)
	}

	var issues []ghIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse closed issues: %w", err)
	}
	return issues, nil
}

// ghPR represents a GitHub pull request from `gh pr list --json`.
type ghPR struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Author    ghAuthor `json:"author"`
	MergedAt  string   `json:"mergedAt"`
	ClosingIssues []struct {
		Number int `json:"number"`
	} `json:"closingIssuesReferences"`
}

// pollMergedPRs detects merged PRs with linked issues and notifies dalroot.
func (d *Daemon) pollMergedPRs(repo string) {
	prs, err := fetchMergedPRs(repo)
	if err != nil {
		log.Printf("[issue-watcher] fetch merged PRs failed: %v", err)
		return
	}

	for _, pr := range prs {
		// Use PR number as a negative key to avoid collision with issues
		seenKey := -pr.Number
		if d.issues.Seen(seenKey) {
			continue
		}

		// Track as seen
		d.issues.Track(&trackedIssue{
			Number:     seenKey,
			Title:      fmt.Sprintf("PR#%d: %s", pr.Number, pr.Title),
			URL:        pr.URL,
			Author:     pr.Author.Login,
			DetectedAt: time.Now().UTC(),
			Status:     "dispatched",
			LastState:  "MERGED",
		})

		// Notify for each linked issue
		for _, linked := range pr.ClosingIssues {
			d.notifyDalroot(fmt.Sprintf("[@dalroot] #%d 완료: %s — PR #%d merged (by %s)",
				linked.Number, pr.Title, pr.Number, pr.Author.Login))
			d.reminders.Remove(linked.Number)
		}

		// If no linked issues, still notify about the merge
		if len(pr.ClosingIssues) == 0 {
			d.notifyDalroot(fmt.Sprintf("[@dalroot] PR #%d merged: %s (by %s)",
				pr.Number, pr.Title, pr.Author.Login))
		}
	}
}

// fetchMergedPRs calls `gh pr list --state merged` for recently merged PRs.
func fetchMergedPRs(repo string) ([]ghPR, error) {
	cmd := exec.Command("gh", "pr", "list",
		"--repo", repo,
		"--state", "merged",
		"--limit", "10",
		"--json", "number,title,url,author,mergedAt,closingIssuesReferences",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh pr list merged: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh pr list merged: %w", err)
	}

	var prs []ghPR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse merged PRs: %w", err)
	}
	return prs, nil
}

// ghComment represents a GitHub issue comment.
type ghComment struct {
	Body      string   `json:"body"`
	Author    ghAuthor `json:"author"`
	CreatedAt string   `json:"createdAt"`
}

// lastMentionCheck tracks the last time we checked mentions for each issue.
var lastMentionCheck = struct {
	mu    sync.RWMutex
	times map[int]time.Time
}{times: make(map[int]time.Time)}

// pollDalrootMentions scans comments on tracked open issues for @dalroot mentions.
func (d *Daemon) pollDalrootMentions(repo string) {
	tracked := d.issues.List()
	for _, t := range tracked {
		if t.LastState != "OPEN" {
			continue
		}
		if t.Number <= 0 {
			continue // skip PR entries (negative numbers)
		}

		lastMentionCheck.mu.RLock()
		lastCheck := lastMentionCheck.times[t.Number]
		lastMentionCheck.mu.RUnlock()

		comments, err := fetchIssueComments(repo, t.Number)
		if err != nil {
			log.Printf("[issue-watcher] fetch comments #%d failed: %v", t.Number, err)
			continue
		}

		for _, c := range comments {
			if !strings.Contains(c.Body, "@dalroot") {
				continue
			}
			commentTime, err := time.Parse(time.RFC3339, c.CreatedAt)
			if err != nil {
				continue
			}
			if !lastCheck.IsZero() && !commentTime.After(lastCheck) {
				continue
			}
			body := truncateStr(c.Body, 200)
			d.notifyDalroot(fmt.Sprintf("[@dalroot] #%d 리마인드: %s — %s (by %s)",
				t.Number, t.Title, body, c.Author.Login))
		}

		lastMentionCheck.mu.Lock()
		lastMentionCheck.times[t.Number] = time.Now().UTC()
		lastMentionCheck.mu.Unlock()
	}
}

// fetchIssueComments retrieves comments for a specific issue.
func fetchIssueComments(repo string, issueNum int) ([]ghComment, error) {
	cmd := exec.Command("gh", "issue", "view",
		fmt.Sprintf("%d", issueNum),
		"--repo", repo,
		"--json", "comments",
	)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh issue view comments: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh issue view comments: %w", err)
	}

	var result struct {
		Comments []ghComment `json:"comments"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse comments: %w", err)
	}
	return result.Comments, nil
}

// startDalrootReminder runs a goroutine that periodically checks for pending
// dalroot-delegated issues and sends reminders with backoff (5m → 15m → 30m).
func (d *Daemon) startDalrootReminder(ctx context.Context) {
	log.Printf("[dalroot-reminder] started")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[dalroot-reminder] stopped")
			return
		case <-ticker.C:
			d.checkReminders()
		}
	}
}

// checkReminders sends reminders for overdue dalroot-delegated issues.
func (d *Daemon) checkReminders() {
	now := time.Now().UTC()
	for _, r := range d.reminders.List() {
		interval := reminderBackoff(r.NotifyCount)
		if now.Sub(r.LastNotify) < interval {
			continue
		}

		d.notifyDalroot(fmt.Sprintf("[@dalroot] #%d 리마인드: %s 미처리 (위임: %s, %s 경과)",
			r.IssueNumber, r.Title, r.DalName,
			now.Sub(r.CreatedAt).Truncate(time.Minute)))

		d.reminders.mu.Lock()
		if rem, ok := d.reminders.reminders[r.IssueNumber]; ok {
			rem.NotifyCount++
			rem.LastNotify = now
		}
		d.reminders.mu.Unlock()
	}
}
