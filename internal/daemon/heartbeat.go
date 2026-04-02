package daemon

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

const (
	heartbeatInterval = 5 * time.Minute
	heartbeatTimeout  = 30 * time.Second
)

// heartbeatMessage is the JSON payload sent to the leader container's stdin.
// The "type" field allows the leader's Claude Code process to identify and
// handle (or ignore) heartbeat pings without disrupting normal task flow.
const heartbeatMessage = `{"type":"heartbeat","ts":"%s"}`

// startHeartbeat periodically sends a lightweight stdin ping to the leader
// container to prevent Claude Code's 300-second idle timeout.
func (d *Daemon) startHeartbeat(ctx context.Context) {
	log.Printf("[heartbeat] started (interval=%s)", heartbeatInterval)

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[heartbeat] stopped")
			return
		case <-ticker.C:
			d.sendHeartbeat(ctx)
		}
	}
}

// sendHeartbeat finds the leader container and sends a heartbeat ping via
// docker exec stdin. It also updates LastSeenAt to keep idle tracking fresh.
func (d *Daemon) sendHeartbeat(ctx context.Context) {
	leaderName, containerID := d.findLeader()
	if leaderName == "" {
		return
	}

	msg := fmt.Sprintf(heartbeatMessage, time.Now().UTC().Format(time.RFC3339))

	execCtx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "docker", "exec", "-i", containerID,
		"bash", "-c", "cat > /dev/null")
	cmd.Stdin = strings.NewReader(msg)

	if err := cmd.Run(); err != nil {
		log.Printf("[heartbeat] ping to %s failed: %v", leaderName, err)
		return
	}

	d.markActivity(leaderName, time.Now().UTC())
	log.Printf("[heartbeat] pinged %s", leaderName)
}
