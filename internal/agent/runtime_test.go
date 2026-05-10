package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
	"github.com/tetexu/tlaude-code/internal/tool"
)

// --- Mock LLM Provider ---

type mockProvider struct {
	name       string
	chatFn     func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error)
	streamFn   func(ctx context.Context, req llm.ChatRequest) (<-chan llm.Chunk, error)
	modelsFn   func() ([]string, error)
	available  bool
	callCount  int
	calls      []llm.ChatRequest
	mu         sync.Mutex
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) IsAvailable() bool { return m.available }

func (m *mockProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	m.mu.Lock()
	m.callCount++
	m.calls = append(m.calls, req)
	m.mu.Unlock()
	if m.chatFn != nil {
		return m.chatFn(ctx, req)
	}
	return &llm.ChatResponse{
		Message:     llm.Message{Role: "assistant", Content: "mock response"},
		Model:       "mock-model",
		InputTokens: 10,
	}, nil
}

func (m *mockProvider) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.Chunk, error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, req)
	}
	ch := make(chan llm.Chunk, 1)
	ch <- llm.Chunk{Content: "mock stream", Done: true}
	close(ch)
	return ch, nil
}

func (m *mockProvider) Models() ([]string, error) {
	if m.modelsFn != nil {
		return m.modelsFn()
	}
	return []string{"mock-model"}, nil
}

// --- Mock Tool ---

type mockTool struct {
	name        string
	description string
	safe        bool
	executeFn   func(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error)
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return t.description }
func (t *mockTool) IsEnabled() bool     { return true }
func (t *mockTool) IsConcurrencySafe() bool { return t.safe }

func (t *mockTool) ToolDefinition() tool.ToolDefinition {
	schema, _ := json.Marshal(map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	})
	return tool.ToolDefinition{
		Name:        t.name,
		Description: t.description,
		InputSchema: json.RawMessage(schema),
	}
}

func (t *mockTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
	if t.executeFn != nil {
		return t.executeFn(ctx, input, toolCtx)
	}
	return &tool.ToolResult{Content: t.name + ":ok"}, nil
}

// --- Test Helpers ---

func newTestRuntime(prov *mockProvider, tools []tool.Tool) *AgentRuntime {
	store := NewAgentDefStore()
	reg := tool.NewRegistry()
	for _, tl := range tools {
		_ = reg.Register(tl)
	}

	// Create an llm.Registry and manually insert the mock provider.
	llmReg := llm.GlobalRegistry()
	// Clear any existing providers and register the mock.
	for _, name := range llmReg.List() {
		llmReg.Unregister(name)
	}

	// We need to register a factory, then we can override the provider.
	// Use a simple approach: clear and re-register.
	llm.RegisterFactory("mock", func(cfg llm.ProviderConfig) (llm.Provider, error) {
		return prov, nil
	})
	factory, _ := llm.GetFactory("mock")
	_ = llmReg.Register("mock", factory)

	return NewAgentRuntime(store, reg, llmReg)
}

func newTestDefinition(agentType string) *AgentDefinition {
	return &AgentDefinition{
		AgentType:   agentType,
		Name:        "Test Agent",
		Description: "A test agent",
		WhenToUse:   "For testing",
		Tools:       []string{"*"},
		MaxTurns:    5,
		Model:       "mock-model",
		Provider:    "mock",
		Source:      "built-in",
	}
}

// --- Tests ---

func TestRunAgent_SimpleResponse(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "Hello, world!"},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}
	rt := newTestRuntime(prov, nil)
	def := newTestDefinition("simple")

	ctx := context.Background()
	run, err := rt.RunAgent(ctx, def, "Say hello", nil)
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
	if run.GetState() != AgentCompleted {
		t.Errorf("State = %q, want %q", run.GetState(), AgentCompleted)
	}
	if run.Result != "Hello, world!" {
		t.Errorf("Result = %q, want %q", run.Result, "Hello, world!")
	}
	if prov.callCount != 1 {
		t.Errorf("callCount = %d, want 1", prov.callCount)
	}
}

