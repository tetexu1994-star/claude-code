// Package tool defines the core Tool interface and supporting types
// for the Tlaude Code tool system, compatible with the Claude Code architecture.
package tool

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/tetexu/tlaude-code/internal/tool/permission"
)

// ToolDefinition is the static description of a tool, sent to LLMs
// so they know what tools are available and how to call them.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema for the tool's input
}

// ToolResult is the result of a tool execution.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

// ToolContext provides execution context to a tool when it runs.
type ToolContext struct {
	CWD           string
	Env           map[string]string
	AbortSignal   <-chan struct{}
	PermissionCtx *permission.PermissionContext
	Logger        *slog.Logger
}

// Tool is the runtime interface that every tool must implement.
type Tool interface {
	Name() string
	Description() string
	ToolDefinition() ToolDefinition
	IsEnabled() bool
	Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error)
	IsConcurrencySafe() bool
}
