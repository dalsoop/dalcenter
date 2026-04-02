package daemon

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DalrootPipeline manages dalroot ↔ MM channel communication.
// Each dalroot pane maps 1:1 to an MM channel (dalroot-{pane-id}).
// MM is the message bus — no room.go/delivery.go needed.
type DalrootPipeline struct {
	mmURL    string            // MM API base URL
	mmToken  string            // MM bot token
	mmTeam   string            // MM team name
	channels map[string]string // pane-id → channel-id
	mu       sync.RWMutex
	filePath string // persistence file
}

// PipelineChannel holds the mapping between a pane and its MM channel.
type PipelineChannel struct {
	PaneID      string    `json:"pane_id"`
	ChannelID   string    `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// PipelineMessage is a message received from MM.
type PipelineMessage struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	Username  string `json:"username"`
	CreatedAt int64  `json:"create_at"`
}

// PipelineHealth reports pipeline component status.
type PipelineHealth struct {
	MM       string `json:"mm"`
	Channels int    `json:"channels"`
	Error    string `json:"error,omitempty"`
}

// newDalrootPipeline creates a pipeline from environment.
func newDalrootPipeline(serviceRepo string) *DalrootPipeline {
	mmURL := os.Getenv("DALCENTER_MM_URL")
	mmToken := os.Getenv("DALCENTER_MM_TOKEN")
	mmTeam := os.Getenv("DALCENTER_MM_TEAM")
	if mmTeam == "" {
		mmTeam = "dalsoop"
	}

	p := &DalrootPipeline{
		mmURL:    strings.TrimRight(mmURL, "/"),
		mmToken:  mmToken,
		mmTeam:   mmTeam,
		channels: make(map[string]string),
		filePath: filepath.Join(stateDir(serviceRepo), "pipeline_channels.json"),
	}
	p.load()
	return p
}

// configured reports whether MM credentials are present.
func (p *DalrootPipeline) configured() bool {
	return p.mmURL != "" && p.mmToken != ""
}

// Init creates an MM channel for a pane and registers the mapping.
func (p *DalrootPipeline) Init(paneID string) (*PipelineChannel, error) {
	if !p.configured() {
		return nil, fmt.Errorf("MM not configured (DALCENTER_MM_URL/DALCENTER_MM_TOKEN)")
	}

	channelName := "dalroot-" + paneID

	// Resolve team ID
	teamID, err := p.getTeamID()
	if err != nil {
		return nil, fmt.Errorf("resolve team: %w", err)
	}

	// Try to get existing channel first
	channelID, err := p.getChannelIDByName(teamID, channelName)
	if err != nil {
		// Create new channel
		channelID, err = p.createChannel(teamID, channelName, fmt.Sprintf("dalroot pane %s pipeline channel", paneID))
		if err != nil {
			return nil, fmt.Errorf("create channel %s: %w", channelName, err)
		}
		log.Printf("[pipeline] created channel %s for pane %s", channelName, paneID)
	} else {
		log.Printf("[pipeline] using existing channel %s for pane %s", channelName, paneID)
	}

	p.mu.Lock()
	p.channels[paneID] = channelID
	p.mu.Unlock()
	p.persist()

	return &PipelineChannel{
		PaneID:      paneID,
		ChannelID:   channelID,
		ChannelName: channelName,
		CreatedAt:   time.Now(),
	}, nil
}

// Send posts a message to the pane's MM channel.
func (p *DalrootPipeline) Send(paneID, message string) error {
	channelID, err := p.resolveChannel(paneID)
	if err != nil {
		return err
	}
	return p.postToChannel(channelID, message)
}

// Receive fetches unread messages from the pane's MM channel.
func (p *DalrootPipeline) Receive(paneID string) ([]PipelineMessage, error) {
	channelID, err := p.resolveChannel(paneID)
	if err != nil {
		return nil, err
	}
	return p.getUnread(channelID)
}

// Broadcast sends a message to all registered pane channels.
func (p *DalrootPipeline) Broadcast(message string) error {
	p.mu.RLock()
	channels := make(map[string]string, len(p.channels))
	for k, v := range p.channels {
		channels[k] = v
	}
	p.mu.RUnlock()

	var errs []string
	for pane, chID := range channels {
		if err := p.postToChannel(chID, message); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", pane, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("broadcast partial failure: %s", strings.Join(errs, "; "))
	}
	return nil
}

// Health checks pipeline status.
func (p *DalrootPipeline) Health() PipelineHealth {
	h := PipelineHealth{
		Channels: p.channelCount(),
	}
	if !p.configured() {
		h.MM = "not_configured"
		h.Error = "DALCENTER_MM_URL or DALCENTER_MM_TOKEN not set"
		return h
	}
	// Ping MM
	if err := p.pingMM(); err != nil {
		h.MM = "unreachable"
		h.Error = err.Error()
	} else {
		h.MM = "ok"
	}
	return h
}

// List returns all pane→channel mappings.
func (p *DalrootPipeline) List() []PipelineChannel {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var result []PipelineChannel
	for pane, chID := range p.channels {
		result = append(result, PipelineChannel{
			PaneID:      pane,
			ChannelID:   chID,
			ChannelName: "dalroot-" + pane,
		})
	}
	return result
}

// Sync performs the UserPromptSubmit hook: send user message to MM + fetch unread.
func (p *DalrootPipeline) Sync(paneID, userMessage string) ([]PipelineMessage, error) {
	if !p.configured() {
		return nil, fmt.Errorf("MM not configured")
	}

	channelID, err := p.resolveChannel(paneID)
	if err != nil {
		// Auto-init if not registered
		ch, initErr := p.Init(paneID)
		if initErr != nil {
			return nil, fmt.Errorf("auto-init failed: %w", initErr)
		}
		channelID = ch.ChannelID
	}

	// Post user message to MM (dalroot-log replacement)
	if userMessage != "" {
		if err := p.postToChannel(channelID, userMessage); err != nil {
			log.Printf("[pipeline] sync send failed for %s: %v", paneID, err)
		}
	}

	// Fetch unread (listener replacement)
	msgs, err := p.getUnread(channelID)
	if err != nil {
		return nil, fmt.Errorf("receive: %w", err)
	}

	return msgs, nil
}

// --- MM API helpers ---

func (p *DalrootPipeline) mmRequest(method, path string, body string) (*http.Response, error) {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, p.mmURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.mmToken)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return (&http.Client{Timeout: 10 * time.Second}).Do(req)
}

func (p *DalrootPipeline) getTeamID() (string, error) {
	resp, err := p.mmRequest("GET", "/api/v4/teams/name/"+p.mmTeam, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("team lookup %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	if result.ID == "" {
		return "", fmt.Errorf("team %q not found", p.mmTeam)
	}
	return result.ID, nil
}

func (p *DalrootPipeline) getChannelIDByName(teamID, name string) (string, error) {
	resp, err := p.mmRequest("GET", fmt.Sprintf("/api/v4/teams/%s/channels/name/%s", teamID, name), "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("channel %q not found", name)
	}
	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.ID, nil
}

func (p *DalrootPipeline) createChannel(teamID, name, purpose string) (string, error) {
	body := fmt.Sprintf(`{"team_id":%q,"name":%q,"display_name":%q,"purpose":%q,"type":"O"}`,
		teamID, name, name, purpose)
	resp, err := p.mmRequest("POST", "/api/v4/channels", body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create channel %d: %s", resp.StatusCode, string(b))
	}
	var result struct {
		ID string `json:"id"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.ID, nil
}

