package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
	"github.com/tetexu/tlaude-code/internal/memory"
	"github.com/tetexu/tlaude-code/internal/tool"
	"github.com/tetexu/tlaude-code/internal/tool/executor"
)

// AgentRuntime is the core execution engine for agents.
// It manages agent lifecycle and implements the LLM message loop.
type AgentRuntime struct {
	store       *AgentDefStore
	toolReg     *tool.Registry
	llmReg      *llm.Registry
	memoryStore *memory.Store
	logger      *slog.Logger
	active      sync.Map // agentID -> *AgentRun

	idCounter atomic.Int64
}

// NewAgentRuntime creates a new AgentRuntime.
func NewAgentRuntime(store *AgentDefStore, toolReg *tool.Registry, llmReg *llm.Registry) *AgentRuntime {
	return &AgentRuntime{
		store:       store,
		toolReg:     toolReg,
		llmReg:      llmReg,
		memoryStore: memory.DefaultStore(),
		logger:      slog.Default().With("component", "agent-runtime"),
	}
}

// SetMemoryStore sets the memory store used for agent system prompts.
func (r *AgentRuntime) SetMemoryStore(ms *memory.Store) {
	r.memoryStore = ms
}

// RunAgent executes an agent synchronously.
// Implements the LLM message loop: call LLM → parse tool calls → execute → continue.
func (r *AgentRuntime) RunAgent(ctx context.Context, def *AgentDefinition, prompt string, opts *RunOptions) (*AgentRun, error) {
	if def == nil {
		return nil, fmt.Errorf("agent definition is nil")
	}
	if opts == nil {
		opts = &RunOptions{}
	}

	// Resolve model and provider from definition.
	model, providerName := r.resolveModelProvider(def, opts)
	prov, ok := r.llmReg.Get(providerName)
	if !ok {
		return nil, fmt.Errorf("provider %q not found", providerName)
	}

	// Build system prompt.
	sysPrompt := r.buildSystemPrompt(def)

	// Create the run.
	run := &AgentRun{
		ID:           r.nextID(),
		Definition:   def,
		State:        AgentRunning,
		Prompt:       prompt,
		SystemPrompt: sysPrompt,
		Model:        model,
		Provider:     providerName,
		ParentID:     opts.ParentID,
		CreatedAt:    time.Now(),
		AbortSignal:  make(chan struct{}),
	}

	// Register as active.
	r.active.Store(run.ID, run)
	defer r.active.Delete(run.ID)

	return r.executeRun(ctx, run, def, prompt, sysPrompt, model, prov, opts)
}

// executeRun is the internal execution method shared by sync and async paths.
func (r *AgentRuntime) executeRun(
	ctx context.Context,
	run *AgentRun,
	def *AgentDefinition,
	prompt string,
	sysPrompt string,
	model string,
	prov llm.Provider,
	opts *RunOptions,
) (*AgentRun, error) {
	defer func() {
		now := time.Now()
		run.CompletedAt = &now
	}()

	// Create a cancellable context that combines parent ctx and abort signal.
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	go func() {
		select {
		case <-runCtx.Done():
		case <-run.AbortSignal:
			runCancel()
		}
	}()

	// Also watch external abort signal.
	if opts.AbortSignal != nil {
		go func() {
			select {
			case <-runCtx.Done():
			case <-opts.AbortSignal:
				runCancel()
			}
		}()
	}

	// Filter tool pool.
	toolPool := r.filterTools(def, opts)
	llmTools := r.toolsToLLMDefs(toolPool)

	// Build initial user message.
	run.Messages = []llm.Message{
		{Role: "user", Content: prompt},
	}

	// Resolve max turns.
	maxTurns := def.MaxTurns
	if opts.MaxTurns > 0 {
		maxTurns = opts.MaxTurns
	}

	// LLM message loop.
	for turn := 0; turn < maxTurns; turn++ {
		select {
		case <-runCtx.Done():
			run.SetState(AgentCancelled)
			run.Error = "cancelled"
			return run, runCtx.Err()
		default:
		}

		req := llm.ChatRequest{
			Model:       model,
			Messages:    run.Messages,
			System:      sysPrompt,
			MaxTokens:   4096,
			Temperature: 0.0,
		}
		if len(llmTools) > 0 {
			req.Tools = llmTools
		}

		resp, err := prov.Chat(runCtx, req)
		if err != nil {
			select {
			case <-runCtx.Done():
				run.SetState(AgentCancelled)
				run.Error = "cancelled"
			default:
				run.SetState(AgentFailed)
				run.Error = err.Error()
			}
			return run, err
		}

		run.TokensInput += resp.InputTokens
		run.TokensOutput += resp.OutputTokens

		assistantMsg := resp.Message
		assistantMsg.Role = "assistant"
		run.Messages = append(run.Messages, assistantMsg)

		// No tool calls — this is the final response.
		if len(resp.Message.ToolCalls) == 0 {
			run.SetState(AgentCompleted)
			run.Result = resp.Message.Content
			return run, nil
		}

		// Execute tool calls via StreamingToolExecutor.
		toolCtx := &tool.ToolContext{
			CWD:         "",
			AbortSignal: run.AbortSignal,
			Logger:      r.logger,
		}
		exec := executor.New(runCtx, toolPool, nil, toolCtx)

		// Track which LLM tool_use IDs failed to add.
		addErrors := make(map[int]llm.Message)
		for i, tc := range resp.Message.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Args)
			if err := exec.AddTool(tc.Name, argsJSON); err != nil {
				addErrors[i] = llm.Message{
					Role:    "user",
					Content: fmt.Sprintf("[tool_result id=%s] error: %v", tc.ID, err),
				}
			}
		}

		// Collect results in order. They come out in the same order as AddTool calls.
		errorCount := len(addErrors)
		toolResults := make([]llm.Message, 0, len(resp.Message.ToolCalls))
		for i, tc := range resp.Message.ToolCalls {
			if errMsg, hadError := addErrors[i]; hadError {
				toolResults = append(toolResults, errMsg)
				continue
			}

			result, ok := exec.NextResult(runCtx)
			if !ok {
				errorCount++
				toolResults = append(toolResults, llm.Message{
					Role:    "user",
					Content: fmt.Sprintf("[tool_result id=%s] (no result)", tc.ID),
				})
				continue
			}

			content := result.Content
			if result.IsError {
				content = "error: " + content
				errorCount++
			}
			toolResults = append(toolResults, llm.Message{
				Role:    "user",
				Content: fmt.Sprintf("[tool_result id=%s] %s", tc.ID, content),
			})
		}

		exec.Close()
		run.Messages = append(run.Messages, toolResults...)

		// Check if context was cancelled (abort signal).
		select {
		case <-runCtx.Done():
			run.SetState(AgentCancelled)
			run.Error = "cancelled"
			return run, runCtx.Err()
		default:
		}

		// If all tool calls resulted in errors, stop the loop.
		if errorCount == len(resp.Message.ToolCalls) {
			run.SetState(AgentFailed)
			run.Error = "all tool calls failed"
			return run, nil
		}
	}

	run.SetState(AgentFailed)
	run.Error = fmt.Sprintf("exceeded max turns (%d)", maxTurns)
	return run, nil
}

