package daemon

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

const heartbeatInterval = 5 * time.Minute

// heartbeatPayload is the JSON message sent to the leader container.
type heartbeatPayload struct {
	Type      string   `json:"type"`
	Team      string   `json:"team"`
	Hostname  string   `json:"hostname"`
	DalCount  int      `json:"dal_count"`
	DalNames  []string `json:"dal_names"`
	Uptime    string   `json:"uptime"`
	Timestamp string   `json:"timestamp"`
}

// startHeartbeat sends a periodic heartbeat to the leader container.
// If no leader is registered, it silently skips until the next tick.
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

// sendHeartbeat sends a JSON heartbeat to the leader container via docker exec stdin.
func (d *Daemon) sendHeartbeat(ctx context.Context) {
	_, containerID := d.findLeader()
	if containerID == "" {
		return
	}

	hostname, _ := os.Hostname()

	d.mu.RLock()
	names := make([]string, 0, len(d.containers))
	for n := range d.containers {
		names = append(names, n)
	}
	d.mu.RUnlock()

	payload := heartbeatPayload{
		Type:      "heartbeat",
		Team:      os.Getenv("DALCENTER_TEAM"),
		Hostname:  hostname,
		DalCount:  len(names),
		DalNames:  names,
		Uptime:    time.Since(d.startTime).Truncate(time.Second).String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[heartbeat] marshal error: %v", err)
		return
	}

	cmd := exec.CommandContext(ctx,
		"docker", "exec", "-i", containerID,
		"cat", ">/dev/null",
	)
	cmd.Stdin = strings.NewReader(string(data) + "\n")

	if err := cmd.Run(); err != nil {
		log.Printf("[heartbeat] send failed (container=%s): %v", containerID[:12], err)
		return
	}

	log.Printf("[heartbeat] sent to leader (dals=%d, uptime=%s)", len(names), payload.Uptime)
}
