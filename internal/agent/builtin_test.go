package agent

import (
	"testing"
)

func TestBuiltInAgents_Count(t *testing.T) {
	agents := BuiltInAgents()
	if len(agents) != 5 {
		t.Errorf("BuiltInAgents() count = %d, want 5", len(agents))
	}
}

func TestBuiltInAgents_Types(t *testing.T) {
	agents := BuiltInAgents()
	types := make(map[string]*AgentDefinition)
	for i := range agents {
		types[agents[i].AgentType] = &agents[i]
	}

	expectedTypes := []string{"general", "explore", "code", "review", "moa"}
	for _, et := range expectedTypes {
		if _, ok := types[et]; !ok {
			t.Errorf("missing agent type %q", et)
		}
	}
}

func TestBuiltInAgents_General(t *testing.T) {
	def := findAgent("general")
	if def == nil {
		t.Fatal("general agent not found")
	}
	if !def.HasAllTools() {
		t.Error("general agent should have all tools (*)")
	}
	if def.MaxTurns != 200 {
		t.Errorf("MaxTurns = %d, want 200", def.MaxTurns)
	}
	if def.Source != "built-in" {
		t.Errorf("Source = %q, want built-in", def.Source)
	}
}

func TestBuiltInAgents_Explore(t *testing.T) {
	def := findAgent("explore")
	if def == nil {
		t.Fatal("explore agent not found")
	}
	if def.HasAllTools() {
		t.Error("explore agent should NOT have all tools")
	}
	if def.MaxTurns != 50 {
		t.Errorf("MaxTurns = %d, want 50", def.MaxTurns)
	}
	if !def.Background {
		t.Error("explore agent should be background by default")
	}
	// Verify explore is read-only (no write tools)
	for _, tool := range def.Tools {
		if tool == "Write" || tool == "Edit" {
			t.Errorf("explore agent should not have write tool: %s", tool)
		}
	}
}

func TestBuiltInAgents_Code(t *testing.T) {
	def := findAgent("code")
	if def == nil {
		t.Fatal("code agent not found")
	}
	if !def.HasAllTools() {
		t.Error("code agent should have all tools")
	}
	// Should disallow Agent tool to prevent recursive spawning
	hasAgentDisallowed := false
	for _, t := range def.DisallowedTools {
		if t == "Agent" {
			hasAgentDisallowed = true
			break
		}
	}
	if !hasAgentDisallowed {
		t.Error("code agent should disallow Agent tool")
	}
}

func TestBuiltInAgents_Review(t *testing.T) {
	def := findAgent("review")
	if def == nil {
		t.Fatal("review agent not found")
	}
	if def.HasAllTools() {
		t.Error("review agent should NOT have all tools (read-only)")
	}
	if def.MaxTurns != 50 {
		t.Errorf("MaxTurns = %d, want 50", def.MaxTurns)
	}
}

func TestBuiltInAgents_MoA(t *testing.T) {
	def := findAgent("moa")
	if def == nil {
		t.Fatal("moa agent not found")
	}
	if def.PermissionMode != "accepts" {
		t.Errorf("PermissionMode = %q, want accepts", def.PermissionMode)
	}
	if def.MaxTurns != 30 {
		t.Errorf("MaxTurns = %d, want 30", def.MaxTurns)
	}
}

func TestBuiltInAgentTypes(t *testing.T) {
	types := BuiltInAgentTypes()
	if len(types) != 5 {
		t.Errorf("BuiltInAgentTypes() count = %d, want 5", len(types))
	}
	for i, expected := range []string{"general", "explore", "code", "review", "moa"} {
		if types[i] != expected {
			t.Errorf("BuiltInAgentTypes()[%d] = %q, want %q", i, types[i], expected)
		}
	}
}

func findAgent(agentType string) *AgentDefinition {
	for _, a := range BuiltInAgents() {
		if a.AgentType == agentType {
			return &a
		}
	}
	return nil
}
