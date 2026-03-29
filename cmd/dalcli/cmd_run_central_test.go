package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestExecuteTask_CentralOverrideFailureDoesNotRetryPrimary(t *testing.T) {
	providerCircuit = NewCircuitBreaker(3, 2*time.Minute)
	defer func() { providerCircuit = NewCircuitBreaker(3, 2*time.Minute) }()

	tmpDir := t.TempDir()
	primaryMarker := filepath.Join(tmpDir, "claude-called")
	fallbackMarker := filepath.Join(tmpDir, "codex-called")

	writeExecutable(t, tmpDir, "claude", fmt.Sprintf("#!/bin/sh\necho called > %s\necho primary-should-not-run\nexit 1\n", primaryMarker))
	writeExecutable(t, tmpDir, "codex", fmt.Sprintf("#!/bin/sh\necho called > %s\necho central-fallback-failed\nexit 1\n", fallbackMarker))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/provider-status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"active_provider":"codex"}`))
	}))
	defer srv.Close()

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	os.Setenv("DALCENTER_URL", srv.URL)
	os.Setenv("DAL_PLAYER", "claude")
	os.Setenv("DAL_ROLE", "member")
	os.Setenv("DAL_MAX_DURATION", "1s")
	defer os.Setenv("PATH", oldPath)
	defer os.Unsetenv("DALCENTER_URL")
	defer os.Unsetenv("DAL_PLAYER")
	defer os.Unsetenv("DAL_ROLE")
	defer os.Unsetenv("DAL_MAX_DURATION")

	out, err := executeTask("test")
	if err == nil {
		t.Fatal("expected central override failure")
	}
	if !strings.Contains(err.Error(), "central provider codex failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "central-fallback-failed" {
		t.Fatalf("output = %q, want central fallback output", out)
	}
	if _, statErr := os.Stat(fallbackMarker); statErr != nil {
		t.Fatalf("codex should be invoked: %v", statErr)
	}
	if _, statErr := os.Stat(primaryMarker); !os.IsNotExist(statErr) {
		t.Fatalf("primary claude should not be invoked, stat err=%v", statErr)
	}
}

func TestExecuteTask_OpenCircuitFallbackFailureDoesNotRetryPrimary(t *testing.T) {
	providerCircuit = NewCircuitBreaker(1, 2*time.Minute)
	defer func() { providerCircuit = NewCircuitBreaker(3, 2*time.Minute) }()
	providerCircuit.RecordFailure()

	tmpDir := t.TempDir()
	primaryMarker := filepath.Join(tmpDir, "claude-called")
	fallbackMarker := filepath.Join(tmpDir, "codex-called")

	writeExecutable(t, tmpDir, "claude", fmt.Sprintf("#!/bin/sh\necho called >> %s\necho primary-should-not-run\nexit 1\n", primaryMarker))
	writeExecutable(t, tmpDir, "codex", fmt.Sprintf("#!/bin/sh\necho called >> %s\necho fallback-failed-because-open\nexit 1\n", fallbackMarker))

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	os.Setenv("DAL_PLAYER", "claude")
	os.Setenv("DAL_ROLE", "member")
	os.Setenv("DAL_MAX_DURATION", "1s")
	defer os.Setenv("PATH", oldPath)
	defer os.Unsetenv("DAL_PLAYER")
	defer os.Unsetenv("DAL_ROLE")
	defer os.Unsetenv("DAL_MAX_DURATION")

	out, err := executeTask("test")
	if err == nil {
		t.Fatal("expected fallback failure")
	}
	if !strings.Contains(err.Error(), "fallback codex failed while primary claude circuit is open") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) != "fallback-failed-because-open" {
		t.Fatalf("output = %q, want fallback output", out)
	}
	if _, statErr := os.Stat(fallbackMarker); statErr != nil {
		t.Fatalf("codex should be invoked: %v", statErr)
	}
	if _, statErr := os.Stat(primaryMarker); !os.IsNotExist(statErr) {
		t.Fatalf("primary claude should not be invoked when circuit is already open, stat err=%v", statErr)
	}
}

func TestExecuteTask_CircuitOpensAndStopsRetryingPrimaryAfterFallbackFailure(t *testing.T) {
	providerCircuit = NewCircuitBreaker(1, 2*time.Minute)
	defer func() { providerCircuit = NewCircuitBreaker(3, 2*time.Minute) }()

	tmpDir := t.TempDir()
	primaryMarker := filepath.Join(tmpDir, "claude-called")
	fallbackMarker := filepath.Join(tmpDir, "codex-called")

	writeExecutable(t, tmpDir, "claude", fmt.Sprintf("#!/bin/sh\necho called >> %s\necho You've hit your limit\nexit 1\n", primaryMarker))
	writeExecutable(t, tmpDir, "codex", fmt.Sprintf("#!/bin/sh\necho called >> %s\necho fallback-failed-after-open\nexit 1\n", fallbackMarker))

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath)
	os.Setenv("DAL_PLAYER", "claude")
	os.Setenv("DAL_ROLE", "member")
	os.Setenv("DAL_MAX_DURATION", "1s")
	defer os.Setenv("PATH", oldPath)
	defer os.Unsetenv("DAL_PLAYER")
	defer os.Unsetenv("DAL_ROLE")
	defer os.Unsetenv("DAL_MAX_DURATION")

	out, err := executeTask("test")
	if err == nil {
		t.Fatal("expected fallback failure after circuit open")
	}
	if !strings.Contains(err.Error(), "fallback codex failed while primary claude circuit is open") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "max retries") {
		t.Fatalf("should stop after circuit opens and fallback fails, got retry exhaustion: %v", err)
	}
	if strings.TrimSpace(out) != "fallback-failed-after-open" {
		t.Fatalf("output = %q, want fallback output", out)
	}

	primaryData, readErr := os.ReadFile(primaryMarker)
	if readErr != nil {
		t.Fatalf("read primary marker: %v", readErr)
	}
	if got := strings.Count(strings.TrimSpace(string(primaryData)), "called"); got != 1 {
		t.Fatalf("primary should be invoked exactly once before circuit opens, got %d", got)
	}

	fallbackData, readErr := os.ReadFile(fallbackMarker)
	if readErr != nil {
		t.Fatalf("read fallback marker: %v", readErr)
	}
	if got := strings.Count(strings.TrimSpace(string(fallbackData)), "called"); got != 1 {
		t.Fatalf("fallback should be invoked exactly once after circuit opens, got %d", got)
	}
}