// RunAgentAsync starts an agent in the background.
// Returns the agent ID immediately.
func (r *AgentRuntime) RunAgentAsync(ctx context.Context, def *AgentDefinition, prompt string, opts *RunOptions) (string, error) {
	if opts == nil {
		opts = &RunOptions{}
	}

	model, providerName := r.resolveModelProvider(def, opts)
	prov, ok := r.llmReg.Get(providerName)
	if !ok {
		return "", fmt.Errorf("provider %q not found", providerName)
	}

	sysPrompt := r.buildSystemPrompt(def)

	run := &AgentRun{
		ID:           r.nextID(),
		Definition:   def,
		State:        AgentPending,
		Prompt:       prompt,
		ParentID:     opts.ParentID,
		CreatedAt:    time.Now(),
		AbortSignal:  make(chan struct{}),
	}
	r.active.Store(run.ID, run)

	// Make a copy of opts so the goroutine owns it.
	optsCopy := *opts

	go func() {
		bgCtx := context.Background()
		result, _ := r.executeRun(bgCtx, run, def, prompt, sysPrompt, model, prov, &optsCopy)
		// Overwrite with final state so querying the same ID sees the completed run.
		if result != nil {
			r.active.Store(run.ID, result)
		} else {
			r.active.Delete(run.ID)
		}
	}()

	return run.ID, nil
}

// GetAgent returns an active agent run by ID.
func (r *AgentRuntime) GetAgent(id string) (*AgentRun, bool) {
	v, ok := r.active.Load(id)
	if !ok {
		return nil, false
	}
	run, ok := v.(*AgentRun)
	return run, ok
}

// ListAgents returns all active agent runs.
func (r *AgentRuntime) ListAgents() []*AgentRun {
	var result []*AgentRun
	r.active.Range(func(_, v interface{}) bool {
		run, ok := v.(*AgentRun)
		if ok {
			result = append(result, run)
		}
		return true
	})
	return result
}

// StopAgent stops a running agent by closing its abort signal.
func (r *AgentRuntime) StopAgent(id string) error {
	v, ok := r.active.Load(id)
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	run, ok := v.(*AgentRun)
	if !ok {
		return fmt.Errorf("agent %q: invalid type", id)
	}
	if run.GetState() != AgentRunning && run.GetState() != AgentPending {
		return fmt.Errorf("agent %q is not running (state: %s)", id, run.GetState())
	}
	close(run.AbortSignal)
	return nil
}

