package daemon

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	bridgeWatchReconnectMin = 2 * time.Second
	bridgeWatchReconnectMax = 60 * time.Second
	// dalrootMentionCooldown prevents duplicate notifications for rapid mentions.
	dalrootMentionCooldown = 30 * time.Second
)

// bridgeStreamMsg mirrors the matterbridge stream JSON format.
type bridgeStreamMsg struct {
	Text      string `json:"text"`
	Username  string `json:"username"`
	Gateway   string `json:"gateway"`
	ParentID  string `json:"parent_id"`
	Timestamp string `json:"timestamp"`
	ID        string `json:"id"`
	Event     string `json:"event"`
}

// startBridgeWatcher connects to the matterbridge stream and watches for
// @dalroot mentions. When detected, it calls notify-dalroot via HTTP
// (DALCENTER_NOTIFY_URL) and optionally the notify-dalroot CLI.
func (d *Daemon) startBridgeWatcher(ctx context.Context) {
	if d.bridgeURL == "" {
		log.Printf("[bridge-watcher] bridge URL not configured — skipping")
		return
	}

	log.Printf("[bridge-watcher] started (bridge=%s, cooldown=%s)", d.bridgeURL, dalrootMentionCooldown)

	var mu sync.Mutex
	lastNotify := time.Time{}

	consecutiveErrors := 0
	for {
		select {
		case <-ctx.Done():
			log.Printf("[bridge-watcher] stopped")
			return
		default:
		}

		err := d.bridgeWatchOnce(ctx, &mu, &lastNotify)
		if err != nil {
			consecutiveErrors++
			backoff := bridgeWatchReconnectMin * time.Duration(1<<uint(min(consecutiveErrors-1, 5)))
			if backoff > bridgeWatchReconnectMax {
				backoff = bridgeWatchReconnectMax
			}
			log.Printf("[bridge-watcher] stream error (%d): %v, retry in %v", consecutiveErrors, err, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}
			continue
		}
		consecutiveErrors = 0
	}
}

// bridgeWatchOnce opens a single streaming connection and scans for @dalroot mentions.
func (d *Daemon) bridgeWatchOnce(ctx context.Context, mu *sync.Mutex, lastNotify *time.Time) error {
	streamURL := d.bridgeURL + "/api/stream"
	req, err := http.NewRequestWithContext(ctx, "GET", streamURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg bridgeStreamMsg
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		// Skip connection events and dalcenter's own messages.
		if msg.Event == "api_connected" {
			continue
		}
		if msg.Username == "dalcenter" || msg.Username == "dalcenter-ops" {
			continue
		}

		if !containsDalrootMention(msg.Text) {
			continue
		}

		// Cooldown check to avoid notification spam.
		mu.Lock()
		if time.Since(*lastNotify) < dalrootMentionCooldown {
			mu.Unlock()
			log.Printf("[bridge-watcher] @dalroot mention by %s (cooldown, skipped)", msg.Username)
			continue
		}
		*lastNotify = time.Now()
		mu.Unlock()

		log.Printf("[bridge-watcher] @dalroot mention detected from %s: %s",
			msg.Username, truncateStr(msg.Text, 120))

		go d.notifyDalrootMention(msg.Username, msg.Text)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read: %w", err)
	}
	return fmt.Errorf("stream closed by server")
}

// containsDalrootMention checks if text contains an @dalroot mention.
func containsDalrootMention(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "@dalroot")
}

// notifyDalrootMention sends a notification when @dalroot is mentioned in the bridge.
func (d *Daemon) notifyDalrootMention(from, text string) {
	payload := NotifyPayload{
		Event:     "dalroot_mention",
		Dal:       from,
		Task:      truncateStr(text, 200),
		Status:    "mention",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	// 1. HTTP notification via DALCENTER_NOTIFY_URL
	if url := os.Getenv("DALCENTER_NOTIFY_URL"); url != "" {
		sendNotifyHTTP(url, payload)
	}

	// 2. CLI notification via notify-dalroot (if DALCENTER_CALLBACK_PANE is set)
	if pane := os.Getenv("DALCENTER_CALLBACK_PANE"); pane != "" {
		msg := fmt.Sprintf("[@dalroot mention by %s] %s", from, truncateStr(text, 100))
		cmd := exec.Command("notify-dalroot", d.serviceRepo, msg, pane)
		if err := cmd.Run(); err != nil {
			log.Printf("[bridge-watcher] notify-dalroot CLI failed: %v", err)
		}
	}

	dispatchWebhook(WebhookEvent{
		Event:     "dalroot_mention",
		Dal:       from,
		Task:      truncateStr(text, 200),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}
