package swarm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/tetexu/tlaude-code/internal/agent"
	"github.com/tetexu/tlaude-code/internal/llm"
	"github.com/tetexu/tlaude-code/internal/tool"
)

// activeTeammate tracks a running teammate agent.
type activeTeammate struct {
	cancel  context.CancelFunc
	config  TeammateSpawnConfig
	agentID string
}

// InProcessBackend is the in-process implementation of TeammateExecutor.
// Each teammate runs as a goroutine within the same process.
type InProcessBackend struct {
	runtime  *agent.AgentRuntime
	store    *agent.AgentDefStore
	llmReg   *llm.Registry
	toolReg  *tool.Registry
	logger   *slog.Logger

	ctx context.Context

	mu            sync.Mutex
	activeAgents  map[string]*activeTeammate // agentID -> teammate

	swarmStore    *SwarmStore
	mailboxStates map[string]*mailboxState // agentID -> mailbox state tracker
}

// NewInProcessBackend creates a new InProcessBackend.
func NewInProcessBackend(
	rt *agent.AgentRuntime,
	store *agent.AgentDefStore,
	lr *llm.Registry,
	tr *tool.Registry,
) *InProcessBackend {
	return &InProcessBackend{
		runtime:       rt,
		store:         store,
		llmReg:        lr,
		toolReg:       tr,
		logger:        slog.Default().With("component", "swarm-backend"),
		activeAgents:  make(map[string]*activeTeammate),
		mailboxStates: make(map[string]*mailboxState),
	}
}

// SetSwarmStore sets the SwarmStore reference for permission bridge.
func (b *InProcessBackend) SetSwarmStore(s *SwarmStore) {
	b.swarmStore = s
}

// Type returns "in-process".
func (b *InProcessBackend) Type() string {
	return "in-process"
}

// SetContext sets the base context for spawned agents.
func (b *InProcessBackend) SetContext(ctx context.Context) {
	b.ctx = ctx
}

// Spawn starts a new teammate agent in a goroutine.
func (b *InProcessBackend) Spawn(config TeammateSpawnConfig) (*TeammateSpawnResult, error) {
	agentType := DefaultAgentType
	if config.Name != "" {
		// Use config name to determine agent type; default to general.
		agentType = DefaultAgentType
	}

	def, ok := b.store.Get(agentType)
	if !ok {
		return nil, fmt.Errorf("agent type %q not found", agentType)
	}

	// Apply config overrides to a copy of the definition.
	runDef := *def
	if config.Model != "" {
		runDef.Model = config.Model
	}
	if config.SystemPrompt != "" {
		if config.SystemPromptMode == "replace" {
			runDef.SystemPrompt = config.SystemPrompt
		} else if config.SystemPromptMode == "append" {
			runDef.SystemPrompt = runDef.SystemPrompt + "\n\n" + config.SystemPrompt
		}
		// "default": use agent's own system prompt, ignore config's.
	}
	if len(config.AllowedTools) > 0 {
		runDef.Tools = config.AllowedTools
	}
	runDef.Color = config.Color

	// Create isolated context.
	bgCtx := context.Background()
	if b.ctx != nil {
		bgCtx = b.ctx
	}
	agentCtx, cancel := context.WithCancel(bgCtx)

	agentID := fmt.Sprintf("%s@%s", config.Name, config.TeamName)

	at := &activeTeammate{
		cancel:  cancel,
		config:  config,
		agentID: agentID,
	}

	b.mu.Lock()
	b.activeAgents[agentID] = at
	b.mailboxStates[agentID] = NewMailboxState()
	b.mu.Unlock()

	// Launch the runner goroutine.
	go b.runTeammate(agentCtx, agentID, &runDef, config)

	return &TeammateSpawnResult{
		Success: true,
		AgentID: agentID,
		TaskID:  agentID,
		Cancel:  cancel,
	}, nil
}

// SendMessage sends a message to a teammate's mailbox.
func (b *InProcessBackend) SendMessage(agentID string, msg MailboxMessage) error {
	// agentID format: "name@team-name"
	teamName, agentName := parseAgentID(agentID)
	return WriteToMailbox(teamName, agentName, msg)
}

// ReadMessages reads a teammate's mailbox.
func (b *InProcessBackend) ReadMessages(agentID string) ([]MailboxEntry, error) {
	teamName, agentName := parseAgentID(agentID)

	b.mu.Lock()
	state := b.mailboxStates[agentID]
	if state == nil {
		state = NewMailboxState()
		b.mailboxStates[agentID] = state
	}
	b.mu.Unlock()

	return ReadMailbox(teamName, agentName, state)
}

// MarkMessageRead marks a message as read.
func (b *InProcessBackend) MarkMessageRead(agentID, filename string) error {
	teamName, agentName := parseAgentID(agentID)

	b.mu.Lock()
	state := b.mailboxStates[agentID]
	if state == nil {
		state = NewMailboxState()
		b.mailboxStates[agentID] = state
	}
	b.mu.Unlock()

	return MarkAsRead(teamName, agentName, filename, state)
}

// Terminate sends a graceful termination request to the teammate.
func (b *InProcessBackend) Terminate(agentID string) error {
	b.mu.Lock()
	at, ok := b.activeAgents[agentID]
	b.mu.Unlock()

	if !ok {
		return fmt.Errorf("teammate %q not found", agentID)
	}

	// Write a termination message to the teammate's mailbox.
	teamName, _ := parseAgentID(agentID)
	msg := MailboxMessage{
		From: "leader",
		Text: "/terminate",
	}
	_ = WriteToMailbox(teamName, agentID, msg)

	// Cancel the context to stop the goroutine.
	at.cancel()
	return nil
}

// Kill forcefully stops a teammate by cancelling its context.
func (b *InProcessBackend) Kill(agentID string) error {
	b.mu.Lock()
	at, ok := b.activeAgents[agentID]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("teammate %q not found", agentID)
	}
	delete(b.activeAgents, agentID)
	delete(b.mailboxStates, agentID)
	b.mu.Unlock()

	at.cancel()
	return nil
}

// IsActive checks if a teammate is currently running.
func (b *InProcessBackend) IsActive(agentID string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.activeAgents[agentID]
	return ok, nil
}

// ListActive returns all currently active teammate agent IDs.
func (b *InProcessBackend) ListActive() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	result := make([]string, 0, len(b.activeAgents))
	for id := range b.activeAgents {
		result = append(result, id)
	}
	return result
}

// parseAgentID splits "name@team-name" into (teamName, agentName).
func parseAgentID(agentID string) (teamName, agentName string) {
	for i := len(agentID) - 1; i >= 0; i-- {
		if agentID[i] == '@' {
			return agentID[i+1:], agentID[:i]
		}
	}
	// No @ separator — treat entire string as both.
	return agentID, agentID
}
