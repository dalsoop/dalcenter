package main

import (
	"strings"
	"sync"
	"testing"
)

// ══════════════════════════════════════════════════════════════
// classifyTaskError: 모든 패턴 키워드 개별 검증
// ══════════════════════════════════════════════════════════════

func TestClassify_EnvPatterns_All(t *testing.T) {
	cases := []struct {
		output string
		note   string
	}{
		{"bash: npm: command not found", "command not found"},
		{"/bin/sh: 1: go: not found — command not found", "command not found variant"},
		{"Error: No such file or directory: /workspace/main.go", "no such file or directory"},
		{"Permission denied (publickey).", "permission denied"},
		{"error: not a directory: /workspace/file.txt/sub", "not a directory"},
		{"exec format error: ./binary", "exec format error"},
		{"'go' is not recognized as an internal or external command", "is not recognized"},
	}
	for _, tt := range cases {
		t.Run(tt.note, func(t *testing.T) {
			if got := classifyTaskError(tt.output); got != ErrClassEnv {
				t.Errorf("classifyTaskError(%q) = %s, want env [%s]", tt.output, got, tt.note)
			}
		})
	}
}

func TestClassify_DepsPatterns_All(t *testing.T) {
	cases := []struct {
		output string
		note   string
	}{
		{"go: module github.com/foo/bar: reading github.com/foo/bar: 404", "go: module"},
		{"go mod tidy to fix missing modules", "go mod tidy"},
		{"cannot find module providing package github.com/x/y", "cannot find module"},
		{"npm ERR! code ERESOLVE", "npm err"},
		{"Error: Module not found: Error: Can't resolve 'react'", "module not found"},
		{"error[E0432]: unresolved import `crate::foo`", "unresolved import"},
		{"error: could not compile `my-crate` due to previous error", "could not compile"},
		{"Error: package not found: @types/node", "package not found"},
		{"cargo build failed with exit code 101", "cargo build"},
	}
	for _, tt := range cases {
		t.Run(tt.note, func(t *testing.T) {
			if got := classifyTaskError(tt.output); got != ErrClassDeps {
				t.Errorf("classifyTaskError(%q) = %s, want deps [%s]", tt.output, got, tt.note)
			}
		})
	}
}

func TestClassify_GitPatterns_All(t *testing.T) {
	cases := []struct {
		output string
		note   string
	}{
		{"CONFLICT (content): Merge conflict in main.go", "merge conflict"},
		{"HEAD detached at 4f2a3bc", "detached head"},
		{"You are in 'head detached' state", "head detached"},
		{"fatal: not a git repository (or any parent up to mount point /)", "not a git repository"},
		{"Your branch is behind 'origin/main' by 3 commits", "your branch is behind"},
		{"error: you need to resolve your current index first\nUnmerged files:", "unmerged files"},
		{"fatal: refusing to merge unrelated histories", "refusing to merge unrelated"},
	}
	for _, tt := range cases {
		t.Run(tt.note, func(t *testing.T) {
			if got := classifyTaskError(tt.output); got != ErrClassGit {
				t.Errorf("classifyTaskError(%q) = %s, want git [%s]", tt.output, got, tt.note)
			}
		})
	}
}

func TestClassify_Instructions_NeedsMultipleIndicators(t *testing.T) {
	tests := []struct {
		output string
		want   ErrorClass
		note   string
	}{
		// 2+ indicators + "error" → instructions
		{"error: instructions.md references stale instruction from issue #100", ErrClassInstructions, "3 indicators + error"},
		{"error in task.md: issue #200 is closed", ErrClassInstructions, "task.md + issue # + error"},

		// single indicator → NOT instructions
		{"reading instructions.md for setup", ErrClassUnknown, "single: instructions.md only"},
		{"fixed issue #42 in PR", ErrClassUnknown, "single: issue # only"},
		{"stale instruction detected", ErrClassUnknown, "single: stale instruction only"},
		{"updated task.md", ErrClassUnknown, "single: task.md only"},

		// 2 indicators but no "error" → NOT instructions
		{"instructions.md mentions issue #50", ErrClassUnknown, "2 indicators but no error"},
	}
	for _, tt := range tests {
		t.Run(tt.note, func(t *testing.T) {
			if got := classifyTaskError(tt.output); got != tt.want {
				t.Errorf("classifyTaskError(%q) = %s, want %s [%s]", tt.output, got, tt.want, tt.note)
			}
		})
	}
}

// ══════════════════════════════════════════════════════════════
// classifyTaskError: 우선순위 (env > deps > git > instructions)
// ══════════════════════════════════════════════════════════════

