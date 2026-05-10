package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tetexu/tlaude-code/internal/cost"
	"github.com/tetexu/tlaude-code/internal/llm"
	"github.com/tetexu/tlaude-code/internal/tool"
)

// Backend is the execution interface for agents.
// Different backends can run agents in-process (via LLM provider),
// or out-of-process (via external CLI tools like Claude Code, Hermes).
type Backend interface {
	Name() string
	Execute(ctx context.Context, task AgentTask) (*AgentResult, error)
	Capabilities() []string
}

// AgentTask is the input to a Backend.Execute call.
type AgentTask struct {
	Def      *AgentDefinition
	Prompt   string
	Model    string // resolved model name
	Provider string // resolved provider name
	Timeout  time.Duration
}

// AgentResult is the output from a Backend.Execute call.
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

// InProcessBackend executes agents by calling the LLM provider directly.
type InProcessBackend struct {
	registry *llm.Registry
	tracker  *cost.Tracker
}

// NewInProcessBackend creates a backend that uses the LLM registry for direct calls.
func NewInProcessBackend(registry *llm.Registry, tracker *cost.Tracker) *InProcessBackend {
	return &InProcessBackend{
		registry: registry,
		tracker:  tracker,
	}
}

func (b *InProcessBackend) Name() string { return "inprocess" }

func (b *InProcessBackend) Capabilities() []string {
	return []string{"chat", "stream", "tools"}
}

func (b *InProcessBackend) Execute(ctx context.Context, task AgentTask) (*AgentResult, error) {
	provider, providerName, err := b.registry.SelectAvailable(task.Provider)
	if err != nil {
		return nil, fmt.Errorf("select provider %q: %w", task.Provider, err)
	}

	model := task.Model
	if model == "" {
		if cfg, ok := b.registry.GetConfig(providerName); ok && cfg.Model != "" {
			model = cfg.Model
		} else {
			model = "claude-sonnet-4"
		}
	}

	req := llm.ChatRequest{
		Model:    model,
		Messages: []llm.Message{{Role: "user", Content: task.Prompt}},
	}

	if task.Def != nil {
		req.MaxTokens = 4096
		req.Temperature = 0.0
	}

	start := time.Now()
	resp, err := provider.Chat(ctx, req)
	duration := time.Since(start)

	if err != nil {
		return &AgentResult{
			Duration: duration,
			Error:    err,
		}, err
	}

	result := &AgentResult{
		Content:      resp.Message.Content,
		ModelUsed:    resp.Model,
		Provider:     providerName,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		CostUSD:      cost.EstimateCost(providerName, model, resp.InputTokens, resp.OutputTokens),
		Duration:     duration,
	}

	if b.tracker != nil {
		b.tracker.Record(providerName, model, resp.InputTokens, resp.OutputTokens)
	}

	return result, nil
}

// SubprocessBackend executes agents by launching an external CLI process
// (e.g., Claude Code CLI, Hermes Agent) and piping the prompt to stdin,
// reading the result from stdout.
type SubprocessBackend struct {
	Cmd     string        // the command to execute
	Args    []string      // base arguments (e.g., ["--print"])
	Timeout time.Duration // per-call timeout
	Label   string        // display name for this backend
}

// NewSubprocessBackend creates a generic subprocess backend from explicit config values.
func NewSubprocessBackend(label, cmd string, args []string, timeout time.Duration) *SubprocessBackend {
	return &SubprocessBackend{
		Cmd:     cmd,
		Args:    args,
		Timeout: timeout,
		Label:   label,
	}
}

// NewClaudeCodeBackend creates a backend that calls Claude Code CLI via npx.
func NewClaudeCodeBackend(timeout time.Duration) *SubprocessBackend {
	return NewSubprocessBackend("claude-code", "npx",
		[]string{"@anthropic-ai/claude-code", "--print", "--dangerously-skip-permissions"},
		timeout)
}

// NewHermesBackend creates a backend that calls Hermes Agent in oneshot mode.
func NewHermesBackend(timeout time.Duration) *SubprocessBackend {
	home, _ := os.UserHomeDir()
	return NewSubprocessBackend("hermes",
		filepath.Join(home, ".hermes", "hermes-agent", "venv", "bin", "hermes"),
		[]string{"-z"}, timeout)
}

func (b *SubprocessBackend) Name() string {
	if b.Label != "" {
		return b.Label
	}
	return "subprocess"
}

func (b *SubprocessBackend) Capabilities() []string {
	return []string{"subprocess", "external"}
}

func (b *SubprocessBackend) Execute(ctx context.Context, task AgentTask) (*AgentResult, error) {
	timeout := b.Timeout
	if task.Timeout > 0 {
		timeout = task.Timeout
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := b.buildArgs(task)

	cmd := exec.CommandContext(ctx, b.Cmd, args...)

	// Pipe stdin for the prompt.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	// Capture stdout and stderr.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start subprocess %s: %w", b.Cmd, err)
	}

	// Write prompt to stdin, then close it.
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, task.Prompt)
	}()

	err = cmd.Wait()
	duration := time.Since(start)

	if err != nil {
		errMsg := stderr.String()
		if ctx.Err() == context.DeadlineExceeded {
			return &AgentResult{
				Duration: duration,
				Error:    fmt.Errorf("subprocess timeout after %v", timeout),
			}, fmt.Errorf("subprocess timeout after %v", timeout)
		}
		return &AgentResult{
			Duration: duration,
			Error:    fmt.Errorf("subprocess failed: %w\nstderr: %s", err, errMsg),
		}, fmt.Errorf("subprocess failed: %w\nstderr: %s", err, errMsg)
	}

	output := stdout.String()

	return &AgentResult{
		Content:  output,
		Provider: task.Provider,
		ModelUsed: task.Model,
		Duration: duration,
	}, nil
}

// buildArgs constructs the CLI arguments for the subprocess call.
func (b *SubprocessBackend) buildArgs(task AgentTask) []string {
	args := make([]string, len(b.Args))
	copy(args, b.Args)

	if task.Model != "" && task.Model != "inherit" {
		args = append(args, "--model", task.Model)
	}
	if task.Provider != "" && task.Provider != "inherit" {
		args = append(args, "--provider", task.Provider)
	}

	return args
}

// NewBackendAdapter wraps an agent.Backend as a tool.AgentBackend
// so it can be registered with AgentTool for external subprocess execution.
func NewBackendAdapter(b Backend) tool.AgentBackend {
	return &backendAdapter{b: b}
}

type backendAdapter struct {
	b Backend
}

func (a *backendAdapter) Name() string { return a.b.Name() }

func (a *backendAdapter) Execute(ctx context.Context, task tool.AgentTask) (*tool.AgentResult, error) {
	agentTask := AgentTask{
		Prompt:   task.Prompt,
		Model:    task.Model,
		Provider: task.Provider,
		Timeout:  task.Timeout,
	}
	result, err := a.b.Execute(ctx, agentTask)
	if err != nil {
		return nil, err
	}
	return &tool.AgentResult{
		Content:      result.Content,
		ModelUsed:    result.ModelUsed,
		Provider:     result.Provider,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		CostUSD:      result.CostUSD,
		Duration:     result.Duration,
		Error:        result.Error,
	}, nil
}
