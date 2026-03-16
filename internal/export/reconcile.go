package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReconcileResult describes the outcome for one instance.
type ReconcileResult struct {
	DalID     string
	Status    string // "ok", "repaired", "error"
	Repairs   []string
	Errors    []string
}

// Reconcile checks skill/hook symlinks and settings.json for drift, repairing where possible.
func Reconcile(plan *Plan, runtimeHomes map[string]string) ReconcileResult {
	result := ReconcileResult{Status: "ok"}

	// Check skills
	for runtime, skills := range plan.Exports {
		home, ok := runtimeHomes[runtime]
		if !ok {
			home, _ = runtimeHome(runtime)
			if home == "" {
				continue
			}
		}
		sr := filepath.Join(home, "skills")
		for _, rel := range skills {
			src := filepath.Join(plan.RepoRoot, filepath.Clean(rel))
			srcDir := filepath.Dir(src)
			name := filepath.Base(srcDir)
			dst := filepath.Join(sr, name)

			repair, err := checkSymlink(dst, srcDir, "skill")
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("skill %s: %v", name, err))
			} else if repair != "" {
				result.Repairs = append(result.Repairs, repair)
			}
		}
	}

	// Check hooks (file symlinks)
	for runtime, hooks := range plan.Hooks {
		home, ok := runtimeHomes[runtime]
		if !ok {
			home, _ = runtimeHome(runtime)
			if home == "" {
				continue
			}
		}
		hr := filepath.Join(home, "hooks")
		for _, rel := range hooks {
			src := filepath.Join(plan.RepoRoot, filepath.Clean(rel))
			name := filepath.Base(src)
			dst := filepath.Join(hr, name)

			repair, err := checkSymlink(dst, src, "hook")
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("hook %s: %v", name, err))
			} else if repair != "" {
				result.Repairs = append(result.Repairs, repair)
			}
		}

		// Check settings.json hooks (Claude only)
		if runtime == "claude" {
			repairs := reconcileSettings(plan, home)
			result.Repairs = append(result.Repairs, repairs...)
		}
	}

	if len(result.Errors) > 0 {
		result.Status = "error"
	} else if len(result.Repairs) > 0 {
		result.Status = "repaired"
	}
	return result
}

// checkSymlink verifies a symlink points to expected target, repairing if drifted.
func checkSymlink(dst, expectedTarget, kind string) (repair string, err error) {
	info, err := os.Lstat(dst)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing — recreate
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return "", fmt.Errorf("mkdir for %s: %w", kind, err)
			}
			if err := os.Symlink(expectedTarget, dst); err != nil {
				return "", fmt.Errorf("recreate %s symlink: %w", kind, err)
			}
			return fmt.Sprintf("%s %q: recreated (was missing)", kind, filepath.Base(dst)), nil
		}
		return "", err
	}

	if info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("%s %q: exists but is not a symlink", kind, filepath.Base(dst))
	}

	target, err := os.Readlink(dst)
	if err != nil {
		return "", err
	}

	if target == expectedTarget {
		return "", nil // ok
	}

	// Drifted — repair
	os.Remove(dst)
	if err := os.Symlink(expectedTarget, dst); err != nil {
		return "", fmt.Errorf("repair %s symlink: %w", kind, err)
	}
	return fmt.Sprintf("%s %q: repaired (was %s)", kind, filepath.Base(dst), target), nil
}

// reconcileSettings checks that dalcenter-managed hooks are present in settings.json.
func reconcileSettings(plan *Plan, claudeHome string) []string {
	hooks, ok := plan.Hooks["claude"]
	if !ok || len(hooks) == 0 {
		return nil
	}

	settingsPath := filepath.Join(claudeHome, "settings.json")
	s, err := loadSettings(settingsPath)
	if err != nil {
		return nil
	}

	dalName := filepath.Base(plan.RepoRoot)
	entries := ParseHookEntries(plan.RepoRoot, hooks, dalName)

	var repairs []string
	hooksMap := s.ensureHooksMap()
	for _, entry := range entries {
		command := entry.Command + hookMarker
		found := false
		eventEntries := s.getEventEntries(hooksMap, entry.Event)
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
				if cmd, _ := hm["command"].(string); cmd == command {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			s.addHook(entry)
			repairs = append(repairs, fmt.Sprintf("settings.json: re-added %s hook for %s", entry.Event, strings.TrimSuffix(filepath.Base(entry.Command), filepath.Ext(entry.Command))))
		}
	}

	if len(repairs) > 0 {
		s.save(settingsPath)
	}
	return repairs
}