func TestClassify_Priority_EnvBeforeDeps(t *testing.T) {
	// 'command not found' (env) + 'npm err' (deps) → env wins (checked first)
	output := "npm ERR! command not found: webpack"
	got := classifyTaskError(output)
	if got != ErrClassEnv {
		t.Errorf("env should take priority over deps, got %s", got)
	}
}

func TestClassify_Priority_EnvBeforeGit(t *testing.T) {
	output := "permission denied: fatal: not a git repository"
	got := classifyTaskError(output)
	if got != ErrClassEnv {
		t.Errorf("env should take priority over git, got %s", got)
	}
}

func TestClassify_Priority_DepsBeforeGit(t *testing.T) {
	output := "go: module not found, your branch is behind"
	got := classifyTaskError(output)
	if got != ErrClassDeps {
		t.Errorf("deps should take priority over git, got %s", got)
	}
}

// ══════════════════════════════════════════════════════════════
// classifyTaskError: edge cases
// ══════════════════════════════════════════════════════════════

func TestClassify_EmptyString(t *testing.T) {
	if got := classifyTaskError(""); got != ErrClassUnknown {
		t.Errorf("empty → %s, want unknown", got)
	}
}

func TestClassify_CaseInsensitive(t *testing.T) {
	cases := []struct {
		output string
		want   ErrorClass
	}{
		{"COMMAND NOT FOUND", ErrClassEnv},
		{"NPM ERR! code E404", ErrClassDeps},
		{"MERGE CONFLICT in file.go", ErrClassGit},
	}
	for _, tt := range cases {
		if got := classifyTaskError(tt.output); got != tt.want {
			t.Errorf("classifyTaskError(%q) = %s, want %s", tt.output, got, tt.want)
		}
	}
}

func TestClassify_LongOutput(t *testing.T) {
	padding := strings.Repeat("ok step completed\n", 1000)
	output := padding + "fatal: not a git repository\n" + padding
	if got := classifyTaskError(output); got != ErrClassGit {
		t.Errorf("should find git error in long output, got %s", got)
	}
}

func TestClassify_MultilineErrors(t *testing.T) {
	output := `Building project...
Step 1/10: ok
Step 2/10: ok
error: could not compile 'mycrate'
aborting due to previous error`
	if got := classifyTaskError(output); got != ErrClassDeps {
		t.Errorf("multiline: got %s, want deps", got)
	}
}

// ══════════════════════════════════════════════════════════════
// extractErrorSummary: 다양한 출력 패턴
// ══════════════════════════════════════════════════════════════

func TestExtractErrorSummary_ErrorLine(t *testing.T) {
	output := "Starting build...\nCompiling main.go\nerror: undefined variable x\nDone."
	got := extractErrorSummary(output)
	if got != "error: undefined variable x" {
		t.Errorf("got %q", got)
	}
}

func TestExtractErrorSummary_NotFoundLine(t *testing.T) {
	output := "Looking for config...\nConfig file not found in /etc/app\nUsing defaults"
	got := extractErrorSummary(output)
	if !strings.Contains(got, "not found") {
		t.Errorf("should pick 'not found' line, got %q", got)
	}
}

func TestExtractErrorSummary_PermissionLine(t *testing.T) {
	output := "Opening /var/log/app.log\npermission denied while opening file\nFailed"
	got := extractErrorSummary(output)
	if !strings.Contains(got, "permission denied") {
		t.Errorf("should pick 'permission denied' line, got %q", got)
	}
}

func TestExtractErrorSummary_CannotLine(t *testing.T) {
	output := "Resolving deps...\ncannot find module providing package foo\nAborted"
	got := extractErrorSummary(output)
	if !strings.Contains(got, "cannot") {
		t.Errorf("should pick 'cannot' line, got %q", got)
	}
}

func TestExtractErrorSummary_NoKeywordLine(t *testing.T) {
	// No recognizable keyword → returns full output (or truncated)
	output := "step 1 ok\nstep 2 ok\nstep 3 failed\nexit 1"
	got := extractErrorSummary(output)
	// Should return full output since no keyword match
	if got != output {
		t.Errorf("should return full output when no keyword, got %q", got)
	}
}

func TestExtractErrorSummary_TruncatesLongOutput(t *testing.T) {
	output := strings.Repeat("x", 300)
	got := extractErrorSummary(output)
	if len(got) > 201 { // 200 + possible trailing
		t.Errorf("should truncate to ~200, got len=%d", len(got))
	}
}

func TestExtractErrorSummary_Empty(t *testing.T) {
	if got := extractErrorSummary(""); got != "" {
		t.Errorf("empty → %q, want empty", got)
	}
}

