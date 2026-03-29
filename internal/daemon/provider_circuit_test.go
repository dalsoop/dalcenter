package daemon

import (
	"testing"
	"time"
)

func TestProviderCircuitStatusResetsAfterCooldown(t *testing.T) {
	pc := &ProviderCircuit{
		primary:      "claude",
		fallback:     "codex",
		activePlayer: "codex",
		cooldown:     50 * time.Millisecond,
		trippedAt:    time.Now().Add(-time.Minute),
		trippedBy:    "leader",
		reason:       "rate limit",
	}

	status := pc.Status()
	if got := status["active_provider"]; got != "claude" {
		t.Fatalf("active_provider = %v, want claude", got)
	}
	if pc.activePlayer != "claude" {
		t.Fatalf("activePlayer = %q, want claude", pc.activePlayer)
	}
	if _, ok := status["resets_in"]; ok {
		t.Fatal("status should not expose resets_in after cooldown reset")
	}
}

func TestProviderCircuitStatusKeepsFallbackWithinCooldown(t *testing.T) {
	pc := &ProviderCircuit{
		primary:      "claude",
		fallback:     "codex",
		activePlayer: "codex",
		cooldown:     time.Hour,
		trippedAt:    time.Now().Add(-time.Minute),
		trippedBy:    "leader",
		reason:       "rate limit",
	}

	status := pc.Status()
	if got := status["active_provider"]; got != "codex" {
		t.Fatalf("active_provider = %v, want codex", got)
	}
	if _, ok := status["resets_in"]; !ok {
		t.Fatal("status should expose resets_in while cooldown remains")
	}
}
