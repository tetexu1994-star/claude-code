package agent

import (
	"testing"
	"time"
)

func TestAgentDefinition_HasAllTools(t *testing.T) {
	tests := []struct {
		name  string
		tools []string
		want  bool
	}{
		{"all tools with star", []string{"*"}, true},
		{"all tools with star and others", []string{"Read", "*", "Bash"}, true},
		{"specific tools only", []string{"Read", "Glob", "Grep"}, false},
		{"empty tools", []string{}, false},
		{"nil tools", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &AgentDefinition{Tools: tt.tools}
			if got := def.HasAllTools(); got != tt.want {
				t.Errorf("HasAllTools() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgentState_Values(t *testing.T) {
	states := []AgentState{
		AgentPending,
		AgentRunning,
		AgentCompleted,
		AgentFailed,
		AgentCancelled,
	}
	expected := []string{"pending", "running", "completed", "failed", "cancelled"}
	for i, s := range states {
		if string(s) != expected[i] {
			t.Errorf("AgentState = %q, want %q", s, expected[i])
		}
	}
}

func TestAgentRun_Initialization(t *testing.T) {
	def := &AgentDefinition{
		AgentType: "test-agent",
		Name:      "Test Agent",
		Source:    "built-in",
		Tools:     []string{"*"},
		MaxTurns:  100,
	}
	run := &AgentRun{
		ID:           "run-001",
		Definition:   def,
		State:        AgentPending,
		Prompt:       "test prompt",
		SystemPrompt: "test system prompt",
		Model:        "claude-sonnet-4",
		Provider:     "anthropic",
		CreatedAt:    time.Now(),
		AbortSignal:  make(chan struct{}),
	}
	if run.ID != "run-001" {
		t.Errorf("ID = %q, want %q", run.ID, "run-001")
	}
	if run.GetState() != AgentPending {
		t.Errorf("State = %q, want %q", run.GetState(), AgentPending)
	}
	if run.Definition.AgentType != "test-agent" {
		t.Errorf("Definition.AgentType = %q, want %q", run.Definition.AgentType, "test-agent")
	}
}

func TestRunOptions_Defaults(t *testing.T) {
	opts := &RunOptions{
		ParentID: "parent-001",
		MaxTurns: 50,
	}
	if opts.ParentID != "parent-001" {
		t.Errorf("ParentID = %q, want %q", opts.ParentID, "parent-001")
	}
	if opts.Isolation != "" {
		t.Errorf("Isolation = %q, want empty", opts.Isolation)
	}
	if opts.AbortSignal != nil {
		t.Error("AbortSignal should be nil by default")
	}
}

func TestModelConfig_Weights(t *testing.T) {
	models := []ModelConfig{
		{Provider: "anthropic", Model: "claude-sonnet-4", Weight: 0.4, Priority: 1},
		{Provider: "openai", Model: "gpt-4o", Weight: 0.3, Priority: 2},
		{Provider: "deepseek", Model: "deepseek-chat", Weight: 0.2, Priority: 3},
		{Provider: "tongyi", Model: "qwen-plus", Weight: 0.1, Priority: 4},
	}
	var totalWeight float64
	for _, m := range models {
		totalWeight += m.Weight
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("total weight = %v, want ~1.0", totalWeight)
	}
}

func TestAggregatedResult_Strategy(t *testing.T) {
	result := &AggregatedResult{
		Strategy:  "consensus",
		Consensus: 0.85,
		Final:     "aggregated answer",
	}
	if result.Strategy != "consensus" {
		t.Errorf("Strategy = %q, want %q", result.Strategy, "consensus")
	}
	if result.Consensus < 0.5 {
		t.Errorf("Consensus too low: %v", result.Consensus)
	}
}
