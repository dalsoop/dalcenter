package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	configSyncInterval = 30 * time.Minute
	configSyncLogTag   = "[config-sync]"
)

// syncableFiles are template files that should be synced to team repos.
// Team-specific files (dal.cue, instructions.md) are excluded.
var syncableFiles = []string{
	"charter.md",
	"dal.spec.cue",
}

// syncableDirs are template directories that should be synced.
var syncableDirs = []string{
	"skills/",
}

// configSyncState persists the last-synced template hash.
type configSyncState struct {
	LastHash string    `json:"last_hash"`
	LastSync time.Time `json:"last_sync"`
	mu       sync.Mutex
}

// startConfigSyncWatcher periodically checks .dal/template/ for changes
// and logs sync-needed alerts when shared files (charter, schema, skills) change.
func (d *Daemon) startConfigSyncWatcher(ctx context.Context) {
	templateDir := filepath.Join(d.serviceRepo, ".dal", "template")
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		log.Printf("%s template dir not found (%s) — skipping", configSyncLogTag, templateDir)
		return
	}

	stateFile := filepath.Join(stateDir(d.serviceRepo), "config_sync_state.json")
	state := loadConfigSyncState(stateFile)

	log.Printf("%s started (interval=%s, template=%s)", configSyncLogTag, configSyncInterval, templateDir)

	// Run initial check
	d.configSyncCheck(templateDir, state, stateFile)

	ticker := time.NewTicker(configSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("%s stopped", configSyncLogTag)
			return
		case <-ticker.C:
			d.configSyncCheck(templateDir, state, stateFile)
		}
	}
}

// configSyncCheck computes the current template hash, compares with last-synced,
// and alerts if changes are detected.
func (d *Daemon) configSyncCheck(templateDir string, state *configSyncState, stateFile string) {
	hash, exists, err := templateSyncableHash(templateDir)
	if err != nil {
		log.Printf("%s hash error: %v", configSyncLogTag, err)
		return
	}
	if !exists {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.LastHash == "" {
		// First run — record baseline, no alert
		state.LastHash = hash
		state.LastSync = time.Now()
		saveConfigSyncState(stateFile, state)
		log.Printf("%s baseline recorded: %s", configSyncLogTag, short(hash))
		return
	}

	if hash == state.LastHash {
		return
	}

	// Detect which syncable files changed
	diff := templateDiffSince(templateDir, state.LastHash)
	log.Printf("%s template changed (%s → %s): %s",
		configSyncLogTag, short(state.LastHash), short(hash), strings.Join(diff, ", "))

	// Alert via bridge
	msg := fmt.Sprintf(":arrows_counterclockwise: **config-sync** — 템플릿 변경 감지: %s. config-manager auto_task에서 동기화 PR 생성 예정.",
		strings.Join(diff, ", "))
	d.postAlert(msg)

	// Check tool installation in running containers
	d.checkToolInstallation(templateDir)

	state.LastHash = hash
	state.LastSync = time.Now()
	saveConfigSyncState(stateFile, state)
}

// templateSyncableHash computes a deterministic hash of all syncable files
// in the template directory (charter.md, dal.spec.cue, skills/).
func templateSyncableHash(templateDir string) (hash string, exists bool, err error) {
	h := sha256.New()
	var found bool

	// Hash syncable files at template root
	for _, name := range syncableFiles {
		path := filepath.Join(templateDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", false, fmt.Errorf("read %s: %w", name, err)
		}
		found = true
		h.Write([]byte(name))
		h.Write(data)
	}

	// Hash syncable directories (skills/)
	for _, dir := range syncableDirs {
		dirPath := filepath.Join(templateDir, dir)
		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(templateDir, path)
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read %s: %w", rel, err)
			}
			found = true
			h.Write([]byte(rel))
			h.Write(data)
			return nil
		})
		if err != nil {
			return "", false, fmt.Errorf("walk %s: %w", dir, err)
		}
	}

	if !found {
		return "", false, nil
	}
	return hex.EncodeToString(h.Sum(nil)), true, nil
}

// templateDiffSince returns the list of changed syncable files since the given hash.
// Uses git diff if the template dir is in a git repo, otherwise returns all syncable files.
func templateDiffSince(templateDir string, lastHash string) []string {
	// Try git diff against HEAD
	repoDir := filepath.Dir(filepath.Dir(templateDir)) // .dal/template -> repo root
	cmd := exec.Command("git", "diff", "--name-only", "HEAD~1", "--", ".dal/template/")
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		// Fallback: return all syncable patterns
		var all []string
		all = append(all, syncableFiles...)
		all = append(all, syncableDirs...)
		return all
	}

	var changed []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// Strip .dal/template/ prefix
		rel := strings.TrimPrefix(line, ".dal/template/")
		// Check if it's a syncable file/dir
		for _, sf := range syncableFiles {
			if rel == sf && !seen[sf] {
				changed = append(changed, sf)
				seen[sf] = true
			}
		}
		for _, sd := range syncableDirs {
			if strings.HasPrefix(rel, sd) && !seen[sd] {
				changed = append(changed, sd)
				seen[sd] = true
			}
		}
	}
	sort.Strings(changed)
	return changed
}

