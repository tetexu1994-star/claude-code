package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
)

// --- Agent×Model Matrix Tests ---

func TestResolveModelProvider_ModelRef(t *testing.T) {
	prov := &mockProvider{name: "anthropic", available: true}
	rt := newTestRuntime(prov, nil)

	tests := []struct {
		name         string
		def          *AgentDefinition
		opts         *RunOptions
		wantModel    string
		wantProvider string
	}{
		{
			name: "ModelRef takes precedence over legacy fields",
			def: &AgentDefinition{
				AgentType: "test",
				Model:     "old-model",
				Provider:  "old-provider",
				ModelRef: ModelRef{
					Model:    "new-model",
					Provider: "new-provider",
				},
			},
			wantModel:    "new-model",
			wantProvider: "new-provider",
		},
		{
			name: "inherit from session defaults",
			def: &AgentDefinition{
				AgentType: "test",
				ModelRef:  ModelRef{},
			},
			opts: &RunOptions{
				SessionModel:    "session-model",
				SessionProvider: "session-provider",
			},
			wantModel:    "session-model",
			wantProvider: "session-provider",
		},
		{
			name: "explicit 'inherit' falls back to session",
			def: &AgentDefinition{
				AgentType: "test",
				ModelRef: ModelRef{
					Model:    "inherit",
					Provider: "inherit",
				},
			},
			opts: &RunOptions{
				SessionModel:    "session-model",
				SessionProvider: "session-provider",
			},
			wantModel:    "session-model",
			wantProvider: "session-provider",
		},
		{
			name: "partial inheritance: model specified, provider inherited",
			def: &AgentDefinition{
				AgentType: "test",
				ModelRef: ModelRef{
					Model:    "specific-model",
					Provider: "",
				},
			},
			opts: &RunOptions{
				SessionProvider: "session-provider",
			},
			wantModel:    "specific-model",
			wantProvider: "session-provider",
		},
		{
			name: "hardcoded defaults when nothing specified",
			def: &AgentDefinition{
				AgentType: "test",
			},
			wantModel:    "claude-sonnet-4",
			wantProvider: "anthropic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, provider := rt.resolveModelProvider(tt.def, tt.opts)
			if model != tt.wantModel {
				t.Errorf("model = %q, want %q", model, tt.wantModel)
			}
			if provider != tt.wantProvider {
				t.Errorf("provider = %q, want %q", provider, tt.wantProvider)
			}
		})
	}
}

func TestResolveModelProvider_LegacyFallback(t *testing.T) {
	prov := &mockProvider{name: "mock", available: true}
	rt := newTestRuntime(prov, nil)

	def := &AgentDefinition{
		AgentType: "test",
		Model:     "legacy-model",
		Provider:  "legacy-provider",
	}

	model, provider := rt.resolveModelProvider(def, nil)
	if model != "legacy-model" {
		t.Errorf("model = %q, want %q", model, "legacy-model")
	}
	if provider != "legacy-provider" {
		t.Errorf("provider = %q, want %q", provider, "legacy-provider")
	}
}

// --- MoA Multi-Model Tests ---

func TestRunMoA_SingleMock(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "MoA response"},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}
	rt := newTestRuntime(prov, nil)
	def := &AgentDefinition{
		AgentType: "moa",
		Name:      "Test MoA",
		Tools:     []string{},
		MaxTurns:  5,
		ModelRef: ModelRef{
			Model:    "mock-model",
			Provider: "mock",
		},
	}

	models := []ModelConfig{
		{Provider: "mock", Model: "mock-model", Weight: 1.0},
	}

	ctx := context.Background()
	result, err := rt.RunMoA(ctx, def, "Test prompt", nil, models)
	if err != nil {
		t.Fatalf("RunMoA failed: %v", err)
	}
	if result.Final == "" {
		t.Error("expected non-empty final result")
	}
	if len(result.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if result.TokenCost < 0 {
		t.Error("expected non-negative token cost")
	}
}

func TestRunMoA_UsesModelStrategy(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "model says hi"},
				Model:       req.Model,
				InputTokens: 10,
			}, nil
		},
	}
	rt := newTestRuntime(prov, nil)

	def := &AgentDefinition{
		AgentType: "moa",
		Name:      "MoA with Strategy",
		Tools:     []string{},
		MaxTurns:  5,
		ModelRef: ModelRef{
			Model:    "mock-model",
			Provider: "mock",
		},
		ModelStrategy: []ModelRef{
			{Provider: "mock", Model: "mock-model", Reason: "main"},
			{Provider: "mock", Model: "mock-model", Reason: "second"},
		},
	}

	ctx := context.Background()
	result, err := rt.RunMoA(ctx, def, "Test", nil, nil)
	if err != nil {
		t.Fatalf("RunMoA failed: %v", err)
	}
	if len(result.Results) != 2 {
		t.Errorf("expected 2 results from ModelStrategy, got %d", len(result.Results))
	}
	if len(result.ModelCfgs) != 2 {
		t.Errorf("expected 2 model configs, got %d", len(result.ModelCfgs))
	}
}

