package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestContainsDalrootMention(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"hello @dalroot please check", true},
		{"@dalroot", true},
		{"@DALROOT help", true},
		{"@Dalroot 확인 부탁", true},
		{"hey @dalleader do this", false},
		{"no mention here", false},
		{"email dalroot@example.com", true}, // contains @dalroot substring
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := containsDalrootMention(tt.text)
			if got != tt.want {
				t.Errorf("containsDalrootMention(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestBridgeWatcherDetectsMention(t *testing.T) {
	// Create a fake bridge stream server that sends one @dalroot mention.
	mention := bridgeStreamMsg{
		Text:     "hey @dalroot check this PR",
		Username: "reviewer",
		Gateway:  "dal-team",
		ID:       "msg-1",
	}
	mentionJSON, _ := json.Marshal(mention)

	streamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stream" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(200)
			fmt.Fprintf(w, "%s\n", mentionJSON)
			// Close immediately after sending one message.
			return
		}
		w.WriteHeader(404)
	}))
	defer streamSrv.Close()

	// Track notification via DALCENTER_NOTIFY_URL.
	var notified bool
	var notifyPayload NotifyPayload
	var notifyMu sync.Mutex

	notifySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notifyMu.Lock()
		defer notifyMu.Unlock()
		notified = true
		json.NewDecoder(r.Body).Decode(&notifyPayload)
		w.WriteHeader(200)
	}))
	defer notifySrv.Close()

	t.Setenv("DALCENTER_NOTIFY_URL", notifySrv.URL)

	d := &Daemon{
		bridgeURL:   streamSrv.URL,
		serviceRepo: "/tmp/test-repo",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Run one iteration of bridge watch (it will return on stream close).
	var mu sync.Mutex
	lastNotify := time.Time{}
	_ = d.bridgeWatchOnce(ctx, &mu, &lastNotify)

	// Give the goroutine a moment to send the notification.
	time.Sleep(200 * time.Millisecond)

	notifyMu.Lock()
	defer notifyMu.Unlock()

	if !notified {
		t.Fatal("expected notification to be sent for @dalroot mention")
	}
	if notifyPayload.Event != "dalroot_mention" {
		t.Errorf("expected event=dalroot_mention, got %s", notifyPayload.Event)
	}
	if notifyPayload.Dal != "reviewer" {
		t.Errorf("expected dal=reviewer, got %s", notifyPayload.Dal)
	}
}

func TestBridgeWatcherCooldown(t *testing.T) {
	// Two mentions within cooldown — only first should notify.
	msg1 := bridgeStreamMsg{Text: "@dalroot first", Username: "a", ID: "1"}
	msg2 := bridgeStreamMsg{Text: "@dalroot second", Username: "b", ID: "2"}
	j1, _ := json.Marshal(msg1)
	j2, _ := json.Marshal(msg2)

	streamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stream" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(200)
			fmt.Fprintf(w, "%s\n%s\n", j1, j2)
			return
		}
		w.WriteHeader(404)
	}))
	defer streamSrv.Close()

	var notifyCount int
	var notifyMu sync.Mutex

	notifySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notifyMu.Lock()
		notifyCount++
		notifyMu.Unlock()
		w.WriteHeader(200)
	}))
	defer notifySrv.Close()

	t.Setenv("DALCENTER_NOTIFY_URL", notifySrv.URL)

	d := &Daemon{
		bridgeURL:   streamSrv.URL,
		serviceRepo: "/tmp/test-repo",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var mu sync.Mutex
	lastNotify := time.Time{}
	_ = d.bridgeWatchOnce(ctx, &mu, &lastNotify)

	time.Sleep(200 * time.Millisecond)

	notifyMu.Lock()
	defer notifyMu.Unlock()

	if notifyCount != 1 {
		t.Errorf("expected 1 notification (cooldown), got %d", notifyCount)
	}
}

func TestBridgeWatcherSkipsSelfMessages(t *testing.T) {
	// Messages from dalcenter should be skipped.
	msg := bridgeStreamMsg{Text: "@dalroot check", Username: "dalcenter", ID: "1"}
	j, _ := json.Marshal(msg)

	streamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stream" {
			w.WriteHeader(200)
			fmt.Fprintf(w, "%s\n", j)
			return
		}
		w.WriteHeader(404)
	}))
	defer streamSrv.Close()

	var notified bool
	var notifyMu sync.Mutex

	notifySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notifyMu.Lock()
		notified = true
		notifyMu.Unlock()
		w.WriteHeader(200)
	}))
	defer notifySrv.Close()

	t.Setenv("DALCENTER_NOTIFY_URL", notifySrv.URL)

	d := &Daemon{
		bridgeURL:   streamSrv.URL,
		serviceRepo: "/tmp/test-repo",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var mu sync.Mutex
	lastNotify := time.Time{}
	_ = d.bridgeWatchOnce(ctx, &mu, &lastNotify)

	time.Sleep(200 * time.Millisecond)

	notifyMu.Lock()
	defer notifyMu.Unlock()

	if notified {
		t.Error("should not notify for dalcenter's own messages")
	}
}

func TestBridgeWatcherNoBridgeURL(t *testing.T) {
	// Should return immediately when bridge URL is empty.
	d := &Daemon{bridgeURL: ""}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Should not block — just log and return.
	done := make(chan struct{})
	go func() {
		d.startBridgeWatcher(ctx)
		close(done)
	}()

	select {
	case <-done:
		// OK — returned quickly.
	case <-time.After(2 * time.Second):
		t.Fatal("startBridgeWatcher should return immediately when bridgeURL is empty")
	}
}