func (p *DalrootPipeline) postToChannel(channelID, message string) error {
	body := fmt.Sprintf(`{"channel_id":%q,"message":%q}`, channelID, message)
	resp, err := p.mmRequest("POST", "/api/v4/posts", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("post %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (p *DalrootPipeline) getUnread(channelID string) ([]PipelineMessage, error) {
	// Get the bot's user ID to use "since" endpoint
	// Use channel posts endpoint with since=0 and per_page limit
	// For simplicity, get last 10 posts
	resp, err := p.mmRequest("GET", fmt.Sprintf("/api/v4/channels/%s/posts?per_page=10", channelID), "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get posts %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Order []string                  `json:"order"`
		Posts map[string]json.RawMessage `json:"posts"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	var msgs []PipelineMessage
	for _, id := range result.Order {
		raw, ok := result.Posts[id]
		if !ok {
			continue
		}
		var post struct {
			ID       string `json:"id"`
			Message  string `json:"message"`
			UserID   string `json:"user_id"`
			CreateAt int64  `json:"create_at"`
		}
		json.Unmarshal(raw, &post)
		// Resolve username
		username := p.resolveUsername(post.UserID)
		msgs = append(msgs, PipelineMessage{
			ID:        post.ID,
			Message:   post.Message,
			Username:  username,
			CreatedAt: post.CreateAt,
		})
	}
	return msgs, nil
}

func (p *DalrootPipeline) resolveUsername(userID string) string {
	resp, err := p.mmRequest("GET", "/api/v4/users/"+userID, "")
	if err != nil {
		return userID
	}
	defer resp.Body.Close()
	var u struct {
		Username string `json:"username"`
	}
	json.NewDecoder(resp.Body).Decode(&u)
	if u.Username != "" {
		return u.Username
	}
	return userID
}

func (p *DalrootPipeline) pingMM() error {
	resp, err := p.mmRequest("GET", "/api/v4/system/ping", "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("mm ping %d", resp.StatusCode)
	}
	return nil
}

func (p *DalrootPipeline) resolveChannel(paneID string) (string, error) {
	p.mu.RLock()
	chID, ok := p.channels[paneID]
	p.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("pane %q not registered — run pipeline init first", paneID)
	}
	return chID, nil
}

func (p *DalrootPipeline) channelCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.channels)
}

// --- Channel CRUD (named channels, not pane-bound) ---

// ChannelInfo describes a Mattermost channel.
type ChannelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Purpose     string `json:"purpose"`
	Type        string `json:"type"`
}

// CreateNamedChannel creates an MM channel with the given name.
func (p *DalrootPipeline) CreateNamedChannel(name, purpose string) (*ChannelInfo, error) {
	if !p.configured() {
		return nil, fmt.Errorf("MM not configured (DALCENTER_MM_URL/DALCENTER_MM_TOKEN)")
	}

	teamID, err := p.getTeamID()
	if err != nil {
		return nil, fmt.Errorf("resolve team: %w", err)
	}

	// Check if already exists
	if _, err := p.getChannelIDByName(teamID, name); err == nil {
		return nil, fmt.Errorf("channel %q already exists", name)
	}

	if purpose == "" {
		purpose = fmt.Sprintf("channel %s managed by dalcenter", name)
	}

	body := fmt.Sprintf(`{"team_id":%q,"name":%q,"display_name":%q,"purpose":%q,"type":"O"}`,
		teamID, name, name, purpose)
	resp, err := p.mmRequest("POST", "/api/v4/channels", body)
	if err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create channel %d: %s", resp.StatusCode, string(b))
	}

	var ch ChannelInfo
	json.NewDecoder(resp.Body).Decode(&ch)
	log.Printf("[channel] created %s (id=%s)", name, ch.ID)
	return &ch, nil
}

// DeleteNamedChannel deletes an MM channel by name.
func (p *DalrootPipeline) DeleteNamedChannel(name string) error {
	if !p.configured() {
		return fmt.Errorf("MM not configured (DALCENTER_MM_URL/DALCENTER_MM_TOKEN)")
	}

	teamID, err := p.getTeamID()
	if err != nil {
		return fmt.Errorf("resolve team: %w", err)
	}

	channelID, err := p.getChannelIDByName(teamID, name)
	if err != nil {
		return fmt.Errorf("channel %q not found", name)
	}

	resp, err := p.mmRequest("DELETE", fmt.Sprintf("/api/v4/channels/%s", channelID), "")
	if err != nil {
		return fmt.Errorf("delete channel: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete channel %d: %s", resp.StatusCode, string(b))
	}

	log.Printf("[channel] deleted %s (id=%s)", name, channelID)
	return nil
}

// ListTeamChannels lists all channels in the team.
func (p *DalrootPipeline) ListTeamChannels() ([]ChannelInfo, error) {
	if !p.configured() {
		return nil, fmt.Errorf("MM not configured (DALCENTER_MM_URL/DALCENTER_MM_TOKEN)")
	}

	teamID, err := p.getTeamID()
	if err != nil {
		return nil, fmt.Errorf("resolve team: %w", err)
	}

	var allChannels []ChannelInfo
	page := 0
	perPage := 100
	for {
		resp, err := p.mmRequest("GET", fmt.Sprintf("/api/v4/teams/%s/channels?page=%d&per_page=%d", teamID, page, perPage), "")
		if err != nil {
			return nil, fmt.Errorf("list channels: %w", err)
		}
		var batch []ChannelInfo
		json.NewDecoder(resp.Body).Decode(&batch)
		resp.Body.Close()
		if len(batch) == 0 {
			break
		}
		allChannels = append(allChannels, batch...)
		if len(batch) < perPage {
			break
		}
		page++
	}

	return allChannels, nil
}

// --- persistence ---

func (p *DalrootPipeline) persist() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	b, _ := json.MarshalIndent(p.channels, "", "  ")
	tmp := p.filePath + ".tmp"
	os.WriteFile(tmp, b, 0o644)
	os.Rename(tmp, p.filePath)
}

func (p *DalrootPipeline) load() {
	data, err := os.ReadFile(p.filePath)
	if err != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	json.Unmarshal(data, &p.channels)
}