func TestRunMoA_MajorityVote(t *testing.T) {
	var callCount atomic.Int32
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			callCount.Add(1)
			content := "Unique"
			if callCount.Load() <= 2 {
				content = "Majority wins"
			}
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: content},
				Model:       req.Model,
				InputTokens: 10,
			}, nil
		},
	}
	rt := newTestRuntime(prov, nil)

	def := &AgentDefinition{
		AgentType: "moa",
		Name:      "Majority Test",
		Tools:     []string{},
		MaxTurns:  5,
		ModelRef: ModelRef{
			Provider: "mock",
			Model:    "mock-model",
		},
		ModelStrategy: []ModelRef{
			{Provider: "mock", Model: "mock-model"},
			{Provider: "mock", Model: "mock-model"},
			{Provider: "mock", Model: "mock-model"},
		},
	}

	ctx := context.Background()
	result, err := rt.RunMoA(ctx, def, "Test", nil, nil)
	if err != nil {
		t.Fatalf("RunMoA failed: %v", err)
	}
	if result.Strategy != string(MoAMajority) {
		t.Errorf("Strategy = %q, want %q", result.Strategy, MoAMajority)
	}
	if !strings.Contains(result.Final, "Majority wins") {
		t.Errorf("Final = %q, want to contain 'Majority wins'", result.Final)
	}
}

func TestRunMoA_WithExplicitModels(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "response"},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}
	rt := newTestRuntime(prov, nil)

	def := &AgentDefinition{
		AgentType: "general",
		Name:      "Test",
		Tools:     []string{},
		MaxTurns:  5,
	}

	models := []ModelConfig{
		{Provider: "mock", Model: "mock-model", Weight: 1.0},
	}

	ctx := context.Background()
	result, err := rt.RunMoA(ctx, def, "Test", nil, models)
	if err != nil {
		t.Fatalf("RunMoA failed: %v", err)
	}
	if len(result.Results) == 0 {
		t.Error("expected at least 1 result")
	}
}

func TestRunMoA_AllModelsFail(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return nil, context.DeadlineExceeded
		},
	}
	rt := newTestRuntime(prov, nil)

	def := &AgentDefinition{
		AgentType: "moa",
		Name:      "Failing MoA",
		Tools:     []string{},
		MaxTurns:  5,
		ModelRef: ModelRef{
			Provider: "mock",
			Model:    "mock-model",
		},
		ModelStrategy: []ModelRef{
			{Provider: "mock", Model: "mock-model"},
		},
	}

	ctx := context.Background()
	_, err := rt.RunMoA(ctx, def, "Test", nil, nil)
	if err == nil {
		t.Fatal("expected error when all models fail")
	}
}

// --- SelectModelsForTask Tests ---

func TestSelectModelsForTask(t *testing.T) {
	tests := []struct {
		agentType string
		wantMin   int
	}{
		{"explore", 1},
		{"code", 1},
		{"review", 2},
		{"moa", 3},
		{"general", 1},
	}

	for _, tt := range tests {
		t.Run(tt.agentType, func(t *testing.T) {
			models := SelectModelsForTask(tt.agentType, nil)
			if len(models) < tt.wantMin {
				t.Errorf("SelectModelsForTask(%q) = %d models, want at least %d",
					tt.agentType, len(models), tt.wantMin)
			}
		})
	}
}

// --- Built-In Agent Definition Tests ---

func TestBuiltInAgents_HaveModelRefs(t *testing.T) {
	agents := BuiltInAgents()

	for _, a := range agents {
		switch a.AgentType {
		case "general":
			if a.ModelRef.Provider != "" || a.ModelRef.Model != "" {
				t.Errorf("general agent should have empty ModelRef (inherit), got provider=%q model=%q",
					a.ModelRef.Provider, a.ModelRef.Model)
			}
		case "explore":
			if a.ModelRef.Provider != "deepseek" {
				t.Errorf("explore agent should use deepseek, got %q", a.ModelRef.Provider)
			}
			if a.ModelRef.Model != "deepseek-chat" {
				t.Errorf("explore agent should use deepseek-chat, got %q", a.ModelRef.Model)
			}
		case "code":
			if a.ModelRef.Provider != "anthropic" {
				t.Errorf("code agent should use anthropic, got %q", a.ModelRef.Provider)
			}
		case "review":
			if a.ModelRef.Provider != "openai" {
				t.Errorf("review agent should use openai, got %q", a.ModelRef.Provider)
			}
		case "moa":
			if len(a.ModelStrategy) < 2 {
				t.Errorf("moa agent should have at least 2 model strategies, got %d", len(a.ModelStrategy))
			}
		}
	}
}