// checkToolInstallation verifies that tools mentioned in charter.md files
// are available in running containers.
func (d *Daemon) checkToolInstallation(templateDir string) {
	d.mu.RLock()
	running := make(map[string]*Container)
	for k, v := range d.containers {
		if v.Status == "running" {
			running[k] = v
		}
	}
	d.mu.RUnlock()

	if len(running) == 0 {
		return
	}

	// Discover required tools from charter.md files in template subdirs
	tools := discoverCharterTools(templateDir)
	if len(tools) == 0 {
		return
	}

	for name, c := range running {
		for _, tool := range tools {
			if !containerHasBinary(c.ContainerID, tool) {
				log.Printf("%s tool missing: %s not found in %s", configSyncLogTag, tool, name)
				d.fileToolMissingIssue(name, tool)
			}
		}
	}
}

// discoverCharterTools parses charter.md files to extract tool binary names
// listed under the ## Tools section.
func discoverCharterTools(templateDir string) []string {
	var tools []string
	seen := make(map[string]bool)

	entries, err := os.ReadDir(templateDir)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		charterPath := filepath.Join(templateDir, e.Name(), "charter.md")
		data, err := os.ReadFile(charterPath)
		if err != nil {
			continue
		}

		// Parse ## Tools section
		inTools := false
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "## Tools") {
				inTools = true
				continue
			}
			if inTools && strings.HasPrefix(trimmed, "## ") {
				break // next section
			}
			if inTools && strings.HasPrefix(trimmed, "- ") {
				// Extract binary name: "- gh — GitHub CLI ..."  →  "gh"
				entry := strings.TrimPrefix(trimmed, "- ")
				parts := strings.Fields(entry)
				if len(parts) > 0 {
					bin := parts[0]
					// Strip trailing punctuation
					bin = strings.TrimRight(bin, "—-:,")
					if bin != "" && !seen[bin] {
						tools = append(tools, bin)
						seen[bin] = true
					}
				}
			}
		}
	}
	return tools
}

// containerHasBinary checks if a binary exists in a running Docker container.
func containerHasBinary(containerID, binary string) bool {
	cmd := exec.Command("docker", "exec", containerID, "which", binary)
	return cmd.Run() == nil
}

// fileToolMissingIssue creates a GitHub issue for a missing tool.
func (d *Daemon) fileToolMissingIssue(dalName, tool string) {
	if d.githubRepo == "" {
		log.Printf("%s no github repo configured — skipping issue for %s/%s", configSyncLogTag, dalName, tool)
		return
	}

	title := fmt.Sprintf("tool missing: %s in %s", tool, dalName)

	// Check if issue already exists to avoid duplicates
	cmd := exec.Command("gh", "issue", "list",
		"--repo", d.githubRepo,
		"--label", "config-audit",
		"--search", title,
		"--state", "open",
		"--json", "number",
		"--jq", "length",
	)
	out, err := cmd.Output()
	if err == nil && strings.TrimSpace(string(out)) != "0" {
		log.Printf("%s issue already exists for %s/%s — skipping", configSyncLogTag, dalName, tool)
		return
	}

	cmd = exec.Command("gh", "issue", "create",
		"--repo", d.githubRepo,
		"--title", title,
		"--label", "config-audit",
		"--body", fmt.Sprintf("config-sync 자동 감사: `%s` 컨테이너에 `%s` 바이너리가 없습니다.\n\ncharter.md에 명시된 도구가 설치되어 있지 않으면 해당 dal의 기능이 제한됩니다.", dalName, tool),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("%s failed to create issue for %s/%s: %v: %s", configSyncLogTag, dalName, tool, err, string(out))
	} else {
		log.Printf("%s created issue: %s", configSyncLogTag, title)
	}
}

// handleConfigSync is the API handler for POST /api/config-sync.
// Triggers an immediate config sync check.
func (d *Daemon) handleConfigSync(w http.ResponseWriter, r *http.Request) {
	templateDir := filepath.Join(d.serviceRepo, ".dal", "template")
	if _, err := os.Stat(templateDir); os.IsNotExist(err) {
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"error":  "template directory not found",
		})
		return
	}

	stateFile := filepath.Join(stateDir(d.serviceRepo), "config_sync_state.json")
	state := loadConfigSyncState(stateFile)

	hash, exists, err := templateSyncableHash(templateDir)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	state.mu.Lock()
	changed := exists && hash != state.LastHash
	lastHash := state.LastHash
	state.mu.Unlock()

	diff := templateDiffSince(templateDir, lastHash)

	if changed {
		go d.configSyncCheck(templateDir, state, stateFile)
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status":       "ok",
		"changed":      changed,
		"current_hash": short(hash),
		"last_hash":    short(lastHash),
		"diff":         diff,
	})
}

// loadConfigSyncState reads persisted sync state from disk.
func loadConfigSyncState(path string) *configSyncState {
	state := &configSyncState{}
	data, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	json.Unmarshal(data, state)
	return state
}

// saveConfigSyncState writes sync state to disk.
func saveConfigSyncState(path string, state *configSyncState) {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		log.Printf("%s save state error: %v", configSyncLogTag, err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("%s write state error: %v", configSyncLogTag, err)
		return
	}
	os.Rename(tmp, path)
}

// configSyncEnabled returns true when the config sync watcher env var is set.
func configSyncEnabled() bool {
	return os.Getenv("DALCENTER_CONFIG_SYNC") == "1"
}
