package talk

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"dalforge-hub/dalcenter/internal/bridge"
)

// ConductorConfig for the central orchestrator bot.
type ConductorConfig struct {
	URL        string
	BotToken   string
	ChannelID  string
	BotUsername string
	Agents     []AgentInfo // registered agents
	HookPort   int
}

// AgentInfo describes a registered agent for the conductor.
type AgentInfo struct {
	Username string // MM bot username (e.g. "agent-200")
	Role     string // role description (e.g. "마케팅 전략가")
}

// Conductor is the central orchestrator that routes messages to agents.
type Conductor struct {
	cfg       ConductorConfig
	br        bridge.Bridge
	executor  *Executor
	sanitizer *Sanitizer
	seen      map[string]bool
}

func NewConductor(cfg ConductorConfig) (*Conductor, error) {
	br := bridge.NewMattermostBridge(cfg.URL, cfg.BotToken, cfg.ChannelID, 2*time.Second)
	return &Conductor{
		cfg:       cfg,
		br:        br,
		executor:  NewExecutor("에이전트 오케스트레이터"),
		sanitizer: NewSanitizer(),
		seen:      make(map[string]bool),
	}, nil
}

// Run starts the conductor and blocks until ctx is cancelled.
func (c *Conductor) Run(ctx context.Context) error {
	if err := c.br.Connect(); err != nil {
		return fmt.Errorf("bridge connect: %w", err)
	}
	defer c.br.Close()

	botUserID := ""
	if mm, ok := c.br.(*bridge.MattermostBridge); ok {
		botUserID = mm.BotUserID
	}

	// Build agent list description for Claude
	agentList := c.buildAgentList()
	log.Printf("[conductor] started, %d agents: %s", len(c.cfg.Agents), agentList)

	go c.serveHook(ctx)

	for {
		select {
		case msg := <-c.br.Listen():
			// Skip bot messages (self + other bots)
			if msg.From == botUserID || c.isAgentMessage(msg.From) {
				continue
			}
			// Skip if already in a thread (let agents handle thread replies)
			if msg.RootID != "" {
				continue
			}
			if c.isDuplicate(msg.ID) {
				continue
			}

			go c.route(ctx, msg, agentList)

		case err := <-c.br.Errors():
			log.Printf("[conductor] bridge error: %v", err)

		case <-ctx.Done():
			return nil
		}
	}
}

func (c *Conductor) route(ctx context.Context, msg bridge.Message, agentList string) {
	log.Printf("[conductor] routing: %s", truncate(msg.Content, 80))

	prompt := fmt.Sprintf(`너는 에이전트 오케스트레이터야. 사용자의 메시지를 분석해서 적절한 에이전트에게 작업을 배분해.

등록된 에이전트:
%s

사용자 메시지: %s

규칙:
- 적절한 에이전트를 @멘션으로 태그해서 지시해
- 여러 에이전트가 필요하면 각각 역할을 명시해
- 간결하게 지시해 (2-3문장)
- 에이전트가 없는 작업이면 직접 답변해
- 반드시 @username 형식으로 태그해`, agentList, msg.Content)

	response, err := c.executor.Run(ctx, ModeAsk, prompt)
	if err != nil {
		log.Printf("[conductor] claude error: %v", err)
		return
	}
	response = c.sanitizer.Clean(response)
	log.Printf("[conductor] routing decision: %s", truncate(response, 120))

	// Post as thread reply to the original message
	if err := c.br.Send(bridge.Message{
		Channel: msg.Channel,
		Content: response,
		RootID:  msg.ID,
	}); err != nil {
		log.Printf("[conductor] send error: %v", err)
	}
}

func (c *Conductor) buildAgentList() string {
	var parts []string
	for _, a := range c.cfg.Agents {
		parts = append(parts, fmt.Sprintf("- @%s: %s", a.Username, a.Role))
	}
	return strings.Join(parts, "\n")
}

func (c *Conductor) isAgentMessage(userID string) bool {
	// Check if sender is a known bot by comparing with MM bot user IDs
	// For now, we skip any bot message (bots have "from_bot" prop in MM)
	// The bridge doesn't expose props yet, so we rely on the botUserID check in Run()
	return false
}

func (c *Conductor) isDuplicate(id string) bool {
	if c.seen[id] {
		return true
	}
	c.seen[id] = true
	return false
}

func (c *Conductor) serveHook(ctx context.Context) {
	// Conductor doesn't need a hook server for now
	// but reserve the port for future use
}
