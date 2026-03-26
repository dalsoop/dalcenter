package daemon

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// botTokenStore persists Mattermost bot tokens across daemon restarts.
type botTokenStore struct {
	path   string
	tokens map[string]string // instanceName -> botToken
	mu     sync.RWMutex
}

func newBotTokenStore(team string) *botTokenStore {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".dalcenter")
	os.MkdirAll(dir, 0700)
	return &botTokenStore{
		path:   filepath.Join(dir, team+"-bot-tokens.json"),
		tokens: make(map[string]string),
	}
}

// Load reads bot tokens from disk.
func (s *botTokenStore) Load() {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return // file doesn't exist yet
	}
	if err := json.Unmarshal(data, &s.tokens); err != nil {
		log.Printf("[bot-tokens] failed to parse %s: %v", s.path, err)
		s.tokens = make(map[string]string)
	}
}

// Get returns the bot token for an instance.
func (s *botTokenStore) Get(instanceName string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tokens[instanceName]
}

// Set stores a bot token and persists to disk.
func (s *botTokenStore) Set(instanceName, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[instanceName] = token
	data, err := json.MarshalIndent(s.tokens, "", "  ")
	if err != nil {
		log.Printf("[bot-tokens] marshal error: %v", err)
		return
	}
	if err := os.WriteFile(s.path, data, 0600); err != nil {
		log.Printf("[bot-tokens] write error: %v", err)
	}
}
