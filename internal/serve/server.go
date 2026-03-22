package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// AgentEntry represents a registered agent in the registry.
type AgentEntry struct {
	Name         string    `json:"name"`
	IP           string    `json:"ip"`
	Port         int       `json:"port"`
	Role         string    `json:"role"`
	Status       string    `json:"status"` // "online", "offline"
	RegisteredAt time.Time `json:"registered_at"`
	LastSeen     time.Time `json:"last_seen"`
}

// Registry holds registered agents.
type Registry struct {
	agents map[string]AgentEntry
	mu     sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]AgentEntry)}
}

func (r *Registry) Register(entry AgentEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry.RegisteredAt = time.Now()
	entry.LastSeen = time.Now()
	entry.Status = "online"
	r.agents[entry.Name] = entry
	log.Printf("[serve] registered agent: %s (%s:%d)", entry.Name, entry.IP, entry.Port)
}

func (r *Registry) Deregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, name)
	log.Printf("[serve] deregistered agent: %s", name)
}

func (r *Registry) Get(name string) (AgentEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.agents[name]
	return e, ok
}

func (r *Registry) List() []AgentEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []AgentEntry
	for _, e := range r.agents {
		list = append(list, e)
	}
	return list
}

// Server is the dalcenter serve API server.
type Server struct {
	registry *Registry
	port     int
}

func NewServer(port int) *Server {
	return &Server{
		registry: NewRegistry(),
		port:     port,
	}
}

// Run starts the API server and blocks until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/agents", s.handleList)
	mux.HandleFunc("GET /api/agents/{name}", s.handleGet)
	mux.HandleFunc("POST /api/agents/register", s.handleRegister)
	mux.HandleFunc("DELETE /api/agents/{name}", s.handleDeregister)
	mux.HandleFunc("POST /api/agents/{name}/hook", s.handleProxy)

	srv := &http.Server{Addr: fmt.Sprintf(":%d", s.port), Handler: mux}
	log.Printf("[serve] listening on :%d", s.port)

	go func() {
		<-ctx.Done()
		srv.Close()
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.registry.List())
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entry, ok := s.registry.Get(name)
	if !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", name), 404)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entry)
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var entry AgentEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	if entry.Name == "" {
		http.Error(w, "name required", 400)
		return
	}
	s.registry.Register(entry)
	w.WriteHeader(201)
	json.NewEncoder(w).Encode(entry)
}

func (s *Server) handleDeregister(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.registry.Deregister(name)
	w.WriteHeader(204)
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	entry, ok := s.registry.Get(name)
	if !ok {
		http.Error(w, fmt.Sprintf("agent %q not found", name), 404)
		return
	}

	// Forward request to agent's hook server
	targetURL := fmt.Sprintf("http://%s:%d/hook", entry.IP, entry.Port)
	proxyReq, _ := http.NewRequestWithContext(r.Context(), "POST", targetURL, r.Body)
	proxyReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy to %s failed: %v", name, err), 502)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}
