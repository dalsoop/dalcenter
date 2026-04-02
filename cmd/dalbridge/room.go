package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// room represents a registered communication room (1:1 with MM channel).
type room struct {
	Name      string    `json:"name"`
	Channel   string    `json:"channel"`              // MM channel name
	Webhook   string    `json:"webhook,omitempty"`     // dal callback URL
	CreatedAt time.Time `json:"created_at"`
	broker    *broker   // per-room SSE broker (not serialized)
}

// roomRegistry manages room registrations in memory.
type roomRegistry struct {
	mu    sync.RWMutex
	rooms map[string]*room // keyed by room name
}

func newRoomRegistry() *roomRegistry {
	return &roomRegistry{rooms: make(map[string]*room)}
}

func (rr *roomRegistry) get(name string) (*room, bool) {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	r, ok := rr.rooms[name]
	return r, ok
}

func (rr *roomRegistry) list() []*room {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	out := make([]*room, 0, len(rr.rooms))
	for _, r := range rr.rooms {
		out = append(out, r)
	}
	return out
}

func (rr *roomRegistry) register(r *room) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	r.broker = newBroker()
	rr.rooms[r.Name] = r
}

// findByChannel returns all rooms mapped to the given MM channel name.
func (rr *roomRegistry) findByChannel(channel string) []*room {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	var out []*room
	for _, r := range rr.rooms {
		if r.Channel == channel {
			out = append(out, r)
		}
	}
	return out
}

func (rr *roomRegistry) remove(name string) bool {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if _, ok := rr.rooms[name]; !ok {
		return false
	}
	delete(rr.rooms, name)
	return true
}

// registerRoomHandlers registers /api/rooms endpoints on mux.
func registerRoomHandlers(mux *http.ServeMux, rr *roomRegistry, dt *deliveryTracker) {
	// POST /api/rooms — register a new room
	// GET  /api/rooms — list all rooms
	mux.HandleFunc("/api/rooms", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handleCreateRoom(w, r, rr)
		case http.MethodGet:
			handleListRooms(w, r, rr)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// /api/rooms/{name}[/action] — room CRUD + send + stream
	mux.HandleFunc("/api/rooms/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/rooms/")
		if rest == "" {
			http.Error(w, "room name required", http.StatusBadRequest)
			return
		}

		// parse: {name} or {name}/{action}
		name, action, _ := strings.Cut(rest, "/")

		switch action {
		case "":
			// /api/rooms/{name}
			switch r.Method {
			case http.MethodGet:
				handleGetRoom(w, r, rr, name)
			case http.MethodDelete:
				handleDeleteRoom(w, r, rr, name)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		case "send":
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			handleRoomSend(w, r, rr, dt, name)
		case "stream":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			handleRoomStream(w, r, rr, name)
		default:
			http.Error(w, "unknown action", http.StatusNotFound)
		}
	})
}

func handleCreateRoom(w http.ResponseWriter, r *http.Request, rr *roomRegistry) {
	var req struct {
		Name    string `json:"name"`
		Channel string `json:"channel"`
		Webhook string `json:"webhook"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, `"name" is required`, http.StatusBadRequest)
		return
	}
	if req.Channel == "" {
		req.Channel = req.Name // default: channel name = room name
	}

	if _, exists := rr.get(req.Name); exists {
		http.Error(w, fmt.Sprintf("room %q already exists", req.Name), http.StatusConflict)
		return
	}

	rm := &room{
		Name:      req.Name,
		Channel:   req.Channel,
		Webhook:   req.Webhook,
		CreatedAt: time.Now().UTC(),
	}
	rr.register(rm)

	log.Printf("[room] registered: %s (channel=%s, webhook=%s)", rm.Name, rm.Channel, rm.Webhook)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(rm)
}

func handleListRooms(w http.ResponseWriter, _ *http.Request, rr *roomRegistry) {
	rooms := rr.list()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rooms)
}

func handleGetRoom(w http.ResponseWriter, _ *http.Request, rr *roomRegistry, name string) {
	rm, ok := rr.get(name)
	if !ok {
		http.Error(w, fmt.Sprintf("room %q not found", name), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rm)
}

func handleDeleteRoom(w http.ResponseWriter, _ *http.Request, rr *roomRegistry, name string) {
	if !rr.remove(name) {
		http.Error(w, fmt.Sprintf("room %q not found", name), http.StatusNotFound)
		return
	}

	log.Printf("[room] unregistered: %s", name)

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","removed":%q}`, name)
}

// handleRoomSend sends a message to the room with delivery tracking.
func handleRoomSend(w http.ResponseWriter, r *http.Request, rr *roomRegistry, dt *deliveryTracker, name string) {
	rm, ok := rr.get(name)
	if !ok {
		http.Error(w, fmt.Sprintf("room %q not found", name), http.StatusNotFound)
		return
	}

	var req struct {
		Text     string `json:"text"`
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	d := dt.create(rm.Name, req.Text, req.Username)
	dt.deliver(rm, d)

	log.Printf("[room:%s] %s: %s (id=%s, %d subscribers)",
		rm.Name, req.Username, truncate(req.Text, 80), d.ID, rm.broker.clientCount())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":      "ok",
		"delivery_id": d.ID,
	})
}

// handleRoomStream serves an SSE stream scoped to a single room.
func handleRoomStream(w http.ResponseWriter, r *http.Request, rr *roomRegistry, name string) {
	rm, ok := rr.get(name)
	if !ok {
		http.Error(w, fmt.Sprintf("room %q not found", name), http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := rm.broker.subscribe()
	defer rm.broker.unsubscribe(ch)

	fmt.Fprintf(w, "data: {\"event\":\"api_connected\",\"room\":%q}\n\n", rm.Name)
	flusher.Flush()

	log.Printf("[room:%s] stream client connected (%d subscribers)", rm.Name, rm.broker.clientCount())
	defer func() {
		log.Printf("[room:%s] stream client disconnected (%d remaining)", rm.Name, rm.broker.clientCount())
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
