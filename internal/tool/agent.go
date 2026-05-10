package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// AgentBackend defines the interface for agent execution backends used by AgentTool.
// This avoids circular imports between tool and agent packages.
type AgentBackend interface {
	Name() string
	Execute(ctx context.Context, task AgentTask) (*AgentResult, error)
}

// AgentTask mirrors agent.AgentTask to avoid import cycles.
type AgentTask struct {
	AgentType string
	Prompt    string
	Model     string
	Provider  string
	Timeout   time.Duration
}

// AgentResult mirrors agent.AgentResult to avoid import cycles.
type AgentResult struct {
	Content      string
	ModelUsed    string
	Provider     string
	InputTokens  int
	OutputTokens int
	CostUSD      float64
	Duration     time.Duration
	Error        error
}

// AgentTool spawns a sub-agent to handle complex, multi-step tasks.
// It supports in-process execution, external subprocess backends, and MoA.
type AgentTool struct {
	manager  *TaskManager
	backends map[string]AgentBackend // "inprocess" | "claude-code" | "hermes"
	mu       sync.RWMutex

	// Bridge functions for AgentRuntime (set via SetRuntimeBridge).
	runSync func(ctx context.Context, agentType, prompt, model, provider string) (content string, inputTokens, outputTokens int, err error)
	runMoA  func(ctx context.Context, agentType, prompt string) (content string, timeCost time.Duration, tokenCost float64, err error)
}

// NewAgentTool creates an AgentTool with the given TaskManager.
func NewAgentTool(tm *TaskManager) *AgentTool {
	return &AgentTool{
		manager:  tm,
		backends: make(map[string]AgentBackend),
	}
}

// RegisterBackend adds an execution backend.
func (t *AgentTool) RegisterBackend(name string, backend AgentBackend) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.backends[name] = backend
}

// Backend returns a registered backend by name, or nil.
func (t *AgentTool) Backend(name string) AgentBackend {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.backends[name]
}

// SetRuntimeBridge wires the AgentTool to AgentRuntime methods.
// runSync: executes an agent synchronously and returns the result.
// runMoA: executes MoA multi-model parallel and returns the aggregated result.
func (t *AgentTool) SetRuntimeBridge(
	runSync func(ctx context.Context, agentType, prompt, model, provider string) (content string, inputTokens, outputTokens int, err error),
	runMoA func(ctx context.Context, agentType, prompt string) (content string, timeCost time.Duration, tokenCost float64, err error),
) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.runSync = runSync
	t.runMoA = runMoA
}

func (t *AgentTool) Name() string              { return "Agent" }
func (t *AgentTool) Description() string       { return "Launch a new agent to handle complex, multi-step tasks." }
func (t *AgentTool) IsEnabled() bool           { return true }
func (t *AgentTool) IsConcurrencySafe() bool   { return false }

func (t *AgentTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "description": {
      "type": "string",
      "description": "A short (3-5 word) description of the task"
    },
    "prompt": {
      "type": "string",
      "description": "The task for the agent to perform"
    },
    "subagent_type": {
      "type": "string",
      "description": "The type of specialized agent to use for this task",
      "enum": ["general", "explore", "code", "review", "moa", "external.claude-code", "external.hermes"]
    },
    "model": {
      "type": "string",
      "description": "Optional model override for this agent"
    },
    "provider": {
      "type": "string",
      "description": "Optional provider override for this agent"
    }
  },
  "required": ["description", "prompt"]
}`)
	return ToolDefinition{Name: "Agent", Description: t.Description(), InputSchema: schema}
}

func (t *AgentTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Description  string `json:"description"`
		Prompt       string `json:"prompt"`
		SubagentType string `json:"subagent_type"`
		Model        string `json:"model"`
		Provider     string `json:"provider"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if params.Description == "" || params.Prompt == "" {
		return &ToolResult{IsError: true, Content: "description and prompt are required"}, nil
	}
	if params.SubagentType == "" {
		params.SubagentType = "general"
	}

	// Create a task for tracking.
	task := t.manager.Create(params.Description, params.Prompt, params.SubagentType)

	// Handle MoA type.
	if params.SubagentType == "moa" {
		return t.executeMoA(ctx, task, params)
	}

	// Handle external agent types.
	if isExternalType(params.SubagentType) {
		return t.executeExternal(ctx, task, params)
	}

	// Default: in-process execution via AgentRuntime bridge.
	return t.executeInProcess(ctx, task, params)
}

