// Package definition implements the agent definition loading system.
//
// It provides types, parsers, a directory loader, built-in agent registry,
// and memory integration — the glue layer connecting agent/store, memory/ (MemDir),
// and swarm/ packages.
package definition

// Source identifies where an agent definition originated.
type Source string

const (
	SourceBuiltIn  Source = "built-in"
	SourceUser     Source = "userSettings"
	SourceProject  Source = "projectSettings"
	SourcePolicy   Source = "policySettings"
	SourcePlugin   Source = "plugin"
)

// MemoryScope represents the persistence scope for agent memory.
type MemoryScope string

const (
	MemUser    MemoryScope = "user"
	MemProject MemoryScope = "project"
	MemLocal   MemoryScope = "local"
)

// EffortValue represents the thinking effort level for an agent.
type EffortValue string

const (
	EffortLow    EffortValue = "low"
	EffortMedium EffortValue = "medium"
	EffortHigh   EffortValue = "high"
)

// McpServerConfig is an inline MCP server configuration.
type McpServerConfig struct {
	Command string            `yaml:"command" json:"command"`
	Args    []string          `yaml:"args" json:"args"`
	Env     map[string]string `yaml:"env" json:"env"`
}

// AgentMcpServerSpec references an MCP server by name or inline config.
type AgentMcpServerSpec struct {
	Name         string            `yaml:"name" json:"name"`
	InlineConfig *McpServerConfig  `yaml:"inline_config" json:"inline_config"`
}

// HooksSettings configures hook scripts for an agent.
type HooksSettings struct {
	BeforeToolUse []string `yaml:"before_tool_use" json:"before_tool_use"`
	AfterToolUse  []string `yaml:"after_tool_use" json:"after_tool_use"`
}

// AgentDefinition defines an agent type loaded from markdown, JSON, or built-in registry.
type AgentDefinition struct {
	AgentType       string               `yaml:"agent_type" json:"agent_type"`
	Name            string               `yaml:"name" json:"name"`
	Description     string               `yaml:"description" json:"description"`
	WhenToUse       string               `yaml:"when_to_use" json:"when_to_use"`
	Tools           []string             `yaml:"tools" json:"tools"`           // nil = all tools
	DisallowedTools []string             `yaml:"disallowed_tools" json:"disallowed_tools"`
	Skills          []string             `yaml:"skills" json:"skills"`
	McpServers      []AgentMcpServerSpec `yaml:"mcp_servers" json:"mcp_servers"`
	Hooks           *HooksSettings       `yaml:"hooks" json:"hooks"`
	Color           string               `yaml:"color" json:"color"`
	Model           string               `yaml:"model" json:"model"` // "inherit" or model name
	Effort          *EffortValue         `yaml:"effort" json:"effort"`
	PermissionMode  *string              `yaml:"permission_mode" json:"permission_mode"` // "bubble", "bypassPermissions", etc.
	MaxTurns        *int                 `yaml:"max_turns" json:"max_turns"`
	Background             bool                 `yaml:"background" json:"background"`
	OmitClaudeMd           bool                 `yaml:"omit_claude_md" json:"omit_claude_md"`
	InitialPrompt          string               `yaml:"initial_prompt" json:"initial_prompt"`
	CriticalSystemReminder string               `yaml:"critical_system_reminder" json:"critical_system_reminder"`
	Memory          *MemoryScope         `yaml:"memory" json:"memory"`
	Isolation       string               `yaml:"isolation" json:"isolation"` // "worktree"
	Source          Source               `yaml:"source" json:"source"`
	Filename        string               `yaml:"filename" json:"filename"` // for user/project agents (without .md)
	BaseDir         string               `yaml:"base_dir" json:"base_dir"` // for file-sourced agents
	Plugin          string               `yaml:"plugin" json:"plugin"`     // for plugin agents
	SystemPrompt    string               `yaml:"system_prompt" json:"system_prompt"` // static prompt body
}

// AgentDefinitionsResult holds the merged result of loading agent definitions.
type AgentDefinitionsResult struct {
	ActiveAgents []*AgentDefinition
	AllAgents    []*AgentDefinition
	FailedFiles  []ParseError
}

// ParseError records a file that failed to parse.
type ParseError struct {
	Path  string
	Error string
}
