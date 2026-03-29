package daemon

import "testing"

func TestFeedbackStore_AddUsesUUIDStyleID(t *testing.T) {
	s := newFeedbackStore()
	fb := s.Add("dev", "task-123", "triage issue", "success", "", 0, 42)
	if !prefixedUUIDPattern.MatchString(fb.ID) {
		t.Fatalf("expected feedback UUID-style id, got %s", fb.ID)
	}
	if fb.TaskID != "task-123" {
		t.Fatalf("task id = %q, want task-123", fb.TaskID)
	}
}
