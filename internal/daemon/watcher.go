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
	peerCheckInterval  = 30 * time.Second
	peerCheckTimeout   = 10 * time.Second
	peerFailThreshold  = 3
)

// startPeerWatcher periodically checks the peer dalcenter's health endpoint.
// After peerFailThreshold consecutive failures, it sends a bridge alert.
func (d *Daemon) startPeerWatcher(ctx context.Context) {
	peerURL := os.Getenv("DALCENTER_PEER_URL")
	if peerURL == "" {
		log.Printf("[peer-watcher] DALCENTER_PEER_URL not set — skipping")
		return
	}
	peerURL = strings.TrimRight(peerURL, "/")

	log.Printf("[peer-watcher] started (peer=%s, interval=%s, threshold=%d)",
		peerURL, peerCheckInterval, peerFailThreshold)

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
				}
			} else {
				if alerted {
					log.Printf("[peer-watcher] peer recovered after alert")
					d.notifyPeerRecovered(peerURL)
				}
				consecutiveFails = 0
				alerted = false
			}
		}
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

// notifyPeerDown posts a bridge alert about peer failure.
func (d *Daemon) notifyPeerDown(peerURL string, lastErr error) {
	msg := fmt.Sprintf(":warning: **dalcenter peer down** — `%s` failed %d consecutive health checks. Last error: %s",
		peerURL, peerFailThreshold, lastErr)
	d.postAlert(msg)
	log.Printf("[peer-watcher] alert sent: peer %s down", peerURL)
}

// notifyPeerRecovered posts a bridge recovery notice.
func (d *Daemon) notifyPeerRecovered(peerURL string) {
	msg := fmt.Sprintf(":white_check_mark: **dalcenter peer recovered** — `%s` is healthy again", peerURL)
	d.postAlert(msg)
	log.Printf("[peer-watcher] recovery notice sent: peer %s up", peerURL)
}

// postAlert sends a message via matterbridge.
func (d *Daemon) postAlert(message string) {
	if d.bridgeURL == "" {
		log.Printf("[peer-watcher] bridge not configured — alert logged only: %s", message)
		return
	}
	if err := d.bridgePost(message, "dalcenter"); err != nil {
		log.Printf("[peer-watcher] failed to post alert: %v", err)
	}
}
