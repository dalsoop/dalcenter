package daemon

import (
	"strings"
	"testing"
)

// ── Constants sanity checks ─────────────────────────────────────

func TestContainerBasePrefix(t *testing.T) {
	if containerBasePrefix == "" {
		t.Fatal("containerBasePrefix must not be empty")
	}
	if !strings.HasSuffix(containerBasePrefix, "-") {
		t.Fatalf("containerBasePrefix %q should end with '-'", containerBasePrefix)
	}
}

func TestImagePrefix(t *testing.T) {
	if imagePrefix == "" {
		t.Fatal("imagePrefix must not be empty")
	}
	if !strings.HasSuffix(imagePrefix, "/") {
		t.Fatalf("imagePrefix %q should end with '/'", imagePrefix)
	}
}

func TestContainerWorkDir(t *testing.T) {
	if containerWorkDir == "" {
		t.Fatal("containerWorkDir must not be empty")
	}
	if !strings.HasPrefix(containerWorkDir, "/") {
		t.Fatalf("containerWorkDir %q should be absolute path", containerWorkDir)
	}
}

func TestContainerDalDir(t *testing.T) {
	if containerDalDir == "" {
		t.Fatal("containerDalDir must not be empty")
	}
	if !strings.HasPrefix(containerDalDir, "/") {
		t.Fatalf("containerDalDir %q should be absolute path", containerDalDir)
	}
}

func TestDockerHostAlias(t *testing.T) {
	if dockerHostAlias == "" {
		t.Fatal("dockerHostAlias must not be empty")
	}
}

func TestDefaultLogTail(t *testing.T) {
	if defaultLogTail == "" {
		t.Fatal("defaultLogTail must not be empty")
	}
	for _, c := range defaultLogTail {
		if c < '0' || c > '9' {
			t.Fatalf("defaultLogTail %q should be numeric", defaultLogTail)
		}
	}
}

func TestDefaultGitEmailDomain(t *testing.T) {
	if defaultGitEmailDomain == "" {
		t.Fatal("defaultGitEmailDomain must not be empty")
	}
	if !strings.Contains(defaultGitEmailDomain, ".") {
		t.Fatalf("defaultGitEmailDomain %q should contain '.'", defaultGitEmailDomain)
	}
}

// ── Container naming with team ──────────────────────────────────

func TestContainerNameFormat(t *testing.T) {
	tests := []struct {
		team     string
		instance string
		want     string
	}{
		{"vk", "dev", "dal-vk-dev"},
		{"gaya", "leader", "dal-gaya-leader"},
		{"dc", "story-checker", "dal-dc-story-checker"},
	}
	for _, tt := range tests {
		got := containerBasePrefix + tt.team + "-" + tt.instance
		if got != tt.want {
			t.Errorf("container name team=%q instance=%q = %q, want %q", tt.team, tt.instance, got, tt.want)
		}
	}
}

// ── Image naming ────────────────────────────────────────────────

func TestImageNameFormat(t *testing.T) {
	tests := []struct {
		player  string
		version string
		want    string
	}{
		{"claude", "latest", "dalcenter/claude:latest"},
		{"codex", "latest", "dalcenter/codex:latest"},
		{"gemini", "1.0", "dalcenter/gemini:1.0"},
	}
	for _, tt := range tests {
		got := imagePrefix + tt.player + ":" + tt.version
		if got != tt.want {
			t.Errorf("image for %q:%q = %q, want %q", tt.player, tt.version, got, tt.want)
		}
	}
}

// ── Git email format with team ──────────────────────────────────

func TestGitEmailFormat(t *testing.T) {
	tests := []struct {
		team    string
		dalName string
		want    string
	}{
		{"vk", "dev", "dal-vk-dev@dalcenter.local"},
		{"gaya", "leader", "dal-gaya-leader@dalcenter.local"},
		{"dc", "verifier", "dal-dc-verifier@dalcenter.local"},
	}
	for _, tt := range tests {
		got := containerBasePrefix + tt.team + "-" + tt.dalName + "@" + defaultGitEmailDomain
		if got != tt.want {
			t.Errorf("email for team=%q dal=%q = %q, want %q", tt.team, tt.dalName, got, tt.want)
		}
	}
}
