package main

import (
	"sync"
	"testing"
	"time"
)

// ── State Transition: full cycle ──

func TestCB_FullCycle_ClosedOpenHalfOpenClosedOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 30*time.Millisecond)

	// closed → open
	cb.RecordFailure()
	cb.RecordFailure()
	assertState(t, cb, "open", "after 2 failures")

	// open → half-open (cooldown)
	time.Sleep(40 * time.Millisecond)
	if cb.ShouldFallback() {
		t.Fatal("should not fallback after cooldown")
	}
	assertState(t, cb, "half-open", "after cooldown")

	// half-open → closed (success)
	cb.RecordSuccess()
	assertState(t, cb, "closed", "after half-open success")

	// closed → open again (failures re-accumulate from 0)
	cb.RecordFailure()
	assertState(t, cb, "closed", "1 failure after reset")
	cb.RecordFailure()
	assertState(t, cb, "open", "2nd failure after reset")
}

// ── Threshold edge: threshold=1 ──

func TestCB_ThresholdOne(t *testing.T) {
	cb := NewCircuitBreaker(1, 30*time.Millisecond)
	assertState(t, cb, "closed", "initial")

	cb.RecordFailure()
	assertState(t, cb, "open", "single failure with threshold=1")
	if !cb.ShouldFallback() {
		t.Error("should fallback immediately")
	}
}

// ── Open: ShouldFallback stays true within cooldown ──

func TestCB_OpenStaysDuringCooldown(t *testing.T) {
	cb := NewCircuitBreaker(1, 200*time.Millisecond)
	cb.RecordFailure()

	for i := 0; i < 5; i++ {
		if !cb.ShouldFallback() {
			t.Fatalf("call %d: should still fallback within cooldown", i)
		}
		time.Sleep(10 * time.Millisecond)
	}
	assertState(t, cb, "open", "still within cooldown")
}

// ── Half-open: only 1 probe allowed before re-opening ──

func TestCB_HalfOpenReOpensOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 30*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(40 * time.Millisecond)
	cb.ShouldFallback() // triggers half-open

	// half-open allows 1 probe, fail it
	cb.RecordFailure()
	assertState(t, cb, "open", "half-open failed, back to open")

	// must wait cooldown again
	if !cb.ShouldFallback() {
		t.Error("should fallback again after half-open→open")
	}
}

// ── Half-open → open → half-open → closed (double bounce) ──

func TestCB_DoubleBounce(t *testing.T) {
	cb := NewCircuitBreaker(1, 30*time.Millisecond)

	// Round 1: closed → open → half-open → fail → open
	cb.RecordFailure()
	time.Sleep(40 * time.Millisecond)
	cb.ShouldFallback()
	cb.RecordFailure()
	assertState(t, cb, "open", "bounce 1: back to open")

	// Round 2: open → half-open → success → closed
	time.Sleep(40 * time.Millisecond)
	cb.ShouldFallback()
	assertState(t, cb, "half-open", "bounce 2: half-open")
	cb.RecordSuccess()
	assertState(t, cb, "closed", "bounce 2: recovered")
}

// ── Success during closed resets failure count ──

func TestCB_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Minute)

	cb.RecordFailure()
	cb.RecordFailure() // 2 failures
	cb.RecordSuccess() // reset

	// need 3 more failures to open, not 1
	cb.RecordFailure()
	assertState(t, cb, "closed", "1 failure after reset")
	cb.RecordFailure()
	assertState(t, cb, "closed", "2 failures after reset")
	cb.RecordFailure()
	assertState(t, cb, "open", "3 failures after reset → open")
}

// ── RecordSuccess on already-closed is safe ──

func TestCB_SuccessOnClosedIsNoop(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Minute)
	cb.RecordSuccess()
	cb.RecordSuccess()
	assertState(t, cb, "closed", "multiple success on closed")
	if cb.ShouldFallback() {
		t.Error("should not fallback")
	}
}

// ── RecordFailure on open keeps it open ──

func TestCB_FailureOnOpenKeepsOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Minute)
	cb.RecordFailure()
	cb.RecordFailure()
	assertState(t, cb, "open", "just opened")

	cb.RecordFailure()
	cb.RecordFailure()
	assertState(t, cb, "open", "extra failures keep it open")
}

// ── Concurrent access: no race condition ──

func TestCB_ConcurrentSafety(t *testing.T) {
	cb := NewCircuitBreaker(100, 50*time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cb.ShouldFallback()
			if n%3 == 0 {
				cb.RecordFailure()
			} else {
				cb.RecordSuccess()
			}
			_ = cb.State()
		}(i)
	}
	wg.Wait()

	// Just verify no panic/deadlock and state is valid
	s := cb.State()
	if s != "closed" && s != "open" && s != "half-open" {
		t.Errorf("invalid state after concurrent access: %q", s)
	}
}

// ── isRetryable: exhaustive keyword coverage ──

