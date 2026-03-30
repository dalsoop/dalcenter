package daemon

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

const defaultIssuePollInterval = 5 * time.Minute

// issueStore tracks seen GitHub issues to avoid duplicate dispatch.
type issueStore struct {
	mu   sync.Mutex
	seen map[int]time.Time
	path string
}

func newIssueStore(path string) *issueStore {
	s := &issueStore{
		seen: make(map[int]time.Time),
		path: path,
	}
	s.load()
	return s
}

func (s *issueStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	json.Unmarshal(data, &s.seen)
}

func (s *issueStore) save() {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(s.seen)
	if err != nil {
		return
	}
	os.WriteFile(s.path, data, 0644)
}

func (s *issueStore) hasSeen(num int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.seen[num]
	return ok
}

func (s *issueStore) markSeen(num int) {
	s.mu.Lock()
	s.seen[num] = time.Now()
	s.mu.Unlock()
	s.save()
}

func (s *issueStore) list() map[int]time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make(map[int]time.Time, len(s.seen))
	for k, v := range s.seen {
		cp[k] = v
	}
	return cp
}

// startIssueWatcher polls GitHub for new issues and dispatches them to the leader.
// This is a stub — full implementation will arrive with #526.
func (d *Daemon) startIssueWatcher(ctx context.Context, repo string, interval time.Duration) {
	if repo == "" {
		log.Println("[issue-watcher] disabled (no --github-repo configured)")
		return
	}
	log.Printf("[issue-watcher] watching %s (interval=%s)", repo, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Stub: poll loop placeholder for #526.
		}
	}
}

// handleIssues returns the list of tracked issues.
func (d *Daemon) handleIssues(w http.ResponseWriter, r *http.Request) {
	seen := d.issues.list()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(seen)
}
