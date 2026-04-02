package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// deliveryStatus represents the state of a message delivery.
type deliveryStatus string

const (
	statusPending   deliveryStatus = "pending"
	statusDelivered deliveryStatus = "delivered"
	statusFailed    deliveryStatus = "failed"
)

const maxRetries = 3

// delivery tracks the lifecycle of a single message delivery.
type delivery struct {
	ID        string         `json:"id"`
	Room      string         `json:"room"`
	Text      string         `json:"text"`
	Username  string         `json:"username"`
	Status    deliveryStatus `json:"status"`
	Attempts  int            `json:"attempts"`
	Error     string         `json:"error,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// deliveryTracker manages message delivery state and retries.
type deliveryTracker struct {
	mu        sync.RWMutex
	items     map[string]*delivery // keyed by delivery ID
	seq       atomic.Int64
	httpClient *http.Client
}

func newDeliveryTracker() *deliveryTracker {
	return &deliveryTracker{
		items: make(map[string]*delivery),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (dt *deliveryTracker) nextID() string {
	n := dt.seq.Add(1)
	return fmt.Sprintf("msg-%d-%d", time.Now().UnixMilli(), n)
}

func (dt *deliveryTracker) create(room, text, username string) *delivery {
	d := &delivery{
		ID:        dt.nextID(),
		Room:      room,
		Text:      text,
		Username:  username,
		Status:    statusPending,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	dt.mu.Lock()
	dt.items[d.ID] = d
	dt.mu.Unlock()
	return d
}

func (dt *deliveryTracker) get(id string) (*delivery, bool) {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	d, ok := dt.items[id]
	return d, ok
}

func (dt *deliveryTracker) list(room string) []*delivery {
	dt.mu.RLock()
	defer dt.mu.RUnlock()
	var out []*delivery
	for _, d := range dt.items {
		if room == "" || d.Room == room {
			out = append(out, d)
		}
	}
	return out
}

func (dt *deliveryTracker) markDelivered(id string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	if d, ok := dt.items[id]; ok {
		d.Status = statusDelivered
		d.UpdatedAt = time.Now().UTC()
	}
}

func (dt *deliveryTracker) markFailed(id, errMsg string) {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	if d, ok := dt.items[id]; ok {
		d.Status = statusFailed
		d.Error = errMsg
		d.UpdatedAt = time.Now().UTC()
	}
}

func (dt *deliveryTracker) incrementAttempt(id string) int {
	dt.mu.Lock()
	defer dt.mu.Unlock()
	if d, ok := dt.items[id]; ok {
		d.Attempts++
		d.UpdatedAt = time.Now().UTC()
		return d.Attempts
	}
	return 0
}

// deliver sends the message to the room's webhook URL with retry logic.
// It broadcasts to the room's SSE broker first, then attempts webhook delivery.
func (dt *deliveryTracker) deliver(rm *room, d *delivery) {
	// Always broadcast to room's SSE subscribers
	msg := streamMessage{
		Text:      d.Text,
		Username:  d.Username,
		Channel:   rm.Channel,
		Gateway:   rm.Name,
		PostID:    d.ID,
		Timestamp: d.CreatedAt.Format(time.RFC3339),
	}
	data, _ := json.Marshal(msg)
	rm.broker.broadcast(data)

	// If no webhook, mark delivered (SSE-only delivery)
	if rm.Webhook == "" {
		dt.markDelivered(d.ID)
		log.Printf("[delivery] %s → %s: SSE-only (no webhook)", d.ID, rm.Name)
		return
	}

	// Webhook delivery with retries
	go dt.deliverWebhook(rm, d)
}

func (dt *deliveryTracker) deliverWebhook(rm *room, d *delivery) {
	payload, _ := json.Marshal(map[string]string{
		"id":       d.ID,
		"text":     d.Text,
		"username": d.Username,
		"room":     rm.Name,
		"channel":  rm.Channel,
	})

	for {
		attempt := dt.incrementAttempt(d.ID)
		if attempt > maxRetries {
			dt.markFailed(d.ID, fmt.Sprintf("max retries (%d) exceeded", maxRetries))
			log.Printf("[delivery] %s → %s: FAILED after %d attempts", d.ID, rm.Name, maxRetries)
			return
		}

		resp, err := dt.httpClient.Post(rm.Webhook, "application/json", bytes.NewReader(payload))
		if err != nil {
			log.Printf("[delivery] %s → %s: attempt %d failed: %v", d.ID, rm.Name, attempt, err)
			time.Sleep(time.Duration(attempt) * time.Second) // linear backoff
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			dt.markDelivered(d.ID)
			log.Printf("[delivery] %s → %s: delivered (attempt %d)", d.ID, rm.Name, attempt)
			return
		}

		log.Printf("[delivery] %s → %s: attempt %d got status %d", d.ID, rm.Name, attempt, resp.StatusCode)
		time.Sleep(time.Duration(attempt) * time.Second)
	}
}

// registerDeliveryHandlers adds delivery status endpoints.
func registerDeliveryHandlers(mux *http.ServeMux, dt *deliveryTracker) {
	// GET /api/deliveries?room=<name> — list deliveries
	mux.HandleFunc("/api/deliveries", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		room := r.URL.Query().Get("room")
		items := dt.list(room)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})

	// GET /api/deliveries/{id} — get delivery status
	mux.HandleFunc("/api/deliveries/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Path[len("/api/deliveries/"):]
		if id == "" {
			http.Error(w, "delivery id required", http.StatusBadRequest)
			return
		}
		d, ok := dt.get(id)
		if !ok {
			http.Error(w, fmt.Sprintf("delivery %q not found", id), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(d)
	})
}