func TestRunAgent_ToolCallLoop(t *testing.T) {
	callNum := 0
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			callNum++
			if callNum == 1 {
				// First call: return a tool call.
				return &llm.ChatResponse{
					Message: llm.Message{
						Role:    "assistant",
						Content: "Let me read that file.",
						ToolCalls: []llm.ToolCall{
							{ID: "toolu-1", Name: "read_file", Args: map[string]interface{}{"path": "/tmp/test.go"}},
						},
					},
					Model:        "mock-model",
					InputTokens:  20,
					OutputTokens: 15,
				}, nil
			}
			// Second call: final response.
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "File contents: package main"},
				Model:       "mock-model",
				InputTokens: 30,
			}, nil
		},
	}

	readTool := &mockTool{
		name:        "read_file",
		description: "Read a file",
		safe:        true,
		executeFn: func(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
			return &tool.ToolResult{Content: "package main\nfunc main() {}"}, nil
		},
	}

	rt := newTestRuntime(prov, []tool.Tool{readTool})
	def := newTestDefinition("tool-user")

	ctx := context.Background()
	run, err := rt.RunAgent(ctx, def, "Read /tmp/test.go", nil)
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
	if run.GetState() != AgentCompleted {
		t.Errorf("State = %q, want %q", run.GetState(), AgentCompleted)
	}
	if prov.callCount != 2 {
		t.Errorf("callCount = %d, want 2", prov.callCount)
	}
	// Should have 4 messages: user, assistant(tool_call), tool_result, assistant(final)
	if len(run.Messages) != 4 {
		t.Errorf("len(Messages) = %d, want 4", len(run.Messages))
	}
}

func TestRunAgent_MaxTurnsLimit(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			// Always return a tool call to force more turns.
			return &llm.ChatResponse{
				Message: llm.Message{
					Role:    "assistant",
					Content: "Calling tool.",
					ToolCalls: []llm.ToolCall{
						{ID: "toolu-1", Name: "read_file", Args: map[string]interface{}{"path": "/tmp/test.go"}},
					},
				},
				Model:        "mock-model",
				InputTokens:  10,
				OutputTokens: 5,
			}, nil
		},
	}

	readTool := &mockTool{
		name:        "read_file",
		description: "Read a file",
		safe:        true,
	}

	rt := newTestRuntime(prov, []tool.Tool{readTool})
	def := newTestDefinition("looper")
	def.MaxTurns = 3

	ctx := context.Background()
	run, err := rt.RunAgent(ctx, def, "Loop forever", nil)
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
	if run.GetState() != AgentFailed {
		t.Errorf("State = %q, want %q", run.GetState(), AgentFailed)
	}
	if !strings.Contains(run.Error, "exceeded max turns") {
		t.Errorf("Error = %q, want exceeded max turns", run.Error)
	}
	if prov.callCount != 3 {
		t.Errorf("callCount = %d, want 3", prov.callCount)
	}
}

func TestRunAgent_ToolFilteringWhitelist(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			// Check that only whitelisted tools are in the request.
			for _, tl := range req.Tools {
				if tl.Name != "read_file" {
					t.Errorf("unexpected tool in request: %q", tl.Name)
				}
			}
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "Done."},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}

	readTool := &mockTool{
		name:        "read_file",
		description: "Read a file",
		safe:        true,
	}
	bashTool := &mockTool{
		name:        "bash",
		description: "Run command",
		safe:        false,
	}

	rt := newTestRuntime(prov, []tool.Tool{readTool, bashTool})
	def := newTestDefinition("whitelist-test")
	def.Tools = []string{"read_file"} // Only allow read_file

	ctx := context.Background()
	run, err := rt.RunAgent(ctx, def, "Do something", nil)
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
	if run.GetState() != AgentCompleted {
		t.Errorf("State = %q, want %q", run.GetState(), AgentCompleted)
	}
	// Verify: only read_file should be in the tools sent to LLM.
	if prov.callCount != 1 {
		t.Fatalf("callCount = %d, want 1", prov.callCount)
	}
	req := prov.calls[0]
	if len(req.Tools) != 1 {
		t.Errorf("len(Tools) = %d, want 1", len(req.Tools))
	}
	if len(req.Tools) > 0 && req.Tools[0].Name != "read_file" {
		t.Errorf("Tools[0].Name = %q, want read_file", req.Tools[0].Name)
	}
}

