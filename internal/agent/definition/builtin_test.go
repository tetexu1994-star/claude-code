package definition

import (
	"testing"
)

func TestGetBuiltInAgents(t *testing.T) {
	agents := GetBuiltInAgents()

	if len(agents) != 4 {
		t.Fatalf("expected 4 built-in agents, got %d", len(agents))
	}

	wantTypes := map[string]bool{
		"general-purpose": false,
		"cli-guide":       false,
		"explore":         false,
		"plan":            false,
	}

	for _, a := range agents {
		if a.AgentType == "" {
			t.Error("agent has empty AgentType")
		}
		if a.Name == "" {
			t.Errorf("agent %q has empty Name", a.AgentType)
		}
		if a.Description == "" {
			t.Errorf("agent %q has empty Description", a.AgentType)
		}
		if a.Source != SourceBuiltIn {
			t.Errorf("agent %q has Source = %q, want %q", a.AgentType, a.Source, SourceBuiltIn)
		}
		if a.SystemPrompt == "" {
			t.Errorf("agent %q has empty SystemPrompt", a.AgentType)
		}
		if _, ok := wantTypes[a.AgentType]; ok {
			wantTypes[a.AgentType] = true
		}
	}

	for typ, found := range wantTypes {
		if !found {
			t.Errorf("expected built-in agent type %q not found", typ)
		}
	}
}

func TestGetBuiltInAgents_ExploreBackground(t *testing.T) {
	agents := GetBuiltInAgents()
	for _, a := range agents {
		if a.AgentType == "explore" && !a.Background {
			t.Error("explore agent should have Background=true")
		}
	}
}

func TestGetBuiltInAgents_GeneralPurposeAllTools(t *testing.T) {
	agents := GetBuiltInAgents()
	for _, a := range agents {
		if a.AgentType == "general-purpose" && a.Tools != nil {
			t.Error("general-purpose agent should have nil Tools (all tools)")
		}
	}
}

func TestBuiltInAgents_HaveValidFields(t *testing.T) {
	agents := GetBuiltInAgents()
	for _, a := range agents {
		if a.AgentType == "" {
			t.Error("empty AgentType")
		}
		if a.Name == "" {
			t.Error("empty Name")
		}
		if a.Description == "" {
			t.Error("empty Description")
		}
		if a.SystemPrompt == "" {
			t.Error("empty SystemPrompt")
		}
		if a.WhenToUse == "" {
			t.Error("empty WhenToUse")
		}
	}
}

func TestBuiltInAgentTypes(t *testing.T) {
	agents := GetBuiltInAgents()
	types := make([]string, len(agents))
	for i, a := range agents {
		types[i] = a.AgentType
	}

	expected := []string{"general-purpose", "cli-guide", "explore", "plan"}
	for i, want := range expected {
		if types[i] != want {
			t.Errorf("agent type at index %d = %q, want %q", i, types[i], want)
		}
	}
}
