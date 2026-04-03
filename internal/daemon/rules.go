package daemon

import (
	"fmt"
	"time"
	"log"
	"os/exec"
	"strings"
)

// enforceWakeRules checks rules before allowing a dal to wake.
// Returns error if wake should be blocked.
func (d *Daemon) enforceWakeRules(name, role string) error {
	// Rule: leader-only-persistent
	if role != "leader" {
		return fmt.Errorf("dal %q (role=%s) cannot be woken as persistent — only leader can be persistent. Use POST /api/task with oneshot=true", name, role)
	}
	return nil
}

// enforceTaskRules checks rules before executing a task.
// Returns error if task should be blocked.
func (d *Daemon) enforceTaskRules(dalName string, isOneshot bool) error {
	// Rule: oneshot-for-members
	d.mu.RLock()
	c, exists := d.containers[dalName]
	d.mu.RUnlock()

	if exists && c.Role != "leader" && !isOneshot {
		return fmt.Errorf("dal %q is a member — must use oneshot=true", dalName)
	}
	return nil
}

// enforceGitRules checks git rules before auto-commit/push.
// Called from autoGitWorkflow.
func enforceGitRules(containerID, issueNumber string) error {
	// Rule: no-duplicate-pr
	if issueNumber != "" {
		out, err := exec.Command("docker", "exec", containerID,
			"gh", "pr", "list", "--state", "open", "--search", issueNumber,
			"--json", "number", "--jq", "length").CombinedOutput()
		if err == nil {
			count := strings.TrimSpace(string(out))
			if count != "0" && count != "" {
				return fmt.Errorf("PR already exists for issue #%s — push to existing branch instead of creating new PR", issueNumber)
			}
		}
	}
	return nil
}

// enforceChannelRules checks MM channel exists before wake.
// Returns error if channel/webhook is missing.
func (d *Daemon) enforceChannelRules(teamName string) error {
	if d.pipeline == nil || !d.pipeline.configured() || d.pipeline.mmToken == "" {
		// MM not configured — skip channel enforcement
		return nil
	}

	// Rule: channel-required-for-wake
	// Check channel exists via MM API
	// (lightweight check — just verify we can post)
	log.Printf("[rules] channel check for team %q — MM configured, check skipped (TODO: implement)", teamName)
	return nil
}

// logRuleViolation logs a rule violation for auditing.
func logRuleViolation(rule, detail string) {
	log.Printf("[rules] VIOLATION: %s — %s", rule, detail)
}

// ── 추가 강제 규칙 ──

// enforceAutoIssueLimit prevents excessive auto-created issues.
// Max 3 auto-created issues per 24h.
var autoIssueCount int
var autoIssueResetTime time.Time

func enforceAutoIssueLimit() error {
	now := time.Now()
	if now.Sub(autoIssueResetTime) > 24*time.Hour {
		autoIssueCount = 0
		autoIssueResetTime = now
	}
	if autoIssueCount >= 3 {
		return fmt.Errorf("auto issue limit reached (3/24h) — manual creation required")
	}
	autoIssueCount++
	log.Printf("[rules] auto issue %d/3 today", autoIssueCount)
	return nil
}

// enforceNoMainPush blocks direct push to main branch.
// Called from autoGitWorkflow before git push.
func enforceNoMainPush(branch string) error {
	if branch == "main" || branch == "master" {
		return fmt.Errorf("direct push to %s forbidden — use branch + PR", branch)
	}
	return nil
}

// enforceOpsWatcherRateLimit limits consecutive wake failures.
// After 3 consecutive failures for same team, stop retrying.
var opsWakeFailures = make(map[string]int)

func enforceOpsWakeLimit(teamName string, success bool) error {
	if success {
		opsWakeFailures[teamName] = 0
		return nil
	}
	opsWakeFailures[teamName]++
	if opsWakeFailures[teamName] >= 3 {
		return fmt.Errorf("team %q wake failed 3 consecutive times — alerting only, no more retries", teamName)
	}
	return nil
}

// enforceMinAutoInterval rejects auto_task intervals below minimum.
func enforceMinAutoInterval(interval time.Duration) error {
	minInterval := 1 * time.Hour
	if interval > 0 && interval < minInterval {
		return fmt.Errorf("auto_task interval %v is below minimum %v", interval, minInterval)
	}
	return nil
}