func TestRunAgent_ToolFilteringDisallowed(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			for _, tl := range req.Tools {
				if tl.Name == "Agent" {
					t.Error("Agent tool should be disallowed and not in request")
				}
			}
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "Done."},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}

	agentTool := &mockTool{name: "Agent", description: "Spawn sub-agent", safe: false}
	readTool := &mockTool{name: "read_file", description: "Read file", safe: true}

	rt := newTestRuntime(prov, []tool.Tool{agentTool, readTool})
	def := newTestDefinition("no-recursion")
	def.Tools = []string{"*"}
	def.DisallowedTools = []string{"Agent"}

	ctx := context.Background()
	run, err := rt.RunAgent(ctx, def, "Do something", nil)
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
	if run.GetState() != AgentCompleted {
		t.Errorf("State = %q, want %q", run.GetState(), AgentCompleted)
	}
}

func TestRunAgent_ToolExecutionError(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message: llm.Message{
					Role:    "assistant",
					Content: "Let me try.",
					ToolCalls: []llm.ToolCall{
						{ID: "toolu-1", Name: "bash", Args: map[string]interface{}{"command": "rm -rf /"}},
					},
				},
				Model:        "mock-model",
				InputTokens:  10,
				OutputTokens: 5,
			}, nil
		},
	}

	bashTool := &mockTool{
		name:        "bash",
		description: "Run command",
		safe:        false,
		executeFn: func(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
			return &tool.ToolResult{IsError: true, Content: "permission denied"}, nil
		},
	}

	rt := newTestRuntime(prov, []tool.Tool{bashTool})
	def := newTestDefinition("error-recovery")
	def.MaxTurns = 2

	ctx := context.Background()
	run, err := rt.RunAgent(ctx, def, "Run command", nil)
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
	// Should fail because all tool calls errored and max_turns not exceeded.
	if run.GetState() != AgentFailed {
		t.Errorf("State = %q, want %q", run.GetState(), AgentFailed)
	}
	if !strings.Contains(run.Error, "all tool calls failed") {
		t.Errorf("Error = %q, want all tool calls failed", run.Error)
	}
}

func TestRunAgent_LLMError(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, context.DeadlineExceeded
		},
	}

	rt := newTestRuntime(prov, nil)
	def := newTestDefinition("error-test")

	ctx := context.Background()
	run, err := rt.RunAgent(ctx, def, "Test", nil)
	if err == nil {
		t.Fatal("expected error from LLM failure")
	}
	if run.GetState() != AgentFailed {
		t.Errorf("State = %q, want %q", run.GetState(), AgentFailed)
	}
}

func TestRunAgent_Cancellation(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message: llm.Message{
					Role:    "assistant",
					Content: "Calling tool.",
					ToolCalls: []llm.ToolCall{
						{ID: "toolu-1", Name: "read_file", Args: map[string]interface{}{"path": "/tmp/test.go"}},
					},
				},
				Model:        "mock-model",
				InputTokens:  10,
				OutputTokens: 5,
			}, nil
		},
	}

	var started chan struct{}
	readTool := &mockTool{
		name:        "read_file",
		description: "Read file",
		safe:        true,
		executeFn: func(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
			// Signal that the tool has started.
			close(started)
			// Wait for abort signal.
			<-ctx.Done()
			return &tool.ToolResult{IsError: true, Content: "cancelled"}, ctx.Err()
		},
	}

	rt := newTestRuntime(prov, []tool.Tool{readTool})
	def := newTestDefinition("cancellable")

	ctx := context.Background()
	started = make(chan struct{})
	agentID, err := rt.RunAgentAsync(ctx, def, "Test", nil)
	if err != nil {
		t.Fatalf("RunAgentAsync failed: %v", err)
	}

	// Wait for the tool to start executing.
	<-started

	// Stop the agent while the tool is running.
	err = rt.StopAgent(agentID)
	if err != nil {
		t.Fatalf("StopAgent failed: %v", err)
	}

	// Wait for cancellation to propagate.
	time.Sleep(100 * time.Millisecond)

	run, ok := rt.GetAgent(agentID)
	if !ok {
		t.Fatal("agent not found after cancellation")
	}
	if run.GetState() != AgentCancelled {
		t.Errorf("State = %q, want %q", run.GetState(), AgentCancelled)
	}
}

func TestRunAgent_SystemPrompt(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			if req.System != "custom system prompt" {
				t.Errorf("System = %q, want %q", req.System, "custom system prompt")
			}
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "OK"},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}

	rt := newTestRuntime(prov, nil)
	def := newTestDefinition("sysprompt")
	def.SystemPrompt = "custom system prompt"

	ctx := context.Background()
	_, err := rt.RunAgent(ctx, def, "Test", nil)
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
}