func TestExtractErrorSummary_TrimsWhitespace(t *testing.T) {
	output := "  \n  error: something broke   \n  "
	got := extractErrorSummary(output)
	if got != "error: something broke" {
		t.Errorf("should trim, got %q", got)
	}
}

// ══════════════════════════════════════════════════════════════
// taskHash: 결정론적, 충돌 없음, edge cases
// ══════════════════════════════════════════════════════════════

func TestTaskHash_Deterministic_Thorough(t *testing.T) {
	h1 := taskHash("hello world")
	h2 := taskHash("hello world")
	if h1 != h2 {
		t.Errorf("same input, different hashes: %s vs %s", h1, h2)
	}
}

func TestTaskHash_DifferentInputs(t *testing.T) {
	h1 := taskHash("task A")
	h2 := taskHash("task B")
	if h1 == h2 {
		t.Error("different inputs should produce different hashes")
	}
}

func TestTaskHash_EmptyString(t *testing.T) {
	h := taskHash("")
	if h == "" {
		t.Error("empty string should still produce a hash")
	}
	if len(h) != 16 { // sha256[:8] → 16 hex chars
		t.Errorf("hash len = %d, want 16", len(h))
	}
}

func TestTaskHash_LongInput(t *testing.T) {
	long := strings.Repeat("a", 10000)
	h := taskHash(long)
	if len(h) != 16 {
		t.Errorf("hash len = %d, want 16", len(h))
	}
}

func TestTaskHash_Unicode(t *testing.T) {
	h1 := taskHash("한국어 작업")
	h2 := taskHash("한국어 작업")
	if h1 != h2 {
		t.Error("unicode should be deterministic")
	}
	h3 := taskHash("다른 작업")
	if h1 == h3 {
		t.Error("different unicode should differ")
	}
}

// ══════════════════════════════════════════════════════════════
// repairCooldown: concurrent access, 다중 task
// ══════════════════════════════════════════════════════════════

func TestRepairCooldown_DifferentTasks(t *testing.T) {
	// 서로 다른 task는 독립적 cooldown
	markRepairAttempted("task-alpha-unique-123")
	if !isRepairCoolingDown("task-alpha-unique-123") {
		t.Error("alpha should be cooling down")
	}
	if isRepairCoolingDown("task-beta-unique-456") {
		t.Error("beta should NOT be cooling down")
	}
}

func TestRepairCooldown_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			task := "concurrent-cooldown-" + string(rune('A'+n%26))
			markRepairAttempted(task)
			_ = isRepairCoolingDown(task)
		}(i)
	}
	wg.Wait()
	// no panic/deadlock = pass
}

// ══════════════════════════════════════════════════════════════
// selfRepair: 각 ErrorClass별 분기 검증
// ══════════════════════════════════════════════════════════════

func TestSelfRepair_InstructionsNoRetry(t *testing.T) {
	retry, fix := selfRepair(
		"instructions-test-unique-xxx",
		"error: instructions.md has stale instruction referencing issue #42 in task.md",
		nil,
	)
	if retry {
		t.Error("instructions class should not retry")
	}
	if fix != "" {
		t.Errorf("fix should be empty, got %q", fix)
	}
}

func TestSelfRepair_UnknownNoRetry(t *testing.T) {
	retry, fix := selfRepair(
		"unknown-test-unique-yyy",
		"something completely unexpected happened",
		nil,
	)
	if retry {
		t.Error("unknown class should not retry")
	}
	if fix != "" {
		t.Errorf("fix should be empty, got %q", fix)
	}
}

func TestSelfRepair_CooldownBlocksSecondAttempt(t *testing.T) {
	task := "cooldown-block-test-zzz"
	// First attempt
	selfRepair(task, "unknown error", nil)
	// Second attempt should be blocked by cooldown
	retry, _ := selfRepair(task, "unknown error", nil)
	if retry {
		t.Error("second attempt should be blocked by cooldown")
	}
}

// ══════════════════════════════════════════════════════════════
// ErrorClass string values
// ══════════════════════════════════════════════════════════════

func TestErrorClass_StringValues(t *testing.T) {
	if ErrClassEnv != "env" {
		t.Errorf("ErrClassEnv = %q", ErrClassEnv)
	}
	if ErrClassDeps != "deps" {
		t.Errorf("ErrClassDeps = %q", ErrClassDeps)
	}
	if ErrClassGit != "git" {
		t.Errorf("ErrClassGit = %q", ErrClassGit)
	}
	if ErrClassInstructions != "instructions" {
		t.Errorf("ErrClassInstructions = %q", ErrClassInstructions)
	}
	if ErrClassUnknown != "unknown" {
		t.Errorf("ErrClassUnknown = %q", ErrClassUnknown)
	}
}
