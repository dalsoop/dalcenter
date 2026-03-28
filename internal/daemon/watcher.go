package daemon

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	peerCheckInterval = 30 * time.Second
	peerCheckTimeout  = 10 * time.Second
	peerFailThreshold = 3
)

// startPeerWatcher periodically checks the peer dalcenter's health endpoint.
// For secondary instances, it also handles automatic promotion when the
// primary becomes unreachable (after peerFailThreshold consecutive failures).
func (d *Daemon) startPeerWatcher(ctx context.Context) {
	peerURL := os.Getenv("DALCENTER_PEER_URL")
	if peerURL == "" {
		log.Printf("[peer-watcher] DALCENTER_PEER_URL not set — skipping")
		return
	}
	peerURL = strings.TrimRight(peerURL, "/")

	log.Printf("[peer-watcher] started (peer=%s, role=%s, interval=%s, threshold=%d)",
		peerURL, d.haRole, peerCheckInterval, peerFailThreshold)

	client := &http.Client{Timeout: peerCheckTimeout}
	healthURL := peerURL + "/api/health"

	ticker := time.NewTicker(peerCheckInterval)
	defer ticker.Stop()

	var consecutiveFails int
	var alerted bool

	for {
		select {
		case <-ctx.Done():
			log.Printf("[peer-watcher] stopped")
			return
		case <-ticker.C:
			if err := checkPeerHealth(client, healthURL); err != nil {
				consecutiveFails++
				log.Printf("[peer-watcher] health check failed (%d/%d): %v",
					consecutiveFails, peerFailThreshold, err)

				if consecutiveFails >= peerFailThreshold && !alerted {
					d.notifyPeerDown(peerURL, err)
					alerted = true

					// Secondary promotes itself when primary is confirmed down
					if d.haRole == HARoleSecondary && !d.promoted {
						d.promote(peerURL)
					}
				}
			} else {
				if alerted {
					log.Printf("[peer-watcher] peer recovered after alert")
					d.notifyPeerRecovered(peerURL)

					// Demote back to secondary if primary has recovered
					if d.haRole == HARoleSecondary && d.promoted {
						d.demote(peerURL)
					}
				}
				consecutiveFails = 0
				alerted = false
			}
		}
	}
}

// promote transitions a secondary instance to act as primary.
// It begins accepting write operations and managing dal lifecycles.
func (d *Daemon) promote(peerURL string) {
	d.promoted = true
	msg := fmt.Sprintf(":rotating_light: **dalcenter secondary promoted** — primary `%s` is down, this instance is now handling dal management", peerURL)
	d.postAlert(msg)
	log.Printf("[peer-watcher] PROMOTED to active (primary %s unreachable)", peerURL)
}

// demote transitions a promoted secondary back to standby.
// The recovered primary resumes dal management.
func (d *Daemon) demote(peerURL string) {
	d.promoted = false
	msg := fmt.Sprintf(":white_check_mark: **dalcenter secondary demoted** — primary `%s` recovered, resuming standby", peerURL)
	d.postAlert(msg)
	log.Printf("[peer-watcher] DEMOTED back to standby (primary %s recovered)", peerURL)
}

// IsActive returns true if this instance should actively manage dals.
// Primary is always active. Secondary is active only when promoted.
// Standalone (no HA) is always active.
func (d *Daemon) IsActive() bool {
	switch d.haRole {
	case HARolePrimary:
		return true
	case HARoleSecondary:
		return d.promoted
	default:
		return true // standalone
	}
}

// checkPeerHealth sends a GET to the peer's /api/health and expects 200.
func checkPeerHealth(client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// notifyPeerDown posts a Mattermost alert about peer failure.
func (d *Daemon) notifyPeerDown(peerURL string, lastErr error) {
	msg := fmt.Sprintf(":warning: **dalcenter peer down** — `%s` failed %d consecutive health checks. Last error: %s",
		peerURL, peerFailThreshold, lastErr)
	d.postAlert(msg)
	log.Printf("[peer-watcher] alert sent: peer %s down", peerURL)
}

// notifyPeerRecovered posts a Mattermost recovery notice.
func (d *Daemon) notifyPeerRecovered(peerURL string) {
	msg := fmt.Sprintf(":white_check_mark: **dalcenter peer recovered** — `%s` is healthy again", peerURL)
	d.postAlert(msg)
	log.Printf("[peer-watcher] recovery notice sent: peer %s up", peerURL)
}

// postAlert sends a message to the project's Mattermost channel.
func (d *Daemon) postAlert(message string) {
	if d.mm == nil || d.mm.URL == "" || d.channelID == "" {
		log.Printf("[peer-watcher] mattermost not configured — alert logged only: %s", message)
		return
	}
	body := fmt.Sprintf(`{"channel_id":%q,"message":%q}`, d.channelID, message)
	if _, err := mmPost(d.mm.URL, d.mm.AdminToken, "/api/v4/posts", body); err != nil {
		log.Printf("[peer-watcher] failed to post alert: %v", err)
	}
}