func TestRunAgentAsync(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			time.Sleep(20 * time.Millisecond)
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "async result"},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}

	rt := newTestRuntime(prov, nil)
	def := newTestDefinition("async-test")

	ctx := context.Background()
	agentID, err := rt.RunAgentAsync(ctx, def, "Test async", nil)
	if err != nil {
		t.Fatalf("RunAgentAsync failed: %v", err)
	}
	if agentID == "" {
		t.Fatal("agentID is empty")
	}

	// Should be found immediately (pending or running).
	run, ok := rt.GetAgent(agentID)
	if !ok {
		t.Fatal("agent not found in active list")
	}
	if run.GetState() != AgentPending && run.GetState() != AgentRunning {
		t.Errorf("State = %q, want pending or running", run.GetState())
	}

	// Wait for completion.
	time.Sleep(100 * time.Millisecond)

	run, ok = rt.GetAgent(agentID)
	if !ok {
		t.Fatal("agent not found after completion")
	}
	if run.GetState() != AgentCompleted {
		t.Errorf("State = %q, want %q", run.GetState(), AgentCompleted)
	}
	if run.Result != "async result" {
		t.Errorf("Result = %q, want %q", run.Result, "async result")
	}
}

func TestAgentRuntime_ListAgents(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			time.Sleep(20 * time.Millisecond)
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "done"},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}

	rt := newTestRuntime(prov, nil)
	def := newTestDefinition("list-test")

	ctx := context.Background()

	// Start 3 async agents.
	var ids []string
	for i := 0; i < 3; i++ {
		id, err := rt.RunAgentAsync(ctx, def, "test", nil)
		if err != nil {
			t.Fatalf("RunAgentAsync failed: %v", err)
		}
		ids = append(ids, id)
	}

	list := rt.ListAgents()
	if len(list) < 3 {
		t.Errorf("ListAgents() = %d, want at least 3", len(list))
	}

	// Wait for them to complete.
	time.Sleep(100 * time.Millisecond)

	// All should have completed (but still in active map since async stores results).
	for _, id := range ids {
		run, ok := rt.GetAgent(id)
		if !ok {
			t.Errorf("agent %q not found", id)
			continue
		}
		if run.GetState() != AgentCompleted {
			t.Errorf("agent %q State = %q, want %q", id, run.GetState(), AgentCompleted)
		}
	}
}

func TestAgentRuntime_StopAgent(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			// Block until cancelled.
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	rt := newTestRuntime(prov, nil)
	def := newTestDefinition("stop-test")

	ctx := context.Background()
	agentID, err := rt.RunAgentAsync(ctx, def, "test", nil)
	if err != nil {
		t.Fatalf("RunAgentAsync failed: %v", err)
	}

	// Give it a moment to start.
	time.Sleep(10 * time.Millisecond)

	err = rt.StopAgent(agentID)
	if err != nil {
		t.Fatalf("StopAgent failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	run, ok := rt.GetAgent(agentID)
	if !ok {
		t.Fatal("agent not found after stop")
	}
	if run.GetState() != AgentCancelled {
		t.Errorf("State = %q, want %q", run.GetState(), AgentCancelled)
	}
}

func TestAgentRuntime_StopAgent_AlreadyCompleted(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "done"},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}

	rt := newTestRuntime(prov, nil)
	def := newTestDefinition("completed-test")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	run, err := rt.RunAgent(ctx, def, "test", nil)
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}

	// Try to stop an already-completed agent (not in active map since RunAgent clears it).
	err = rt.StopAgent(run.ID)
	if err == nil {
		t.Error("expected error when stopping completed agent")
	}
}

func TestAgentRuntime_NilDefinition(t *testing.T) {
	rt := newTestRuntime(nil, nil)

	_, err := rt.RunAgent(context.Background(), nil, "test", nil)
	if err == nil {
		t.Fatal("expected error for nil definition")
	}
}

func TestAgentRuntime_ProviderNotFound(t *testing.T) {
	rt := newTestRuntime(nil, nil)
	def := &AgentDefinition{
		AgentType:   "test",
		Name:        "Test",
		Description: "Test",
		WhenToUse:   "Test",
		Tools:       []string{"*"},
		MaxTurns:    5,
		Provider:    "nonexistent",
		Source:      "built-in",
	}

	_, err := rt.RunAgent(context.Background(), def, "test", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Error = %q, want 'not found'", err.Error())
	}
}

