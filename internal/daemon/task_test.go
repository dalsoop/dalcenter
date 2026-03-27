package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFakeDocker(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "docker")
	script := `#!/bin/sh
stdin=$(cat)
{
  printf 'argv:'
  for arg in "$@"; do
    printf '[%s]' "$arg"
  done
  printf '\nstdin:%s\n--\n' "$stdin"
} >> "$DAL_TEST_CAPTURE"
case "$*" in
  *"git diff --stat HEAD"*)
    exit 0
    ;;
esac
printf '%s' "${DAL_TEST_STDOUT:-task ok}"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	return dir
}

func TestTaskStore_New(t *testing.T) {
	s := newTaskStore()
	tr := s.New("dev", "go test ./...")
	if tr.ID != "task-0001" {
		t.Errorf("expected task-0001, got %s", tr.ID)
	}
	if tr.Dal != "dev" {
		t.Errorf("expected dal=dev, got %s", tr.Dal)
	}
	if tr.Status != "running" {
		t.Errorf("expected status=running, got %s", tr.Status)
	}
}

func TestTaskStore_Get(t *testing.T) {
	s := newTaskStore()
	tr := s.New("dev", "go test ./...")
	got := s.Get(tr.ID)
	if got == nil {
		t.Fatal("expected task, got nil")
	}
	if got.Task != "go test ./..." {
		t.Errorf("expected 'go test ./...', got %q", got.Task)
	}
}

func TestTaskStore_GetMissing(t *testing.T) {
	s := newTaskStore()
	got := s.Get("task-9999")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestTaskStore_List(t *testing.T) {
	s := newTaskStore()
	s.New("dev", "task1")
	s.New("leader", "task2")
	list := s.List()
	if len(list) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(list))
	}
}

func TestTaskStore_Eviction(t *testing.T) {
	s := newTaskStore()
	for i := 0; i < 55; i++ {
		tr := s.New("dev", "task")
		tr.Status = "done" // mark as completed so it can be evicted
	}
	list := s.List()
	if len(list) > 51 {
		t.Errorf("expected <=51 tasks after eviction, got %d", len(list))
	}
}

func TestHandleTask_NoDal(t *testing.T) {
	d := New(":0", "/tmp/test", t.TempDir(), nil)
	body := `{"dal":"nonexistent","task":"hello"}`
	req := httptest.NewRequest("POST", "/api/task", strings.NewReader(body))
	w := httptest.NewRecorder()
	d.handleTask(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleTask_MissingFields(t *testing.T) {
	d := New(":0", "/tmp/test", t.TempDir(), nil)
	body := `{"dal":"","task":""}`
	req := httptest.NewRequest("POST", "/api/task", strings.NewReader(body))
	w := httptest.NewRecorder()
	d.handleTask(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTaskList_Empty(t *testing.T) {
	d := New(":0", "/tmp/test", t.TempDir(), nil)
	req := httptest.NewRequest("GET", "/api/tasks", nil)
	w := httptest.NewRecorder()
	d.handleTaskList(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result []*taskResult
	json.NewDecoder(w.Body).Decode(&result)
	// nil or empty is fine
}

func TestHandleTaskStatus_NotFound(t *testing.T) {
	d := New(":0", "/tmp/test", t.TempDir(), nil)
	req := httptest.NewRequest("GET", "/api/task/task-9999", nil)
	req.SetPathValue("id", "task-9999")
	w := httptest.NewRecorder()
	d.handleTaskStatus(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestTruncateStr(t *testing.T) {
	if truncateStr("hello", 10) != "hello" {
		t.Error("should not truncate short string")
	}
	if truncateStr("hello world", 5) != "hello..." {
		t.Errorf("got %q", truncateStr("hello world", 5))
	}
}

func TestVerifyTaskChanges_NoContainer(t *testing.T) {
	tr := &taskResult{ID: "test-001", Status: "done"}
	verifyTaskChanges("nonexistent-container-id", tr)
	if tr.Verified != "skipped" {
		t.Errorf("expected skipped for invalid container, got %q", tr.Verified)
	}
}

func TestTaskResult_VerifiedFields(t *testing.T) {
	tr := &taskResult{
		ID:         "test-002",
		Verified:   "no_changes",
		GitChanges: 0,
	}
	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	// Verify JSON serialization includes verified field
	if !strings.Contains(string(data), `"verified":"no_changes"`) {
		t.Errorf("JSON should contain verified field: %s", data)
	}
	// git_diff should be omitted when empty
	if strings.Contains(string(data), `"git_diff"`) {
		t.Errorf("git_diff should be omitted when empty: %s", data)
	}
}

func TestTaskResult_WithChanges(t *testing.T) {
	tr := &taskResult{
		ID:         "test-003",
		Verified:   "yes",
		GitDiff:    "M  README.md\nA  new-file.go",
		GitChanges: 2,
	}
	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"git_changes":2`) {
		t.Errorf("expected git_changes:2 in JSON: %s", data)
	}
	if !strings.Contains(string(data), `"verified":"yes"`) {
		t.Errorf("expected verified:yes in JSON: %s", data)
	}
}

