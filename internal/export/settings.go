package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dalcenter-managed hooks carry this marker in the command string.
const hookMarker = " # dalcenter-managed"

// HookEntry describes a hook declaration from .dalfactory.
type HookEntry struct {
	Event   string // e.g. "PreCompact", "Stop", "SessionStart"
	Matcher string // e.g. "Bash", "" (empty = match all)
	Command string // e.g. "bash hooks/pre-compact.sh"
	DalName string // owning dal package name
}

// settingsJSON mirrors the relevant part of ~/.claude/settings.json.
type settingsJSON struct {
	raw map[string]interface{}
}

func loadSettings(path string) (*settingsJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &settingsJSON{raw: map[string]interface{}{}}, nil
		}
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}
	return &settingsJSON{raw: m}, nil
}

func (s *settingsJSON) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.raw, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// addHook adds a dalcenter-managed hook entry, preserving existing entries.
func (s *settingsJSON) addHook(entry HookEntry) {
	hooks := s.ensureHooksMap()
	eventEntries := s.getEventEntries(hooks, entry.Event)

	command := entry.Command + hookMarker
	hookObj := map[string]interface{}{
		"type":    "command",
		"command": command,
	}

	// Check if already exists
	for _, e := range eventEntries {
		em, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		for _, h := range toSlice(em["hooks"]) {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if hm["command"] == command {
				return // already registered
			}
		}
	}

	// Add new entry
	newEntry := map[string]interface{}{
		"matcher": entry.Matcher,
		"hooks":   []interface{}{hookObj},
	}
	eventEntries = append(eventEntries, newEntry)
	hooks[entry.Event] = eventEntries
}

// removeHook removes dalcenter-managed hooks matching the command prefix.
func (s *settingsJSON) removeHook(entry HookEntry) {
	hooks := s.ensureHooksMap()
	eventEntries := s.getEventEntries(hooks, entry.Event)
	if len(eventEntries) == 0 {
		return
	}

	command := entry.Command + hookMarker
	var kept []interface{}
	for _, e := range eventEntries {
		em, ok := e.(map[string]interface{})
		if !ok {
			kept = append(kept, e)
			continue
		}
		hooksList := toSlice(em["hooks"])
		var keptHooks []interface{}
		for _, h := range hooksList {
			hm, ok := h.(map[string]interface{})
			if !ok {
				keptHooks = append(keptHooks, h)
				continue
			}
			cmd, _ := hm["command"].(string)
			if cmd != command {
				keptHooks = append(keptHooks, h)
			}
		}
		if len(keptHooks) > 0 {
			em["hooks"] = keptHooks
			kept = append(kept, em)
		}
	}
	if len(kept) == 0 {
		delete(hooks, entry.Event)
	} else {
		hooks[entry.Event] = kept
	}
}

func (s *settingsJSON) ensureHooksMap() map[string]interface{} {
	h, ok := s.raw["hooks"]
	if !ok {
		m := map[string]interface{}{}
		s.raw["hooks"] = m
		return m
	}
	if m, ok := h.(map[string]interface{}); ok {
		return m
	}
	m := map[string]interface{}{}
	s.raw["hooks"] = m
	return m
}

func (s *settingsJSON) getEventEntries(hooks map[string]interface{}, event string) []interface{} {
	v, ok := hooks[event]
	if !ok {
		return nil
	}
	return toSlice(v)
}

func toSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return nil
}

// ParseHookEntries extracts hook entries from .dalfactory hook file paths.
// Hook files should contain a header comment: # event:PreCompact matcher:Bash
// If no header, defaults to event=SessionStart, matcher="" (match all).
func ParseHookEntries(repoRoot string, hookPaths []string, dalName string) []HookEntry {
	var entries []HookEntry
	for _, rel := range hookPaths {
		src := filepath.Join(repoRoot, filepath.Clean(rel))
		event, matcher := parseHookHeader(src)
		entries = append(entries, HookEntry{
			Event:   event,
			Matcher: matcher,
			Command: src,
			DalName: dalName,
		})
	}
	return entries
}

func parseHookHeader(path string) (event, matcher string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "SessionStart", ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#") {
			break
		}
		if strings.Contains(line, "event:") {
			for _, part := range strings.Fields(line) {
				if strings.HasPrefix(part, "event:") {
					event = strings.TrimPrefix(part, "event:")
				}
				if strings.HasPrefix(part, "matcher:") {
					matcher = strings.TrimPrefix(part, "matcher:")
				}
			}
			return event, matcher
		}
	}
	return "SessionStart", ""
}

// MergeHooksToSettings adds hook entries to the Claude settings.json.
func MergeHooksToSettings(settingsPath string, entries []HookEntry) error {
	if len(entries) == 0 {
		return nil
	}
	s, err := loadSettings(settingsPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s.addHook(e)
	}
	return s.save(settingsPath)
}

// RemoveHooksFromSettings removes dalcenter-managed hook entries from settings.json.
func RemoveHooksFromSettings(settingsPath string, entries []HookEntry) error {
	if len(entries) == 0 {
		return nil
	}
	s, err := loadSettings(settingsPath)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s.removeHook(e)
	}
	return s.save(settingsPath)
}