func isExternalType(agentType string) bool {
	switch agentType {
	case "external.claude-code", "external.hermes", "external":
		return true
	}
	return false
}

func (t *AgentTool) executeInProcess(ctx context.Context, task *Task, params struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
}) (*ToolResult, error) {
	t.mu.RLock()
	runSync := t.runSync
	t.mu.RUnlock()

	if runSync == nil {
		// Bridge not configured — fall back to stub behavior (just create a task).
		t.manager.Complete(task.ID, fmt.Sprintf("Sub-agent %q queued for task: %s", params.SubagentType, params.Description))
		return &ToolResult{
			Content: fmt.Sprintf("Agent task created: %s\nTask ID: %s\nType: %s\nUse TaskGet(%s) to check status.",
				params.Description, task.ID, params.SubagentType, task.ID),
		}, nil
	}

	content, inputTokens, outputTokens, err := runSync(ctx, params.SubagentType, params.Prompt, params.Model, params.Provider)
	if err != nil {
		t.manager.Fail(task.ID, err.Error())
		return &ToolResult{IsError: true, Content: fmt.Sprintf("agent execution failed: %v", err)}, nil
	}

	t.manager.Complete(task.ID, content)

	summary := fmt.Sprintf("Agent %q completed.\nTokens: %d in / %d out\n\n%s",
		params.SubagentType, inputTokens, outputTokens, content)
	return &ToolResult{Content: summary}, nil
}

func (t *AgentTool) executeExternal(ctx context.Context, task *Task, params struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
}) (*ToolResult, error) {
	backendName := "claude-code"
	if params.SubagentType == "external.hermes" {
		backendName = "hermes"
	}

	backend := t.Backend(backendName)
	if backend == nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("external backend %q not configured", backendName)}, nil
	}

	agentTask := AgentTask{
		AgentType: params.SubagentType,
		Prompt:    params.Prompt,
		Model:     params.Model,
		Provider:  params.Provider,
		Timeout:   120 * time.Second,
	}

	result, err := backend.Execute(ctx, agentTask)
	if err != nil {
		t.manager.Fail(task.ID, err.Error())
		return &ToolResult{IsError: true, Content: fmt.Sprintf("external agent failed: %v", err)}, nil
	}

	t.manager.Complete(task.ID, result.Content)

	summary := fmt.Sprintf("External agent %q completed.\nDuration: %v\n\n%s",
		backendName, result.Duration, result.Content)
	return &ToolResult{Content: summary}, nil
}

func (t *AgentTool) executeMoA(ctx context.Context, task *Task, params struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentType string `json:"subagent_type"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
}) (*ToolResult, error) {
	t.mu.RLock()
	runMoA := t.runMoA
	t.mu.RUnlock()

	if runMoA == nil {
		return &ToolResult{IsError: true, Content: "MoA runtime bridge not configured"}, nil
	}

	content, timeCost, tokenCost, err := runMoA(ctx, params.SubagentType, params.Prompt)
	if err != nil {
		t.manager.Fail(task.ID, err.Error())
		return &ToolResult{IsError: true, Content: fmt.Sprintf("MoA execution failed: %v", err)}, nil
	}

	t.manager.Complete(task.ID, content)

	summary := fmt.Sprintf("MoA completed.\nDuration: %v | Est. cost: $%.4f\n\n%s",
		timeCost, tokenCost, content)
	return &ToolResult{Content: summary}, nil
}
