package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIssueStoreSeen(t *testing.T) {
	s := &issueStore{issues: make(map[int]*trackedIssue)}

	if s.Seen(1) {
		t.Error("expected issue 1 not seen")
	}

	s.Track(&trackedIssue{Number: 1, Title: "test", Status: "dispatched", DetectedAt: time.Now()})

	if !s.Seen(1) {
		t.Error("expected issue 1 seen after Track")
	}
	if s.Seen(2) {
		t.Error("expected issue 2 not seen")
	}
}

func TestIssueStoreList(t *testing.T) {
	s := &issueStore{issues: make(map[int]*trackedIssue)}
	s.Track(&trackedIssue{Number: 1, Title: "first", Status: "dispatched", DetectedAt: time.Now()})
	s.Track(&trackedIssue{Number: 2, Title: "second", Status: "error", DetectedAt: time.Now()})

	list := s.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(list))
	}
}

func TestIssueStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "issues_seen.json")

	// Create store and add issues
	s := newIssueStore(path)
	s.Track(&trackedIssue{Number: 10, Title: "persisted", Status: "dispatched", DetectedAt: time.Now()})

	// Verify file was written
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	// Load from file
	s2 := newIssueStore(path)
	if !s2.Seen(10) {
		t.Error("expected issue 10 to be persisted and loaded")
	}
	if s2.Seen(99) {
		t.Error("expected issue 99 not seen")
	}
}

func TestIsPullRequest(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://github.com/owner/repo/issues/1", false},
		{"https://github.com/owner/repo/pull/2", true},
		{"https://github.com/owner/repo/issues/3", false},
	}
	for _, tt := range tests {
		issue := ghIssue{URL: tt.url}
		if got := isPullRequest(issue); got != tt.want {
			t.Errorf("isPullRequest(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestReminderBackoff(t *testing.T) {
	tests := []struct {
		count int
		want  time.Duration
	}{
		{0, 5 * time.Minute},
		{1, 15 * time.Minute},
		{2, 30 * time.Minute},
		{3, 30 * time.Minute},
		{10, 30 * time.Minute},
	}
	for _, tt := range tests {
		got := reminderBackoff(tt.count)
		if got != tt.want {
			t.Errorf("reminderBackoff(%d) = %v, want %v", tt.count, got, tt.want)
		}
	}
}

func TestExtractIssueFromBranch(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"issue-123-fix-bug", "123"},
		{"fix-456-typo", "456"},
		{"feat-789-new-feature", "789"},
		{"main", ""},
		{"issue-abc-something", ""},
		{"random-branch", ""},
		{"issue-42", "42"},
	}
	for _, tt := range tests {
		got := extractIssueFromBranch(tt.branch)
		if got != tt.want {
			t.Errorf("extractIssueFromBranch(%q) = %q, want %q", tt.branch, got, tt.want)
		}
	}
}

func TestHasDalrootDelegation(t *testing.T) {
	tests := []struct {
		name     string
		comments []ghComment
		want     bool
	}{
		{
			name:     "no comments",
			comments: nil,
			want:     false,
		},
		{
			name: "comment with @dalroot mention",
			comments: []ghComment{
				{Body: "이 작업은 @dalroot에게 위임합니다"},
			},
			want: true,
		},
		{
			name: "comment with dalroot reference",
			comments: []ghComment{
				{Body: "dalroot가 처리해야 할 호스트 작업입니다"},
			},
			want: true,
		},
		{
			name: "unrelated comments",
			comments: []ghComment{
				{Body: "LGTM"},
				{Body: "좋은 수정입니다"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasDalrootDelegation(tt.comments)
			if got != tt.want {
				t.Errorf("hasDalrootDelegation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIssueStoreGet(t *testing.T) {
	s := &issueStore{issues: make(map[int]*trackedIssue)}

	if got := s.Get(1); got != nil {
		t.Error("expected nil for unknown issue")
	}

	tracked := &trackedIssue{Number: 1, Title: "test", Status: "dispatched", DetectedAt: time.Now()}
	s.Track(tracked)

	got := s.Get(1)
	if got == nil {
		t.Fatal("expected non-nil for tracked issue")
	}
	if got.Title != "test" {
		t.Errorf("expected title 'test', got %q", got.Title)
	}
}

func TestIssueStoreDelegated(t *testing.T) {
	s := &issueStore{issues: make(map[int]*trackedIssue)}

	now := time.Now().UTC()
	s.Track(&trackedIssue{Number: 1, Title: "delegated", Status: "dispatched", DelegatedAt: &now, DetectedAt: now})
	s.Track(&trackedIssue{Number: 2, Title: "not delegated", Status: "dispatched", DetectedAt: now})
	s.Track(&trackedIssue{Number: 3, Title: "closed delegated", Status: "closed", DelegatedAt: &now, DetectedAt: now})

	delegated := s.Delegated()
	if len(delegated) != 1 {
		t.Fatalf("expected 1 delegated issue, got %d", len(delegated))
	}
	if delegated[0].Number != 1 {
		t.Errorf("expected issue #1, got #%d", delegated[0].Number)
	}
}

func TestTrackedIssueDelegationFields(t *testing.T) {
	now := time.Now().UTC()
	tracked := &trackedIssue{
		Number:        42,
		Title:         "host config",
		Status:        "dispatched",
		DelegatedAt:   &now,
		DelegatedTo:   "dalroot",
		ReminderCount: 2,
	}

	if tracked.DelegatedTo != "dalroot" {
		t.Errorf("expected DelegatedTo=dalroot, got %q", tracked.DelegatedTo)
	}
	if tracked.ReminderCount != 2 {
		t.Errorf("expected ReminderCount=2, got %d", tracked.ReminderCount)
	}
	if tracked.DelegatedAt == nil || tracked.DelegatedAt.IsZero() {
		t.Error("expected DelegatedAt to be set")
	}
}