func TestBuiltInAgents_Consistency(t *testing.T) {
	agents := BuiltInAgents()
	types := BuiltInAgentTypes()

	if len(agents) != len(types) {
		t.Errorf("agent count mismatch: %d definitions vs %d types", len(agents), len(types))
	}

	seen := make(map[string]bool)
	for _, a := range agents {
		if seen[a.AgentType] {
			t.Errorf("duplicate agent type: %q", a.AgentType)
		}
		seen[a.AgentType] = true
	}
}

// --- RunAgentByType / RunMoAByType Bridge Tests ---

func TestRunAgentByType(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "Hello from general agent"},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}
	rt := newTestRuntime(prov, nil)

	ctx := context.Background()
	content, in, out, err := rt.RunAgentByType(ctx, "general", "Say hello", "mock-model", "mock")
	if err != nil {
		t.Fatalf("RunAgentByType failed: %v", err)
	}
	if content != "Hello from general agent" {
		t.Errorf("content = %q, want %q", content, "Hello from general agent")
	}
	if in <= 0 {
		t.Errorf("inputTokens = %d, want > 0", in)
	}
	_ = out
}

func TestRunAgentByType_NotFound(t *testing.T) {
	prov := &mockProvider{name: "mock", available: true}
	rt := newTestRuntime(prov, nil)

	_, _, _, err := rt.RunAgentByType(context.Background(), "nonexistent", "test", "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent agent type")
	}
}

func TestRunMoAByType(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message:     llm.Message{Role: "assistant", Content: "MoA reply"},
				Model:       "mock-model",
				InputTokens: 10,
			}, nil
		},
	}
	rt := newTestRuntime(prov, nil)

	// Register additional mock providers so ModelStrategy can resolve them.
	for _, name := range []string{"anthropic", "deepseek", "openai"} {
		p := &mockProvider{
			name:      name,
			available: true,
			chatFn: prov.chatFn,
		}
		llm.RegisterFactory(name, func(cfg llm.ProviderConfig) (llm.Provider, error) {
			return p, nil
		})
		f, _ := llm.GetFactory(name)
		_ = llm.GlobalRegistry().Register(name, f)
	}

	ctx := context.Background()
	content, timeCost, tokenCost, err := rt.RunMoAByType(ctx, "moa", "Test")
	if err != nil {
		t.Fatalf("RunMoAByType failed: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty content")
	}
	if timeCost <= 0 {
		t.Error("expected positive time cost")
	}
	_ = tokenCost
}

func TestStore(t *testing.T) {
	prov := &mockProvider{name: "mock", available: true}
	rt := newTestRuntime(prov, nil)

	store := rt.Store()
	if store == nil {
		t.Fatal("Store() returned nil")
	}
	if store.Count() == 0 {
		t.Error("store should have built-in agents")
	}
}

// --- Backend Tests ---

func TestInProcessBackend_Execute(t *testing.T) {
	prov := &mockProvider{
		name:      "mock",
		available: true,
		chatFn: func(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
			return &llm.ChatResponse{
				Message:      llm.Message{Role: "assistant", Content: "Backend response"},
				Model:        "mock-model",
				InputTokens:  5,
				OutputTokens: 10,
			}, nil
		},
	}

	// Register the mock provider in the global registry.
	llmReg := llm.GlobalRegistry()
	for _, name := range llmReg.List() {
		llmReg.Unregister(name)
	}
	llm.RegisterFactory("mock", func(cfg llm.ProviderConfig) (llm.Provider, error) {
		return prov, nil
	})
	factory, _ := llm.GetFactory("mock")
	_ = llmReg.Register("mock", factory)

	backend := NewInProcessBackend(llmReg, nil)
	if backend.Name() != "inprocess" {
		t.Errorf("Name = %q, want %q", backend.Name(), "inprocess")
	}

	caps := backend.Capabilities()
	if len(caps) == 0 {
		t.Error("expected non-empty capabilities")
	}

	task := AgentTask{
		Def: &AgentDefinition{
			AgentType: "test",
			Name:      "Test",
		},
		Prompt:   "Hello",
		Provider: "mock",
		Model:    "mock-model",
		Timeout:  5 * time.Second,
	}

	ctx := context.Background()
	result, err := backend.Execute(ctx, task)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Content != "Backend response" {
		t.Errorf("Content = %q, want %q", result.Content, "Backend response")
	}
	if result.Provider != "mock" {
		t.Errorf("Provider = %q, want %q", result.Provider, "mock")
	}
}

