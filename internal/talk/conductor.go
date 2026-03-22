package talk

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"dalforge-hub/dalcenter/internal/bridge"
)

// ConductorConfig for the central orchestrator bot.
type ConductorConfig struct {
	URL        string
	BotToken   string
	ChannelID  string
	BotUsername string
	Agents     []AgentInfo
}

// AgentInfo describes a registered agent for the conductor.
type AgentInfo struct {
	Username string
	Role     string
}

// threadState tracks an active thread.
type threadState struct {
	rootID  string
	topic   string
	done    bool
	turns   int
}

// Conductor is the central orchestrator that routes messages to agents.
type Conductor struct {
	cfg       ConductorConfig
	br        bridge.Bridge
	executor  *Executor
	sanitizer *Sanitizer
	seen      map[string]bool
	threads   map[string]*threadState // rootID → state
	mu        sync.Mutex
	botUserID string
}

func NewConductor(cfg ConductorConfig) (*Conductor, error) {
	br := bridge.NewMattermostBridge(cfg.URL, cfg.BotToken, cfg.ChannelID, 2*time.Second)
	return &Conductor{
		cfg:       cfg,
		br:        br,
		executor:  NewExecutor("에이전트 오케스트레이터", ""),
		sanitizer: NewSanitizer(),
		seen:      make(map[string]bool),
		threads:   make(map[string]*threadState),
	}, nil
}

func (c *Conductor) Run(ctx context.Context) error {
	if err := c.br.Connect(); err != nil {
		return fmt.Errorf("bridge connect: %w", err)
	}
	defer c.br.Close()

	if mm, ok := c.br.(*bridge.MattermostBridge); ok {
		c.botUserID = mm.BotUserID
	}

	agentList := c.buildAgentList()
	log.Printf("[conductor] started, channel=%s, %d agents", c.cfg.ChannelID, len(c.cfg.Agents))

	for {
		select {
		case msg := <-c.br.Listen():
			if msg.From == c.botUserID {
				continue
			}
			if c.isAgentBot(msg.From) {
				continue
			}
			if c.isDuplicate(msg.ID) {
				continue
			}

			if msg.RootID == "" {
				// New root message → start a thread
				go c.startThread(ctx, msg, agentList)
			} else {
				// Reply in existing thread → re-route or close
				go c.handleThreadReply(ctx, msg, agentList)
			}

		case err := <-c.br.Errors():
			log.Printf("[conductor] bridge error: %v", err)

		case <-ctx.Done():
			return nil
		}
	}
}

func (c *Conductor) startThread(ctx context.Context, msg bridge.Message, agentList string) {
	log.Printf("[conductor] new topic: %s", truncate(msg.Content, 80))

	c.mu.Lock()
	c.threads[msg.ID] = &threadState{
		rootID: msg.ID,
		topic:  msg.Content,
	}
	c.mu.Unlock()

	response := c.askRoute(ctx, msg.Content, "", agentList)
	if response == "" {
		return
	}

	c.br.Send(bridge.Message{
		Channel: msg.Channel,
		Content: response,
		RootID:  msg.ID,
	})
}

func (c *Conductor) handleThreadReply(ctx context.Context, msg bridge.Message, agentList string) {
	c.mu.Lock()
	ts, exists := c.threads[msg.RootID]
	c.mu.Unlock()

	if !exists {
		// Thread we didn't start — ignore
		return
	}
	if ts.done {
		return
	}

	// Check if user wants to close
	lower := strings.ToLower(strings.TrimSpace(msg.Content))
	if lower == "끝" || lower == "완료" || lower == "done" || lower == "close" {
		c.closeThread(msg)
		return
	}

	ts.turns++
	log.Printf("[conductor] thread %s turn %d: %s", ts.rootID[:8], ts.turns, truncate(msg.Content, 80))

	response := c.askRoute(ctx, msg.Content, ts.topic, agentList)
	if response == "" {
		return
	}

	c.br.Send(bridge.Message{
		Channel: msg.Channel,
		Content: response,
		RootID:  msg.RootID,
	})
}

func (c *Conductor) closeThread(msg bridge.Message) {
	c.mu.Lock()
	if ts, ok := c.threads[msg.RootID]; ok {
		ts.done = true
	}
	c.mu.Unlock()

	log.Printf("[conductor] thread %s closed", msg.RootID[:8])

	c.br.Send(bridge.Message{
		Channel: msg.Channel,
		Content: "✅ done",
		RootID:  msg.RootID,
	})
}

func (c *Conductor) askRoute(ctx context.Context, message, threadTopic, agentList string) string {
	contextLine := ""
	if threadTopic != "" {
		contextLine = fmt.Sprintf("\n진행 중인 주제: %s\n", threadTopic)
	}

	prompt := fmt.Sprintf(`너는 에이전트 오케스트레이터야. 사용자의 메시지를 분석해서 적절한 에이전트에게 작업을 배분해.

등록된 에이전트:
%s
%s
사용자 메시지: %s

규칙:
- 적절한 에이전트를 @멘션으로 태그해서 지시해
- 여러 에이전트가 필요하면 각각 역할을 명시해
- 간결하게 지시해 (2-3문장)
- 에이전트가 없는 작업이면 직접 답변해
- 반드시 @username 형식으로 태그해`, agentList, contextLine, message)

	response, err := c.executor.Run(ctx, ModeAsk, prompt)
	if err != nil {
		log.Printf("[conductor] claude error: %v", err)
		return ""
	}
	response = c.sanitizer.Clean(response)
	log.Printf("[conductor] routing: %s", truncate(response, 120))
	return response
}

func (c *Conductor) buildAgentList() string {
	var parts []string
	for _, a := range c.cfg.Agents {
		parts = append(parts, fmt.Sprintf("- @%s: %s", a.Username, a.Role))
	}
	return strings.Join(parts, "\n")
}

func (c *Conductor) isAgentBot(userID string) bool {
	// Known bot user IDs from bridge
	// In production, this would query the serve registry
	if mm, ok := c.br.(*bridge.MattermostBridge); ok {
		_ = mm // future: check against registered bot IDs
	}
	return false
}

func (c *Conductor) isDuplicate(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen[id] {
		return true
	}
	c.seen[id] = true
	return false
}
