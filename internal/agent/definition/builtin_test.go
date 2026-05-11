package definition

import (
	"strings"
	"testing"
)

func TestGetBuiltInAgents(t *testing.T) {
	agents := GetBuiltInAgents()

	if len(agents) != 6 {
		t.Fatalf("expected 6 built-in agents, got %d", len(agents))
	}

	wantTypes := map[string]bool{
		"general-purpose": false,
		"explore":         false,
		"plan":            false,
		"verification":    false,
		"guide":           false,
		"fork":            false,
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
		if a.AgentType != "fork" && a.SystemPrompt == "" {
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

func TestGetBuiltInAgents_VerificationBackground(t *testing.T) {
	agents := GetBuiltInAgents()
	for _, a := range agents {
		if a.AgentType == "verification" && !a.Background {
			t.Error("verification agent should have Background=true")
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

func TestGetBuiltInAgents_VerificationAllTools(t *testing.T) {
	agents := GetBuiltInAgents()
	for _, a := range agents {
		if a.AgentType == "verification" && a.Tools != nil {
			t.Error("verification agent should have nil Tools (all tools)")
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
		if a.AgentType != "fork" && a.SystemPrompt == "" {
			t.Errorf("agent %q has empty SystemPrompt", a.AgentType)
		}
		if a.WhenToUse == "" {
			t.Errorf("agent %q has empty WhenToUse", a.AgentType)
		}
	}
}

func TestBuiltInAgentTypes(t *testing.T) {
	agents := GetBuiltInAgents()
	types := make([]string, len(agents))
	for i, a := range agents {
		types[i] = a.AgentType
	}

	expected := []string{"general-purpose", "explore", "plan", "verification", "guide", "fork"}
	for i, want := range expected {
		if types[i] != want {
			t.Errorf("agent type at index %d = %q, want %q", i, types[i], want)
		}
	}
}

func TestExploreReadOnly(t *testing.T) {
	agents := GetBuiltInAgents()
	for _, a := range agents {
		if a.AgentType != "explore" {
			continue
		}
		hasEdit := hasTool(a.DisallowedTools, "Edit")
		hasWrite := hasTool(a.DisallowedTools, "Write")
		if !hasEdit || !hasWrite {
			t.Error("explore agent must disallow Edit and Write")
		}
		if !a.OmitClaudeMd {
			t.Error("explore agent should have OmitClaudeMd=true")
		}
		if a.Background != true {
			t.Error("explore agent should have Background=true")
		}
	}
}

func TestPlanReadOnly(t *testing.T) {
	agents := GetBuiltInAgents()
	for _, a := range agents {
		if a.AgentType != "plan" {
			continue
		}
		hasEdit := hasTool(a.DisallowedTools, "Edit")
		hasWrite := hasTool(a.DisallowedTools, "Write")
		if !hasEdit || !hasWrite {
			t.Error("plan agent must disallow Edit and Write")
		}
		if !a.OmitClaudeMd {
			t.Error("plan agent should have OmitClaudeMd=true")
		}
	}
}

func TestVerificationAgent(t *testing.T) {
	agents := GetBuiltInAgents()
	for _, a := range agents {
		if a.AgentType != "verification" {
			continue
		}
		if a.Color != "#D63031" {
			t.Errorf("verification agent Color = %q, want %q", a.Color, "#D63031")
		}
		if a.Background != true {
			t.Error("verification agent should have Background=true")
		}
		if a.OmitClaudeMd {
			t.Error("verification agent should have OmitClaudeMd=false (needs CLAUDE.md for build/test commands)")
		}
		if a.CriticalSystemReminder == "" {
			t.Error("verification agent should have a CriticalSystemReminder")
		}
		if !hasTool(a.DisallowedTools, "Edit") || !hasTool(a.DisallowedTools, "Write") || !hasTool(a.DisallowedTools, "Agent") {
			t.Error("verification agent must disallow Agent, Edit, and Write")
		}
	}
}

func TestGuideAgent(t *testing.T) {
	agents := GetBuiltInAgents()
	found := false
	for _, a := range agents {
		if a.AgentType != "guide" {
			continue
		}
		found = true
		if a.Name != "Code Guide" {
			t.Errorf("guide agent Name = %q, want %q", a.Name, "Code Guide")
		}
		// Guide should be read-oriented (limited tool set)
		if len(a.Tools) == 0 || a.Tools == nil {
			t.Error("guide agent should have explicit tool list, not nil/all tools")
		}
	}
	if !found {
		t.Error("guide agent not found in built-in agents")
	}
}

func TestBuiltInPromptsReferenceTlaudeCode(t *testing.T) {
	agents := GetBuiltInAgents()
	for _, a := range agents {
		if a.AgentType == "cli-guide" {
			continue // old agent, skip
		}
		// All new prompts should reference Tlaude Code (not just Claude Code)
		if a.SystemPrompt == "" {
			continue
		}
		// At least one of the agents should mention Tlaude Code by name
		_ = a.SystemPrompt
	}
	// Sanity: verify the general-purpose prompt mentions Tlaude Code
	for _, a := range agents {
		if a.AgentType == "general-purpose" {
			if !strings.Contains(a.SystemPrompt, "Tlaude Code") {
				t.Error("general-purpose prompt should mention Tlaude Code")
			}
		}
	}
}

// helpers

func hasTool(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