func TestInProcessBackend_ProviderNotFound(t *testing.T) {
	llmReg := llm.GlobalRegistry()
	for _, name := range llmReg.List() {
		llmReg.Unregister(name)
	}

	backend := NewInProcessBackend(llmReg, nil)
	task := AgentTask{
		Prompt:   "Test",
		Provider: "nonexistent",
		Timeout:  1 * time.Second,
	}

	ctx := context.Background()
	_, err := backend.Execute(ctx, task)
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
}

func TestSelectModelsForTask_Coverage(t *testing.T) {
	// All known types.
	for _, at := range []string{"explore", "code", "review", "moa", "general", "unknown"} {
		models := SelectModelsForTask(at, nil)
		if len(models) == 0 {
			t.Errorf("SelectModelsForTask(%q) returned empty", at)
		}
	}
}

func TestMajorityVoteResults(t *testing.T) {
	results := []*AgentRun{
		{Result: "A", Provider: "p1"},
		{Result: "A", Provider: "p2"},
		{Result: "B", Provider: "p3"},
	}
	content, count := majorityVoteResults(results)
	if content != "A" {
		t.Errorf("content = %q, want %q", content, "A")
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestFilterSuccess(t *testing.T) {
	results := []*AgentRun{
		{State: AgentCompleted, Result: "ok1"},
		{State: AgentFailed, Result: ""},
		{State: AgentCompleted, Result: "ok2"},
		{State: AgentCancelled, Result: ""},
	}
	success := filterSuccess(results)
	if len(success) != 2 {
		t.Errorf("len(success) = %d, want 2", len(success))
	}
}

func TestBuildConsensusReport(t *testing.T) {
	results := []*AgentRun{
		{Result: "Answer A", Provider: "p1", Model: "m1"},
		{Result: "Answer B", Provider: "p2", Model: "m2"},
	}
	report := buildConsensusReport(results)
	if !strings.Contains(report, "No consensus") {
		t.Error("expected 'No consensus' in report")
	}
	if !strings.Contains(report, "Answer A") {
		t.Error("expected 'Answer A' in report")
	}
}

func TestBuildSynthesis(t *testing.T) {
	results := []*AgentRun{
		{Result: "Response 1", Provider: "p1", Model: "m1"},
	}
	s := buildSynthesis(results)
	if !strings.Contains(s, "Synthesize") && !strings.Contains(s, "synthesize") {
		t.Error("expected 'Synthesize' in output")
	}
	if !strings.Contains(s, "Response 1") {
		t.Error("expected 'Response 1' in output")
	}
}

func TestSubprocessBackend_BuildArgs(t *testing.T) {
	b := &SubprocessBackend{
		Cmd:   "test-cmd",
		Args:  []string{"-z"},
		Label: "test",
	}

	task := AgentTask{
		Model:    "claude-sonnet-4",
		Provider: "anthropic",
	}

	args := b.buildArgs(task)
	if len(args) < 1 {
		t.Fatal("expected at least 1 arg")
	}

	// Verify model and provider args are appended.
	hasModel := false
	hasProvider := false
	for i, a := range args {
		if a == "--model" && i+1 < len(args) && args[i+1] == "claude-sonnet-4" {
			hasModel = true
		}
		if a == "--provider" && i+1 < len(args) && args[i+1] == "anthropic" {
			hasProvider = true
		}
	}
	if !hasModel {
		t.Error("expected --model flag in args")
	}
	if !hasProvider {
		t.Error("expected --provider flag in args")
	}
}

func TestSubprocessBackend_SkipInherit(t *testing.T) {
	b := &SubprocessBackend{
		Cmd:  "test-cmd",
		Args: []string{"-z"},
	}

	task := AgentTask{
		Model:    "inherit",
		Provider: "inherit",
	}

	args := b.buildArgs(task)
	for _, a := range args {
		if a == "--model" || a == "--provider" {
			t.Error("should not add model/provider flags when value is 'inherit'")
		}
	}
}

func TestNewClaudeCodeBackend(t *testing.T) {
	b := NewClaudeCodeBackend(60 * time.Second)
	if b.Name() != "claude-code" {
		t.Errorf("Name = %q, want %q", b.Name(), "claude-code")
	}
	if b.Cmd != "npx" {
		t.Errorf("Cmd = %q, want %q", b.Cmd, "npx")
	}
	if len(b.Args) < 2 {
		t.Error("expected args for Claude Code backend")
	}
}

func TestNewHermesBackend(t *testing.T) {
	b := NewHermesBackend(60 * time.Second)
	if b.Name() != "hermes" {
		t.Errorf("Name = %q, want %q", b.Name(), "hermes")
	}
	if len(b.Args) < 1 || b.Args[0] != "-z" {
		t.Error("expected '-z' arg for Hermes backend")
	}
}
