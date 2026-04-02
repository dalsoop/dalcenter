package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T) (*broker, *http.ServeMux) {
	t.Helper()
	b := newBroker()
	mux := http.NewServeMux()

	// Reuse the same handler logic as main() but without os.Getenv.
	webhookToken := ""

	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload webhookPayload
		ct := r.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(ct, "application/json"):
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
		default:
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid form", http.StatusBadRequest)
				return
			}
			payload.Token = r.FormValue("token")
			payload.ChannelName = r.FormValue("channel_name")
			payload.UserName = r.FormValue("user_name")
			payload.Text = r.FormValue("text")
			payload.PostID = r.FormValue("post_id")
		}

		if webhookToken != "" && payload.Token != webhookToken {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		msg := streamMessage{
			Text:     payload.Text,
			Username: payload.UserName,
			Channel:  payload.ChannelName,
			PostID:   payload.PostID,
		}
		data, _ := json.Marshal(msg)
		b.broadcast(data)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
	})

	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")

		ch := b.subscribe()
		defer b.unsubscribe(ch)

		w.Write([]byte("data: {\"event\":\"api_connected\"}\n\n"))
		flusher.Flush()

		// Send one message from the channel then return.
		data, ok := <-ch
		if ok {
			w.Write([]byte("data: "))
			w.Write(data)
			w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	})

	return b, mux
}

func TestWebhookJSON(t *testing.T) {
	b, mux := setupTestServer(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ch := b.subscribe()
	defer b.unsubscribe(ch)

	body := `{"token":"t","channel_name":"town-square","user_name":"alice","text":"hello @dalroot","post_id":"abc123"}`
	resp, err := http.Post(ts.URL+"/webhook", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	data := <-ch
	var msg streamMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello @dalroot" {
		t.Errorf("text = %q, want %q", msg.Text, "hello @dalroot")
	}
	if msg.Username != "alice" {
		t.Errorf("username = %q, want %q", msg.Username, "alice")
	}
	if msg.Channel != "town-square" {
		t.Errorf("channel = %q, want %q", msg.Channel, "town-square")
	}
}

func TestWebhookFormEncoded(t *testing.T) {
	b, mux := setupTestServer(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	ch := b.subscribe()
	defer b.unsubscribe(ch)

	form := url.Values{
		"token":        {"t"},
		"channel_name": {"town-square"},
		"user_name":    {"bob"},
		"text":         {"hey @dalroot-1-1-1"},
		"post_id":      {"xyz789"},
	}
	resp, err := http.PostForm(ts.URL+"/webhook", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	data := <-ch
	var msg streamMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hey @dalroot-1-1-1" {
		t.Errorf("text = %q, want %q", msg.Text, "hey @dalroot-1-1-1")
	}
	if msg.Username != "bob" {
		t.Errorf("username = %q, want %q", msg.Username, "bob")
	}
	if msg.Channel != "town-square" {
		t.Errorf("channel = %q, want %q", msg.Channel, "town-square")
	}
}

func TestStreamSSEFormat(t *testing.T) {
	b, mux := setupTestServer(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Pre-inject a message so /stream has something to return after connected.
	go func() {
		msg := streamMessage{Text: "test message", Username: "alice", Channel: "ch"}
		data, _ := json.Marshal(msg)
		// Wait for a subscriber, then broadcast.
		for b.clientCount() == 0 {
		}
		b.broadcast(data)
	}()

	resp, err := http.Get(ts.URL + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
		if len(lines) >= 2 {
			break
		}
	}

	if len(lines) < 2 {
		t.Fatalf("expected at least 2 SSE data lines, got %d", len(lines))
	}

	// First line: connection event with data: prefix
	if !strings.HasPrefix(lines[0], "data: ") {
		t.Errorf("line[0] = %q, want data: prefix", lines[0])
	}
	connJSON := strings.TrimPrefix(lines[0], "data: ")
	var connEvt struct{ Event string `json:"event"` }
	if err := json.Unmarshal([]byte(connJSON), &connEvt); err != nil {
		t.Fatalf("parse connected event: %v", err)
	}
	if connEvt.Event != "api_connected" {
		t.Errorf("event = %q, want api_connected", connEvt.Event)
	}

	// Second line: message with data: prefix
	if !strings.HasPrefix(lines[1], "data: ") {
		t.Errorf("line[1] = %q, want data: prefix", lines[1])
	}
	msgJSON := strings.TrimPrefix(lines[1], "data: ")
	var msg streamMessage
	if err := json.Unmarshal([]byte(msgJSON), &msg); err != nil {
		t.Fatalf("parse message: %v", err)
	}
	if msg.Text != "test message" {
		t.Errorf("text = %q, want %q", msg.Text, "test message")
	}
}
