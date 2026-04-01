package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultNotifyPollInterval = 2 * time.Minute
	reminderInitialDelay      = 5 * time.Minute
	reminderMaxDelay          = 30 * time.Minute
)

// dalrootPending tracks an action awaiting dalroot completion.
type dalrootPending struct {
	IssueNumber int       `json:"issue_number"`
	Title       string    `json:"title"`
	Message     string    `json:"message"`
	CreatedAt   time.Time `json:"created_at"`
	LastRemind  time.Time `json:"last_remind"`
	RemindCount int       `json:"remind_count"`
	Resolved    bool      `json:"resolved"`
}

// dalrootNotifier monitors issue state changes and sends notifications to dalroot.
type dalrootNotifier struct {
	mu       sync.Mutex
	pending  map[int]*dalrootPending
	filePath string
}

func newDalrootNotifier(path string) *dalrootNotifier {
	n := &dalrootNotifier{
		pending:  make(map[int]*dalrootPending),
		filePath: path,
	}
	var items []*dalrootPending
	if err := loadJSON(path, &items); err == nil {
		for _, p := range items {
			if !p.Resolved {
				n.pending[p.IssueNumber] = p
			}
		}
	}
	return n
}

func (n *dalrootNotifier) save() {
	n.mu.Lock()
	defer n.mu.Unlock()
	items := make([]*dalrootPending, 0, len(n.pending))
	for _, p := range n.pending {
		items = append(items, p)
	}
	persistJSON(n.filePath, items, nil)
}

// NotifyDalroot sends a message to dalroot via dalbridge or dalroot-tell.
func NotifyDalroot(msg string) error {
	// Try dalroot-tell first (host-level command)
	cmd := exec.Command("dalroot-tell", "dalroot", msg)
	if out, err := cmd.CombinedOutput(); err == nil {
		log.Printf("[dalroot-notifier] sent via dalroot-tell: %s", strings.TrimSpace(string(out)))
		return nil
	}

	// Fallback: write to notification file that dalroot-listener picks up
	notifDir := "/workspace/dalroot-notifications"
	cmd = exec.Command("bash", "-c", fmt.Sprintf(
		`mkdir -p %s && echo '%s' > %s/notify-%d.txt`,
		notifDir,
		strings.ReplaceAll(msg, "'", "'\\''"),
		notifDir,
		time.Now().UnixMilli(),
	))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("notification fallback failed: %s: %w", string(out), err)
	}

	log.Printf("[dalroot-notifier] sent via file: %s", msg)
	return nil
}

// startDalrootNotifier watches for issue state changes and dalroot pending items.
func (d *Daemon) startDalrootNotifier(ctx context.Context, repo string, interval time.Duration) {
	if repo == "" {
		return
	}
	if interval <= 0 {
		interval = defaultNotifyPollInterval
	}

	n := newDalrootNotifier(filepath.Join(dataDir(d.serviceRepo), "dalroot-pending.json"))
	d.dalrootNotifier = n

	log.Printf("[dalroot-notifier] started (interval=%s, repo=%s)", interval, repo)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[dalroot-notifier] stopped")
			return
		case <-ticker.C:
			d.checkClosedIssues(repo, n)
			d.checkReminders(n)
		}
	}
}

// checkClosedIssues polls recently closed issues and notifies dalroot.
func (d *Daemon) checkClosedIssues(repo string, n *dalrootNotifier) {
	cmd := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--state", "closed",
		"--limit", "10",
		"--json", "number,title,state,author,closedAt",
	)
	out, err := cmd.Output()
	if err != nil {
		return
	}

	var issues []struct {
		Number   int       `json:"number"`
		Title    string    `json:"title"`
		State    string    `json:"state"`
		Author   ghAuthor  `json:"author"`
		ClosedAt time.Time `json:"closedAt"`
	}
	if err := json.Unmarshal(out, &issues); err != nil {
		return
	}

	for _, issue := range issues {
		// Only notify for recently closed issues (within last poll interval * 2)
		if time.Since(issue.ClosedAt) > defaultNotifyPollInterval*2 {
			continue
		}

		// Check if already notified (use issue store from watcher)
		key := fmt.Sprintf("closed-%d", issue.Number)
		if d.issues.Seen(issue.Number) {
			tracked := d.issues.issues[issue.Number]
			if tracked != nil && tracked.Status == "closed-notified" {
				continue
			}
		}

		msg := fmt.Sprintf("[@dalroot] #%d closed: %s (by %s)",
			issue.Number, issue.Title, issue.Author.Login)

		if err := NotifyDalroot(msg); err != nil {
			log.Printf("[dalroot-notifier] failed to notify: %v", err)
			continue
		}

		// Mark as notified
		d.issues.Track(&trackedIssue{
			Number:     issue.Number,
			Title:      issue.Title,
			Author:     issue.Author.Login,
			DetectedAt: time.Now().UTC(),
			Status:     "closed-notified",
		})

		_ = key // suppress unused
		log.Printf("[dalroot-notifier] notified: #%d closed", issue.Number)
	}
}

// AddDalrootPending registers a pending action for dalroot to complete.
func (d *Daemon) AddDalrootPending(issueNumber int, title, message string) {
	if d.dalrootNotifier == nil {
		return
	}
	n := d.dalrootNotifier
	n.mu.Lock()
	defer n.mu.Unlock()
	n.pending[issueNumber] = &dalrootPending{
		IssueNumber: issueNumber,
		Title:       title,
		Message:     message,
		CreatedAt:   time.Now().UTC(),
		LastRemind:  time.Now().UTC(),
	}
	n.save()
}

// checkReminders sends reminders for pending dalroot actions with exponential backoff.
func (d *Daemon) checkReminders(n *dalrootNotifier) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for num, p := range n.pending {
		if p.Resolved {
			continue
		}

		// Calculate backoff delay: 5m, 15m, 30m, 30m, ...
		delay := reminderInitialDelay
		for i := 0; i < p.RemindCount && delay < reminderMaxDelay; i++ {
			delay = delay * 2
			if delay > reminderMaxDelay {
				delay = reminderMaxDelay
			}
		}

		if time.Since(p.LastRemind) < delay {
			continue
		}

		elapsed := time.Since(p.CreatedAt).Round(time.Minute)
		msg := fmt.Sprintf("[@dalroot] 리마인드: #%d %s (%s 경과)",
			num, p.Title, elapsed)

		if err := NotifyDalroot(msg); err != nil {
			log.Printf("[dalroot-notifier] reminder failed: %v", err)
			continue
		}

		p.RemindCount++
		p.LastRemind = time.Now().UTC()
		log.Printf("[dalroot-notifier] reminder #%d sent for issue #%d", p.RemindCount, num)
	}

	n.save()
}
