package talk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claude")
	script := `#!/bin/sh
{
  printf 'argv:'
  for arg in "$@"; do
    printf '[%s]' "$arg"
  done
  printf '\n'
} > "$DAL_TEST_CAPTURE"
printf '%s' "${DAL_TEST_STDOUT:-ok}"
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	return path
}

func TestExtractResultSuccess(t *testing.T) {
	input := `{"type":"system","subtype":"init","session_id":"abc"}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read"}]}}
{"type":"user","message":{"content":[{"tool_use_id":"x","type":"tool_result","content":"file data"}]}}
{"type":"result","subtype":"success","result":"README의 첫 3줄입니다.","duration_ms":5000}`

	got := extractResult(input)
	want := "README의 첫 3줄입니다."
	if got != want {
		t.Fatalf("extractResult() = %q, want %q", got, want)
	}
}

func TestExtractResultEmpty(t *testing.T) {
	input := `{"type":"system","subtype":"init"}
{"type":"result","subtype":"success","result":"","duration_ms":100}`

	got := extractResult(input)
	// Empty result → returns raw output
	if got == "" {
		t.Fatal("extractResult() should not return empty for non-empty input")
	}
}

func TestExtractResultNoResultLine(t *testing.T) {
	input := `just some text output without json`
	got := extractResult(input)
	if got != "just some text output without json" {
		t.Fatalf("extractResult() fallback = %q", got)
	}
}

func TestExtractResultMultiline(t *testing.T) {
	input := `{"type":"result","subtype":"success","result":"line1\nline2\nline3"}`
	got := extractResult(input)
	if got != "line1\nline2\nline3" {
		t.Fatalf("extractResult() multiline = %q", got)
	}
}

func TestSanitizerClean(t *testing.T) {
	s := NewSanitizer()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"veilkey token", "token is VK:my-secret-key here", "token is [VK:***] here"},
		{"bearer token", "Bearer eyJhbGciOiJIUzI1NiJ9.test", "Bearer [REDACTED]"},
		{"aws secret", "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI", "AWS_SECRET_ACCESS_KEY=[REDACTED]"},
		{"no secrets", "just normal text", "just normal text"},
		{"empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := s.Clean(tc.input)
			if got != tc.want {
				t.Fatalf("Clean(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSanitizer_PreservesNormal(t *testing.T) {
	s := NewSanitizer()
	result := s.Clean("hello world 123")
	if result != "hello world 123" {
		t.Errorf("got %q", result)
	}
}

func TestExtractResult_Success(t *testing.T) {
	result := extractResult("some output\nmore output\nfinal line")
	if result == "" {
		t.Fatal("should extract result")
	}
}

func TestExtractResult_WithError(t *testing.T) {
	result := extractResult("partial output")
	if result == "" {
		t.Fatal("should still return something on error")
	}
}

func TestTalkSessionPersistenceEnabled_DefaultOff(t *testing.T) {
	t.Setenv("DAL_PERSIST_TALK_SESSION", "")
	if talkSessionPersistenceEnabled() {
		t.Fatal("persistence should be disabled by default")
	}
}

func TestTalkSessionPersistenceEnabled_ExplicitOn(t *testing.T) {
	for _, v := range []string{"1", "true", "yes", "on", "TRUE"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("DAL_PERSIST_TALK_SESSION", v)
			if !talkSessionPersistenceEnabled() {
				t.Fatalf("expected %q to enable persistence", v)
			}
		})
	}
}

func TestExecutorSource_DisablesSessionPersistenceByDefault(t *testing.T) {
	data, err := os.ReadFile("executor.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	src := string(data)
	if !strings.Contains(src, "--no-session-persistence") {
		t.Fatal("executor must disable session persistence by default")
	}
}

func TestExecutorRun_Ask_DefaultNonPersistent(t *testing.T) {
	bin := writeFakeClaude(t)
	capture := filepath.Join(t.TempDir(), "ask.txt")
	t.Setenv("DAL_TEST_CAPTURE", capture)
	t.Setenv("DAL_TEST_STDOUT", "ask ok")

	e := &Executor{Binary: bin, Role: "reviewer"}
	out, err := e.Run(context.Background(), ModeAsk, "hello")
	if err != nil {
		t.Fatalf("Run ask: %v", err)
	}
	if out != "ask ok" {
		t.Fatalf("stdout = %q", out)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	got := string(data)
	for _, want := range []string{
		"[--no-session-persistence]",
		"[--print][너는 reviewer 역할이야. hello]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("capture missing %q in %q", want, got)
		}
	}
}

func TestExecutorRun_Exec_PersistenceOverride(t *testing.T) {
	bin := writeFakeClaude(t)
	capture := filepath.Join(t.TempDir(), "exec.txt")
	t.Setenv("DAL_TEST_CAPTURE", capture)
	t.Setenv("DAL_TEST_STDOUT", `{"type":"result","result":"done"}`)
	t.Setenv("DAL_PERSIST_TALK_SESSION", "true")

	e := &Executor{Binary: bin}
	out, err := e.Run(context.Background(), ModeExec, "fix it")
	if err != nil {
		t.Fatalf("Run exec: %v", err)
	}
	if out != "done" {
		t.Fatalf("result = %q", out)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	got := string(data)
	if strings.Contains(got, "--no-session-persistence") {
		t.Fatalf("capture should omit no-session-persistence: %q", got)
	}
	for _, want := range []string{
		"[-p][fix it]",
		"[--allowedTools][Bash,Read,Write,Edit]",
		"[--output-format][stream-json]",
		"[--verbose]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("capture missing %q in %q", want, got)
		}
	}
}