func TestIsRetryable_AllKeywords(t *testing.T) {
	positives := []string{
		"rate limit exceeded",
		"Rate Limit Exceeded",
		"RATE LIMIT",
		"429",
		"HTTP 429 Too Many Requests",
		"529",
		"Error 529",
		"overloaded",
		"server is Overloaded",
		"too many requests",
		"Too Many Requests",
		"at capacity",
		"server at capacity please retry",
		"You've hit your limit · resets 6pm (UTC)",
		"hit your limit",
		"usage limit reached",
		"API usage limit exceeded",
		"limit exceeded",
		"quota exceeded for today",
	}
	for _, s := range positives {
		if !isRetryable(s) {
			t.Errorf("isRetryable(%q) = false, want true", s)
		}
	}

	negatives := []string{
		"",
		"exit status 1",
		"file not found",
		"permission denied",
		"authentication_error",
		"401 Unauthorized",
		"timeout",
		"connection refused",
		"segfault",
	}
	for _, s := range negatives {
		if isRetryable(s) {
			t.Errorf("isRetryable(%q) = true, want false", s)
		}
	}
}

// ── isAuthError: exhaustive keyword coverage ──

func TestIsAuthError_AllKeywords(t *testing.T) {
	positives := []string{
		"401",
		"HTTP 401",
		"401 Unauthorized",
		"authentication_error",
		"authentication_error: invalid key",
		"invalid authentication credentials",
		"Invalid Authentication",
		"oauth token has expired",
		"OAuth Token Has Expired",
		"failed to authenticate",
		"Failed to authenticate. Please login again.",
	}
	for _, s := range positives {
		if !isAuthError(s) {
			t.Errorf("isAuthError(%q) = false, want true", s)
		}
	}

	negatives := []string{
		"",
		"rate limit exceeded",
		"429 Too Many Requests",
		"file not found",
		"exit status 1",
		"timeout",
		"connection refused",
		"200 OK",
	}
	for _, s := range negatives {
		if isAuthError(s) {
			t.Errorf("isAuthError(%q) = true, want false", s)
		}
	}
}

// ── isRetryable and isAuthError are mutually exclusive for clear outputs ──

func TestRetryableAndAuthError_NoOverlap(t *testing.T) {
	// Outputs that should be retryable but NOT auth errors
	retryOnly := []string{
		"rate limit exceeded",
		"429 Too Many Requests",
		"overloaded",
	}
	for _, s := range retryOnly {
		if isRetryable(s) && isAuthError(s) {
			t.Errorf("overlap: %q is both retryable and auth error", s)
		}
	}

	// Outputs that should be auth errors but NOT retryable
	authOnly := []string{
		"authentication_error: bad key",
		"OAuth token has expired",
		"failed to authenticate",
	}
	for _, s := range authOnly {
		if isRetryable(s) && isAuthError(s) {
			t.Errorf("overlap: %q is both retryable and auth error", s)
		}
	}
}

// ── "401" is auth error, not retryable ──

func TestAuthVsRetry_401(t *testing.T) {
	s := "401 Unauthorized"
	if !isAuthError(s) {
		t.Error("401 should be auth error")
	}
	if isRetryable(s) {
		t.Error("401 should NOT be retryable")
	}
}

// ── State() returns correct strings ──

func TestCB_StateStrings(t *testing.T) {
	cb := NewCircuitBreaker(1, 30*time.Millisecond)

	if s := cb.State(); s != "closed" {
		t.Errorf("initial: got %q", s)
	}

	cb.RecordFailure()
	if s := cb.State(); s != "open" {
		t.Errorf("after failure: got %q", s)
	}

	time.Sleep(40 * time.Millisecond)
	cb.ShouldFallback() // triggers half-open
	if s := cb.State(); s != "half-open" {
		t.Errorf("after cooldown: got %q", s)
	}
}

// ── ShouldFallback is idempotent in half-open ──

func TestCB_HalfOpenMultipleShouldFallback(t *testing.T) {
	cb := NewCircuitBreaker(1, 30*time.Millisecond)
	cb.RecordFailure()

	time.Sleep(40 * time.Millisecond)

	// First call transitions open → half-open
	if cb.ShouldFallback() {
		t.Fatal("first call should allow probe")
	}
	assertState(t, cb, "half-open", "first ShouldFallback")

	// Subsequent calls in half-open still allow probe
	if cb.ShouldFallback() {
		t.Fatal("second call in half-open should still allow probe")
	}
	assertState(t, cb, "half-open", "second ShouldFallback")
}

// ── Large threshold ──

func TestCB_LargeThreshold(t *testing.T) {
	cb := NewCircuitBreaker(100, time.Minute)

	for i := 0; i < 99; i++ {
		cb.RecordFailure()
	}
	assertState(t, cb, "closed", "99 failures with threshold 100")

	cb.RecordFailure()
	assertState(t, cb, "open", "100th failure triggers open")
}

// helper
func assertState(t *testing.T, cb *CircuitBreaker, want, msg string) {
	t.Helper()
	if got := cb.State(); got != want {
		t.Errorf("%s: state = %q, want %q", msg, got, want)
	}
}
