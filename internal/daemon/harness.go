package daemon

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// ── Harness: 모든 task 실행 전후 강제 게이트 ──

// preTaskGate runs before every task execution.
// Returns error to block task. All dals must pass.
func (d *Daemon) preTaskGate(c *Container, tr *taskResult) error {
	log.Printf("[harness] pre-gate: dal=%s task=%s", c.DalName, truncateStr(tr.Task, 50))

	// Gate 1: credential 유효성
	if d.pipeline != nil && d.pipeline.configured() {
		// MM 연결 가능한지 간단 체크
	}

	// Gate 2: 동시 실행 제한 (이미 canAcceptTask에서 체크하지만 이중 안전)
	running := 0
	
	for _, t := range d.tasks.List() {
		if t.Dal == c.DalName && t.Status == "running" && t.ID != tr.ID {
			running++
		}
	}
	
	maxRunning := 2
	if c.Role == "leader" {
		maxRunning = 1
	}
	if running >= maxRunning {
		return fmt.Errorf("[harness] dal %q already has %d running tasks (max=%d)", c.DalName, running, maxRunning)
	}

	// Gate 3: task 크기 제한 (10KB 이상이면 경고)
	if len(tr.Task) > 10240 {
		log.Printf("[harness] WARNING: task for %s is %d bytes — consider splitting", c.DalName, len(tr.Task))
	}

	return nil
}

// postTaskGate runs after every task execution.
// Can rollback or flag issues.
func (d *Daemon) postTaskGate(c *Container, tr *taskResult) {
	log.Printf("[harness] post-gate: dal=%s status=%s duration=%v",
		c.DalName, tr.Status, time.Since(tr.StartedAt))

	// Gate 1: 실패 시 로깅 + escalation
	if tr.Status == "failed" {
		logRuleViolation("task-failed", fmt.Sprintf("dal=%s task=%s error=%s",
			c.DalName, truncateStr(tr.Task, 50), tr.Error))

		// 연속 실패 감지
		d.trackConsecutiveFailure(c.DalName)
	}

	// Gate 2: output에 위험 패턴 감지
	if tr.Output != "" {
		dangerPatterns := []string{
			"rm -rf /",
			"DROP TABLE",
			"format c:",
			"sudo rm",
		}
		for _, p := range dangerPatterns {
			if strings.Contains(strings.ToLower(tr.Output), strings.ToLower(p)) {
				logRuleViolation("dangerous-output", fmt.Sprintf("dal=%s pattern=%q", c.DalName, p))
			}
		}
	}

	// Gate 3: 실행 시간 경고
	duration := time.Since(tr.StartedAt)
	if duration > 15*time.Minute {
		log.Printf("[harness] WARNING: dal=%s task took %v", c.DalName, duration)
	}
}

// trackConsecutiveFailure tracks consecutive failures per dal.
var consecutiveFailures = make(map[string]int)

func (d *Daemon) trackConsecutiveFailure(dalName string) {
	consecutiveFailures[dalName]++
	count := consecutiveFailures[dalName]
	if count >= 3 {
		log.Printf("[harness] ALERT: dal %s has %d consecutive failures — consider investigation", dalName, count)
		d.postAlert(fmt.Sprintf(":warning: **harness**: dal `%s` has %d consecutive failures", dalName, count))
	}
}

func resetConsecutiveFailure(dalName string) {
	consecutiveFailures[dalName] = 0
}