// RunAgentByType looks up an agent definition by type and executes it synchronously.
// This is used as a bridge callback from the tool package.
func (r *AgentRuntime) RunAgentByType(ctx context.Context, agentType, prompt, model, provider string) (content string, inputTokens, outputTokens int, err error) {
	def, ok := r.store.Get(agentType)
	if !ok {
		return "", 0, 0, fmt.Errorf("agent type %q not found", agentType)
	}

	opts := &RunOptions{
		SessionModel:    model,
		SessionProvider: provider,
	}

	run, err := r.RunAgent(ctx, def, prompt, opts)
	if err != nil {
		return "", 0, 0, err
	}

	return run.Result, run.TokensInput, run.TokensOutput, nil
}

// RunMoAByType looks up an agent definition by type and runs MoA execution.
// This is used as a bridge callback from the tool package.
func (r *AgentRuntime) RunMoAByType(ctx context.Context, agentType, prompt string) (content string, timeCost time.Duration, tokenCost float64, err error) {
	def, ok := r.store.Get(agentType)
	if !ok {
		return "", 0, 0, fmt.Errorf("agent type %q not found", agentType)
	}

	result, err := r.RunMoA(ctx, def, prompt, nil, nil)
	if err != nil {
		return "", 0, 0, err
	}

	return result.Final, result.TimeCost, result.TokenCost, nil
}

// Store returns the AgentDefStore for external wiring.
func (r *AgentRuntime) Store() *AgentDefStore {
	return r.store
}

// --- internal helpers ---

func (r *AgentRuntime) nextID() string {
	n := r.idCounter.Add(1)
	return fmt.Sprintf("agent-%d", n)
}

// resolveModelProvider determines which model and provider to use for an agent run.
// Priority: ModelRef > legacy Model/Provider fields > session defaults > hardcoded defaults.
func (r *AgentRuntime) resolveModelProvider(def *AgentDefinition, opts *RunOptions) (model, provider string) {
	sessionModel := "claude-sonnet-4"
	sessionProvider := "anthropic"
	if opts != nil {
		if opts.SessionModel != "" {
			sessionModel = opts.SessionModel
		}
		if opts.SessionProvider != "" {
			sessionProvider = opts.SessionProvider
		}
	}

	// Use ModelRef if populated (new field, takes precedence).
	if def.ModelRef.Model != "" || def.ModelRef.Provider != "" {
		model = def.ModelRef.Model
		provider = def.ModelRef.Provider
	} else {
		// Fall back to legacy fields.
		model = def.Model
		provider = def.Provider
	}

	// Apply inheritance: empty means use session default.
	if model == "" || model == "inherit" {
		model = sessionModel
	}
	if provider == "" || provider == "inherit" {
		provider = sessionProvider
	}

	return
}

func (r *AgentRuntime) buildSystemPrompt(def *AgentDefinition) string {
	base := def.SystemPrompt
	if base == "" {
		base = fmt.Sprintf("You are %s. %s\n%s", def.Name, def.Description, def.WhenToUse)
	}

	// Inject memory prompt for agents with memory enabled.
	// Default: inject for "general" and "code" agents with user scope.
	if r.memoryStore != nil {
		agentType := def.AgentType
		memScope := def.Memory

		if memScope == "" && (agentType == "general" || agentType == "code") {
			memScope = "user"
		}

		if memScope != "" {
			var scope memory.AgentMemoryScope
			switch memScope {
			case "project":
				scope = memory.ScopeProject
			case "local":
				scope = memory.ScopeLocal
			default:
				scope = memory.ScopeUser
			}
			memPrompt := memory.LoadAgentMemoryPrompt(agentType, scope)
			base = memPrompt + "\n\n" + base
		}
	}

	return base
}

// filterTools filters the tool pool based on the agent definition's Tools and DisallowedTools.
func (r *AgentRuntime) filterTools(def *AgentDefinition, opts *RunOptions) []tool.Tool {
	toolWhitelist := def.Tools
	if opts.Tools != nil {
		toolWhitelist = opts.Tools
	}

	allTools := r.toolReg.GetAll(nil)

	includeAll := false
	for _, t := range toolWhitelist {
		if t == "*" {
			includeAll = true
			break
		}
	}

	disallowed := make(map[string]bool)
	for _, t := range def.DisallowedTools {
		disallowed[t] = true
	}

	var result []tool.Tool
	for _, t := range allTools {
		name := t.Name()
		if disallowed[name] {
			continue
		}
		if includeAll || r.isInWhitelist(name, toolWhitelist) {
			result = append(result, t)
		}
	}
	return result
}

func (r *AgentRuntime) isInWhitelist(name string, whitelist []string) bool {
	for _, w := range whitelist {
		if name == w {
			return true
		}
	}
	return false
}

// toolsToLLMDefs converts tool.Tool instances to llm.ToolDefinition format.
func (r *AgentRuntime) toolsToLLMDefs(tools []tool.Tool) []llm.ToolDefinition {
	defs := make([]llm.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		td := t.ToolDefinition()
		var schemaMap map[string]interface{}
		if err := json.Unmarshal(td.InputSchema, &schemaMap); err != nil {
			schemaMap = map[string]interface{}{}
		}
		defs = append(defs, llm.ToolDefinition{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: schemaMap,
		})
	}
	return defs
}