func TestMessageFallback_NoMM(t *testing.T) {
	d := New(":0", "/tmp/test", t.TempDir(), nil)
	// No MM configured, no running dals → should return 503
	body := `{"from":"host","message":"test"}`
	req := httptest.NewRequest("POST", "/api/message", strings.NewReader(body))
	w := httptest.NewRecorder()
	d.handleMessage(w, req)
	if w.Code != 503 {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestExecTaskInContainer_ClaudeDisablesSessionPersistence(t *testing.T) {
	fakeDir := writeFakeDocker(t)
	capture := filepath.Join(t.TempDir(), "docker-claude.txt")
	t.Setenv("PATH", fakeDir+":"+os.Getenv("PATH"))
	t.Setenv("DAL_TEST_CAPTURE", capture)
	t.Setenv("DAL_TEST_STDOUT", "claude task ok")

	d := New(":0", "/tmp/test", t.TempDir(), nil)
	tr := &taskResult{ID: "task-claude", Task: "inspect repo"}
	c := &Container{DalName: "writer", ContainerID: "cid-123", Player: "claude"}

	d.execTaskInContainer(c, tr)

	if tr.Status != "done" {
		t.Fatalf("status = %q, error=%q", tr.Status, tr.Error)
	}
	if tr.Output != "claude task ok" {
		t.Fatalf("output = %q", tr.Output)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"argv:[exec][-i][-e][CLAUDE_CODE_ENTRYPOINT=dalcli][cid-123][bash][-c]",
		"claude --no-session-persistence -p --allowedTools \"$TOOLS\"",
		"stdin:inspect repo",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("capture missing %q in %q", want, got)
		}
	}
}

func TestExecTaskInContainer_CodexUsesEphemeral(t *testing.T) {
	fakeDir := writeFakeDocker(t)
	capture := filepath.Join(t.TempDir(), "docker-codex.txt")
	t.Setenv("PATH", fakeDir+":"+os.Getenv("PATH"))
	t.Setenv("DAL_TEST_CAPTURE", capture)
	t.Setenv("DAL_TEST_STDOUT", "codex task ok")

	d := New(":0", "/tmp/test", t.TempDir(), nil)
	tr := &taskResult{ID: "task-codex", Task: "inspect repo"}
	c := &Container{DalName: "writer", ContainerID: "cid-456", Player: "codex"}

	d.execTaskInContainer(c, tr)

	if tr.Status != "done" {
		t.Fatalf("status = %q, error=%q", tr.Status, tr.Error)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "argv:[exec][cid-456][codex][exec][--dangerously-bypass-approvals-and-sandbox][--ephemeral][-C][/workspace][inspect repo]") {
		t.Fatalf("capture = %q", got)
	}
}
