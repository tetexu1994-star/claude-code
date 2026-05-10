// Package agent defines the Agent system for Tlaude Code.
//
// It provides Agent definitions, a definition store, fork message building,
// and the runtime for executing agents (sync, async, MoA).
package agent

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
)

// AgentState represents the lifecycle state of an agent run.
type AgentState string

const (
	AgentPending   AgentState = "pending"
	AgentRunning   AgentState = "running"
	AgentCompleted AgentState = "completed"
	AgentFailed    AgentState = "failed"
	AgentCancelled AgentState = "cancelled"
)

// ModelRef specifies which model and provider an agent should use.
// Empty fields mean "inherit from parent/session default".
type ModelRef struct {
	Provider string `yaml:"provider" json:"provider"` // "" = inherit (use session default)
	Model    string `yaml:"model" json:"model"`       // "" = inherit, "sonnet" = alias, full name = explicit
	Reason   string `yaml:"reason" json:"reason"`     // why this model was chosen (user-facing)
}

// AgentDefinition defines an agent type (equivalent to CC's AgentDefinition).
type AgentDefinition struct {
	AgentType       string            `yaml:"agent_type" json:"agent_type"`
	Name            string            `yaml:"name" json:"name"`
	Description     string            `yaml:"description" json:"description"`
	WhenToUse       string            `yaml:"when_to_use" json:"when_to_use"`
	Tools           []string          `yaml:"tools" json:"tools"`                       // ["*"] means all tools
	DisallowedTools []string          `yaml:"disallowed_tools" json:"disallowed_tools"` // tools to exclude
	MaxTurns        int               `yaml:"max_turns" json:"max_turns"`               // default 200
	Model           string            `yaml:"model" json:"model"`                       // legacy: simple model name override
	Provider        string            `yaml:"provider" json:"provider"`                 // legacy: simple provider override
	ModelRef        ModelRef          `yaml:"model_ref" json:"model_ref"`               // this agent's preferred model (takes precedence over legacy fields)
	ModelStrategy   []ModelRef        `yaml:"model_strategy" json:"model_strategy"`     // multiple model options for MoA
	PermissionMode  string            `yaml:"permission_mode" json:"permission_mode"`   // permission mode override
	Source          string            `yaml:"source" json:"source"`                     // "built-in", "user", "plugin"
	Color           string            `yaml:"color" json:"color"`                       // UI color
	Background      bool              `yaml:"background" json:"background"`             // default to background
	Isolation       string            `yaml:"isolation" json:"isolation"`               // "worktree", "remote", ""
	Memory          string            `yaml:"memory" json:"memory"`                     // memory scope
	SystemPrompt    string            `yaml:"system_prompt" json:"system_prompt"`       // system prompt override
	Env             map[string]string `yaml:"env" json:"env"`                           // extra environment variables
	Skills          []string          `yaml:"skills" json:"skills"`                     // skills to preload
	Filename        string            `yaml:"filename" json:"filename"`                 // original filename without extension
	Backend         string            `yaml:"backend" json:"backend"`                   // "inprocess" (default), "subprocess", "claude-code", "hermes"
}

// HasAllTools returns true if Tools contains "*".
func (d *AgentDefinition) HasAllTools() bool {
	for _, t := range d.Tools {
		if t == "*" {
			return true
		}
	}
	return false
}
// AgentRun tracks a single agent execution.
type AgentRun struct {
	mu           sync.Mutex        // protects mutable fields in concurrent use
	ID           string            `json:"id"`
	Definition   *AgentDefinition  `json:"-"`
	State        AgentState        `json:"state"`
	Prompt       string           `json:"prompt"`
	SystemPrompt string           `json:"system_prompt"`
	Model        string           `json:"model"`
	Provider     string           `json:"provider"`
	ParentID     string           `json:"parent_id"`
	CreatedAt    time.Time        `json:"created_at"`
	CompletedAt  *time.Time       `json:"completed_at,omitempty"`
	Result       string           `json:"result"`
	TokensInput  int              `json:"tokens_input"`
	TokensOutput int              `json:"tokens_output"`
	Messages     []llm.Message    `json:"messages"`
	Error        string           `json:"error,omitempty"`
	AbortSignal  chan struct{}    `json:"-"`
}

// GetState returns the agent state in a thread-safe manner.
func (r *AgentRun) GetState() AgentState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.State
}

// SetState sets the agent state in a thread-safe manner.
func (r *AgentRun) SetState(s AgentState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.State = s
}

// RunOptions configures a single agent run.
type RunOptions struct {
	ParentID       string
	AbortSignal    <-chan struct{}
	Isolation      string            // "" / "worktree" / "remote"
	PermissionMode string
	Env            map[string]string
	MaxTurns       int
	Tools          []string // tool whitelist (nil = use definition's tools)
	// Session defaults for model/provider inheritance.
	// When ModelRef.Model/Provider is empty, these are used.
	SessionModel    string
	SessionProvider string
	// CostTracker is used to record token usage and costs.
	CostTracker interface {
		Record(provider, model string, inputTokens, outputTokens int)
	}
}

// ForkOptions configures a fork sub-agent run.
type ForkOptions struct {
	ParentMessages       []llm.Message // full conversation of parent agent
	ParentSystemPrompt   string        // parent's system prompt (byte-identical for cache sharing)
	Directive            string        // sub-agent's instruction
	ParentToolUseResults []struct {
		ToolUseID string
		Result    string
	}
}

// ModelConfig defines a model for MoA parallel execution.
type ModelConfig struct {
	Provider string  `yaml:"provider" json:"provider"`
	Model    string  `yaml:"model" json:"model"`
	Weight   float64 `yaml:"weight" json:"weight"`
	Priority int     `yaml:"priority" json:"priority"`
}

// AggregatedResult is the result of MoA multi-model parallel execution.
type AggregatedResult struct {
	Results   []*AgentRun    `json:"results"`
	Final     string         `json:"final"`
	Strategy  string         `json:"strategy"` // "fastest", "consensus", "majority", "synthesize"
	Consensus float64        `json:"consensus"`
	TimeCost  time.Duration  `json:"time_cost"`
	TokenCost float64        `json:"token_cost"`
	ModelCfgs []ModelConfig  `json:"model_configs"`
	RawJSON   json.RawMessage `json:"raw_json,omitempty"`
}