func TestRunAgent_OpMaxTurnsOverride(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			// Always return a tool call.
			return &llm.ChatResponse{
				Message: llm.Message{
					Role:    "assistant",
					Content: "Calling tool.",
					ToolCalls: []llm.ToolCall{
						{ID: "toolu-1", Name: "read_file", Args: map[string]interface{}{"path": "/tmp/test.go"}},
					},
				},
				Model:        "mock-model",
				InputTokens:  10,
				OutputTokens: 5,
			}, nil
		},
	}

	readTool := &mockTool{
		name:        "read_file",
		description: "Read file",
		safe:        true,
	}

	rt := newTestRuntime(prov, []tool.Tool{readTool})
	def := newTestDefinition("override-test")
	def.MaxTurns = 100 // high limit, but opts override to 2

	ctx := context.Background()
	run, err := rt.RunAgent(ctx, def, "Test", &RunOptions{MaxTurns: 2})
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
	if run.GetState() != AgentFailed {
		t.Errorf("State = %q, want %q", run.GetState(), AgentFailed)
	}
	if !strings.Contains(run.Error, "exceeded max turns") {
		t.Errorf("Error = %q, want exceeded max turns", run.Error)
	}
	if prov.callCount != 2 {
		t.Errorf("callCount = %d, want 2 (overridden by opts)", prov.callCount)
	}
}

func TestRunAgent_OpToolsOverride(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			if len(req.Tools) != 1 || req.Tools[0].Name != "read_file" {
				t.Errorf("expected only read_file tool, got %d tools", len(req.Tools))
			}
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "Done."},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}

	readTool := &mockTool{name: "read_file", description: "Read", safe: true}
	bashTool := &mockTool{name: "bash", description: "Bash", safe: false}

	rt := newTestRuntime(prov, []tool.Tool{readTool, bashTool})
	def := newTestDefinition("opts-tools")
	def.Tools = []string{"*"} // definition allows all

	ctx := context.Background()
	// Override with opts: only read_file.
	_, err := rt.RunAgent(ctx, def, "Test", &RunOptions{Tools: []string{"read_file"}})
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
}

func TestRunAgent_MultipleToolCalls(t *testing.T) {
	callNum := 0
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			callNum++
			if callNum == 1 {
				return &llm.ChatResponse{
					Message: llm.Message{
						Role:    "assistant",
						Content: "Let me check multiple things.",
						ToolCalls: []llm.ToolCall{
							{ID: "toolu-1", Name: "read_file", Args: map[string]interface{}{"path": "/a.go"}},
							{ID: "toolu-2", Name: "read_file", Args: map[string]interface{}{"path": "/b.go"}},
							{ID: "toolu-3", Name: "read_file", Args: map[string]interface{}{"path": "/c.go"}},
						},
					},
					Model:        "mock-model",
					InputTokens:  20,
					OutputTokens: 20,
				}, nil
			}
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "All files read."},
				Model:       "mock-model",
				InputTokens: 40,
			}, nil
		},
	}

	readTool := &mockTool{
		name:        "read_file",
		description: "Read file",
		safe:        true,
		executeFn: func(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
			var params struct{ Path string }
			json.Unmarshal(input, &params)
			return &tool.ToolResult{Content: params.Path + " contents"}, nil
		},
	}

	rt := newTestRuntime(prov, []tool.Tool{readTool})
	def := newTestDefinition("multi-tool")

	ctx := context.Background()
	run, err := rt.RunAgent(ctx, def, "Read multiple files", nil)
	if err != nil {
		t.Fatalf("RunAgent failed: %v", err)
	}
	if run.GetState() != AgentCompleted {
		t.Errorf("State = %q, want %q", run.GetState(), AgentCompleted)
	}
	// Messages: user, assistant(3 tool calls), 3 tool_results, assistant(final) = 6
	if len(run.Messages) != 6 {
		t.Errorf("len(Messages) = %d, want 6", len(run.Messages))
		for i, m := range run.Messages {
			t.Logf("  [%d] role=%s content=%s toolCalls=%d", i, m.Role, m.Content, len(m.ToolCalls))
		}
	}
}

func TestAgentRuntime_GetAgent_NotFound(t *testing.T) {
	rt := newTestRuntime(nil, nil)
	_, ok := rt.GetAgent("nonexistent")
	if ok {
		t.Error("expected false for nonexistent agent")
	}
}
