package bash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tetexu/tlaude-code/internal/tool"
)

type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
}

type Tool struct {
	Name        string
	Description string
	Enabled     bool
	Timeout     time.Duration
	AllowedCmds []string
}

func NewTool() *Tool {
	return &Tool{
		Name:        "bash",
		Description: "Execute shell commands and return results",
		Enabled:     true,
		Timeout:     120 * time.Second,
		AllowedCmds: nil,
	}
}

func (t *Tool) Execute(ctx context.Context, command string) (*Result, error) {
	if !t.Enabled {
		return nil, fmt.Errorf("bash tool is disabled")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(cmdCtx, "bash", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start).Round(time.Millisecond).String()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to execute command: %w", err)
		}
	}

	return &Result{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

func (t *Tool) SetAllowedCommands(cmds []string) {
	t.AllowedCmds = cmds
}

// --- Tool interface adapter ---

// BashToolName is the canonical name for the bash tool.
const BashToolName = "bash"

// BashTool wraps Tool to implement the tool.Tool interface.
type BashTool struct {
	inner *Tool
}

// NewBashTool returns a BashTool that implements tool.Tool.
func NewBashTool() *BashTool {
	return &BashTool{inner: NewTool()}
}

// Name returns the tool's canonical name.
func (bt *BashTool) Name() string { return BashToolName }

// Description returns a human-readable description for the LLM.
func (bt *BashTool) Description() string {
	return "Execute a shell command in a sandboxed environment. Returns stdout, stderr, and exit code."
}

// ToolDefinition returns the LLM-facing tool definition with JSON Schema.
func (bt *BashTool) ToolDefinition() tool.ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "description": "The shell command to execute"
    }
  },
  "required": ["command"]
}`)
	return tool.ToolDefinition{
		Name:        BashToolName,
		Description: bt.Description(),
		InputSchema: schema,
	}
}

// IsEnabled returns whether the tool is currently enabled.
func (bt *BashTool) IsEnabled() bool { return bt.inner.Enabled }

// IsConcurrencySafe returns false — bash execution has side effects.
func (bt *BashTool) IsConcurrencySafe() bool { return false }

// Execute runs the bash tool from JSON input per the tool.Tool interface.
func (bt *BashTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &tool.ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	result, err := bt.inner.Execute(ctx, params.Command)
	if err != nil {
		return &tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	content := result.Stdout
	if result.Stderr != "" {
		content += "\n[stderr]\n" + result.Stderr
	}
	return &tool.ToolResult{
		Content: content,
	}, nil
}
