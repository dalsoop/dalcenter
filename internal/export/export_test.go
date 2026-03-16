package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPlanAndApply(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	skillDir := filepath.Join(repo, "skills", "demo-skill")
	manifestDir := filepath.Join(repo, ".dalfactory")
	manifestPath := filepath.Join(manifestDir, "dal.cue")
	claudeHome := filepath.Join(root, ".claude")
	codexHome := filepath.Join(root, ".codex")

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("demo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`schema_version: "1.0.0"
dal: {
	id:       "DAL:CLI:848a4292"
	name:     "demo"
	version:  "0.1.0"
	category: "CLI"
}
templates: default: {
	schema_version: "1.0.0"
	name:           "default"
	container: {
		base:   "ubuntu:24.04"
		agents: {}
	}
	exports: claude: {
		skills: ["skills/demo-skill/SKILL.md"]
	}
	exports: codex: {
		skills: ["skills/demo-skill/SKILL.md"]
	}
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DALCENTER_CLAUDE_HOME", claudeHome)
	t.Setenv("DALCENTER_CODEX_HOME", codexHome)

	plan, err := LoadPlan(repo)
	if err != nil {
		t.Fatalf("LoadPlan returned error: %v", err)
	}
	if got := len(plan.Exports["claude"]); got != 1 {
		t.Fatalf("unexpected claude skill count: %d", got)
	}
	if got := len(plan.Exports["codex"]); got != 1 {
		t.Fatalf("unexpected codex skill count: %d", got)
	}

	if err := Apply(plan); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	link := filepath.Join(claudeHome, "skills", "demo-skill")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink returned error: %v", err)
	}
	if target != skillDir {
		t.Fatalf("unexpected symlink target: %s", target)
	}

	codexLink := filepath.Join(codexHome, "skills", "demo-skill")
	codexTarget, err := os.Readlink(codexLink)
	if err != nil {
		t.Fatalf("Readlink returned error: %v", err)
	}
	if codexTarget != skillDir {
		t.Fatalf("unexpected codex symlink target: %s", codexTarget)
	}
}

func TestRemove(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	skillDir := filepath.Join(repo, "skills", "demo-skill")
	claudeHome := filepath.Join(root, ".claude")
	codexHome := filepath.Join(root, ".codex")
	link := filepath.Join(claudeHome, "skills", "demo-skill")
	codexLink := filepath.Join(codexHome, "skills", "demo-skill")

	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(codexLink), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skillDir, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(skillDir, codexLink); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DALCENTER_CLAUDE_HOME", claudeHome)
	t.Setenv("DALCENTER_CODEX_HOME", codexHome)

	plan := &Plan{
		RepoRoot: repo,
		Exports: map[string][]string{
			"claude": {"skills/demo-skill/SKILL.md"},
			"codex":  {"skills/demo-skill/SKILL.md"},
		},
	}
	if err := Remove(plan); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("expected symlink removed, got err=%v", err)
	}
	if _, err := os.Lstat(codexLink); !os.IsNotExist(err) {
		t.Fatalf("expected codex symlink removed, got err=%v", err)
	}
}

func TestHooksExportAndRemove(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	hookFile := filepath.Join(repo, "hooks", "pre-commit.sh")
	manifestDir := filepath.Join(repo, ".dalfactory")
	manifestPath := filepath.Join(manifestDir, "dal.cue")
	claudeHome := filepath.Join(root, ".claude")

	if err := os.MkdirAll(filepath.Dir(hookFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hookFile, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte(`schema_version: "1.0.0"
dal: {
	id:       "DAL:CLI:hooktest1"
	name:     "hook-demo"
	version:  "0.1.0"
	category: "CLI"
}
templates: default: {
	schema_version: "1.0.0"
	name:           "default"
	container: {
		base:   "ubuntu:24.04"
		agents: {}
	}
	exports: claude: {
		hooks: ["hooks/pre-commit.sh"]
	}
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DALCENTER_CLAUDE_HOME", claudeHome)

	// LoadPlan parses hooks
	plan, err := LoadPlan(repo)
	if err != nil {
		t.Fatalf("LoadPlan: %v", err)
	}
	if got := len(plan.Hooks["claude"]); got != 1 {
		t.Fatalf("expected 1 claude hook, got %d", got)
	}

	// Apply creates hook symlink
	if err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	dst := filepath.Join(claudeHome, "hooks", "pre-commit.sh")
	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("Readlink hook: %v", err)
	}
	if target != hookFile {
		t.Fatalf("hook symlink target = %s, want %s", target, hookFile)
	}

	// Remove deletes hook symlink
	if err := Remove(plan); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Fatalf("expected hook symlink removed, got err=%v", err)
	}
}

func TestApplyReturnsErrorForMissingHookPath(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	claudeHome := filepath.Join(root, ".claude")

	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DALCENTER_CLAUDE_HOME", claudeHome)

	plan := &Plan{
		RepoRoot: repo,
		Hooks: map[string][]string{
			"claude": {"hooks/missing-hook.sh"},
		},
	}

	err := Apply(plan)
	if err == nil {
		t.Fatal("Apply returned nil error for missing hook path")
	}
	if got := err.Error(); !strings.Contains(got, "stat hook file") {
		t.Fatalf("unexpected error: %v", err)
	}
}
