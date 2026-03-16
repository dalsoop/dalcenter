package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMergeAndRemoveHooksSettings(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Start with existing settings
	initial := map[string]interface{}{
		"permissions": map[string]interface{}{"defaultMode": "bypassPermissions"},
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{"type": "command", "command": "existing-hook"},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(initial, "", "    ")
	os.WriteFile(settingsPath, data, 0644)

	// Add a dalcenter-managed hook
	entries := []HookEntry{
		{Event: "PreCompact", Matcher: "", Command: "/repo/hooks/pre-compact.sh", DalName: "test-dal"},
	}
	if err := MergeHooksToSettings(settingsPath, entries); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// Verify: existing hook preserved + new hook added
	s, err := loadSettings(settingsPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	hooks := s.ensureHooksMap()

	// Existing PreToolUse should still be there
	preToolUse := s.getEventEntries(hooks, "PreToolUse")
	if len(preToolUse) != 1 {
		t.Fatalf("expected 1 PreToolUse entry, got %d", len(preToolUse))
	}

	// New PreCompact should be added
	preCompact := s.getEventEntries(hooks, "PreCompact")
	if len(preCompact) != 1 {
		t.Fatalf("expected 1 PreCompact entry, got %d", len(preCompact))
	}
	// Verify marker
	em := preCompact[0].(map[string]interface{})
	hooksList := toSlice(em["hooks"])
	hm := hooksList[0].(map[string]interface{})
	cmd := hm["command"].(string)
	if cmd != "/repo/hooks/pre-compact.sh"+hookMarker {
		t.Fatalf("unexpected command: %s", cmd)
	}

	// Idempotent: adding again should not duplicate
	if err := MergeHooksToSettings(settingsPath, entries); err != nil {
		t.Fatalf("merge again: %v", err)
	}
	s2, _ := loadSettings(settingsPath)
	hooks2 := s2.ensureHooksMap()
	if len(s2.getEventEntries(hooks2, "PreCompact")) != 1 {
		t.Fatal("duplicate entry created")
	}

	// Remove
	if err := RemoveHooksFromSettings(settingsPath, entries); err != nil {
		t.Fatalf("remove: %v", err)
	}
	s3, _ := loadSettings(settingsPath)
	hooks3 := s3.ensureHooksMap()
	if len(s3.getEventEntries(hooks3, "PreCompact")) != 0 {
		t.Fatal("hook not removed")
	}
	// Existing PreToolUse should still be there
	if len(s3.getEventEntries(hooks3, "PreToolUse")) != 1 {
		t.Fatal("existing hook was removed")
	}
}

func TestMergeHooksCreatesSettingsIfMissing(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	entries := []HookEntry{
		{Event: "Stop", Matcher: "", Command: "/repo/hooks/cleanup.sh", DalName: "test"},
	}
	if err := MergeHooksToSettings(settingsPath, entries); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings.json not created: %v", err)
	}
}

func TestParseHookHeader(t *testing.T) {
	dir := t.TempDir()

	// Hook with header
	hookFile := filepath.Join(dir, "my-hook.sh")
	os.WriteFile(hookFile, []byte("#!/bin/sh\n# event:PreCompact matcher:Bash\necho hello\n"), 0755)
	event, matcher := parseHookHeader(hookFile)
	if event != "PreCompact" || matcher != "Bash" {
		t.Fatalf("expected PreCompact/Bash, got %s/%s", event, matcher)
	}

	// Hook without header
	hookFile2 := filepath.Join(dir, "plain.sh")
	os.WriteFile(hookFile2, []byte("#!/bin/sh\necho hello\n"), 0755)
	event2, matcher2 := parseHookHeader(hookFile2)
	if event2 != "SessionStart" || matcher2 != "" {
		t.Fatalf("expected SessionStart/'', got %s/%s", event2, matcher2)
	}
}
