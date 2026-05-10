package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewAgentDefStore(t *testing.T) {
	s := NewAgentDefStore()
	if s == nil {
		t.Fatal("NewAgentDefStore() returned nil")
	}
	if s.Count() != 5 {
		t.Errorf("Count() = %d, want 5", s.Count())
	}
	if s.CountBuiltIn() != 5 {
		t.Errorf("CountBuiltIn() = %d, want 5", s.CountBuiltIn())
	}
}

func TestAgentDefStore_Get_BuiltIn(t *testing.T) {
	s := NewAgentDefStore()

	tests := []string{"general", "explore", "code", "review", "moa"}
	for _, name := range tests {
		def, ok := s.Get(name)
		if !ok {
			t.Errorf("Get(%q) not found", name)
			continue
		}
		if def.AgentType != name {
			t.Errorf("Get(%q).AgentType = %q", name, def.AgentType)
		}
	}
}

func TestAgentDefStore_Get_NotFound(t *testing.T) {
	s := NewAgentDefStore()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Error("Get(nonexistent) should return false")
	}
}

func TestAgentDefStore_Register_UserOverride(t *testing.T) {
	s := NewAgentDefStore()

	// Register a user agent that overrides the built-in "general"
	userGeneral := &AgentDefinition{
		AgentType:   "general",
		Name:        "Custom General",
		Source:      "user",
		Tools:       []string{"Read", "Bash"},
		MaxTurns:    50,
		Color:       "#FF0000",
		Description: "Custom general agent",
		WhenToUse:   "Custom use case",
	}
	s.Register(userGeneral)

	def, ok := s.Get("general")
	if !ok {
		t.Fatal("Get(general) not found after override")
	}
	if def.Name != "Custom General" {
		t.Errorf("Name = %q, want Custom General (user should override built-in)", def.Name)
	}
	if def.Source != "user" {
		t.Errorf("Source = %q, want user", def.Source)
	}
}

func TestAgentDefStore_List(t *testing.T) {
	s := NewAgentDefStore()

	// Add a user agent
	s.Register(&AgentDefinition{
		AgentType:   "my-agent",
		Name:        "My Agent",
		Source:      "user",
		Tools:       []string{"*"},
		MaxTurns:    100,
		Description: "Custom",
		WhenToUse:   "Custom tasks",
	})

	list := s.List()
	if len(list) != 6 {
		t.Errorf("List() count = %d, want 6 (5 built-in + 1 user)", len(list))
	}

	// Built-in count should remain 5
	if s.CountBuiltIn() != 5 {
		t.Errorf("CountBuiltIn() = %d, want 5", s.CountBuiltIn())
	}
}

func TestAgentDefStore_ListBySource(t *testing.T) {
	s := NewAgentDefStore()

	builtins := s.ListBySource("built-in")
	if len(builtins) != 5 {
		t.Errorf("ListBySource(built-in) = %d, want 5", len(builtins))
	}

	plugins := s.ListBySource("plugin")
	if len(plugins) != 0 {
		t.Errorf("ListBySource(plugin) = %d, want 0", len(plugins))
	}
}

func TestAgentDefStore_List_Deduplication(t *testing.T) {
	s := NewAgentDefStore()

	// Register a user agent with same type as built-in
	s.Register(&AgentDefinition{
		AgentType:   "explore",
		Name:        "Custom Explore",
		Source:      "user",
		Tools:       []string{"Read"},
		MaxTurns:    30,
		Description: "Custom explore",
		WhenToUse:   "Custom explore tasks",
	})

	list := s.List()
	if len(list) != 5 {
		t.Errorf("List() count = %d, want 5 (user override replaces built-in)", len(list))
	}

	// The explore entry should be the user one
	found := false
	for _, d := range list {
		if d.AgentType == "explore" {
			found = true
			if d.Name != "Custom Explore" {
				t.Errorf("explore Name = %q, want Custom Explore", d.Name)
			}
		}
	}
	if !found {
		t.Error("explore not in List()")
	}
}

func TestAgentDefStore_LoadUserAgents(t *testing.T) {
	// Create a temporary YAML agent file
	tmpDir := t.TempDir()

	yamlContent := `agent_type: my-reviewer
name: "My Custom Reviewer"
description: "Custom code reviewer"
when_to_use: "Review specific code types"
tools: ["Read", "Glob", "Grep"]
max_turns: 50
model: claude-sonnet-4
provider: anthropic
permission_mode: accepts
color: "#E17055"
system_prompt: |
  You are a senior code reviewer. Focus on:
  1. Security issues
  2. Performance bottlenecks
  3. Code maintainability
  4. Best practices
`
	err := os.WriteFile(filepath.Join(tmpDir, "my-reviewer.yaml"), []byte(yamlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Also add a file that should be skipped (not .yaml/.yml)
	err = os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Not an agent"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	s := NewAgentDefStore()
	err = s.LoadUserAgents(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Should now have 6 agents (5 built-in + 1 user)
	if s.Count() != 6 {
		t.Errorf("Count() = %d, want 6", s.Count())
	}

	// Verify the loaded agent
	def, ok := s.Get("my-reviewer")
	if !ok {
		t.Fatal("my-reviewer not found")
	}
	if def.Name != "My Custom Reviewer" {
		t.Errorf("Name = %q, want My Custom Reviewer", def.Name)
	}
	if def.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", def.Provider)
	}
	if def.Model != "claude-sonnet-4" {
		t.Errorf("Model = %q, want claude-sonnet-4", def.Model)
	}
	if def.PermissionMode != "accepts" {
		t.Errorf("PermissionMode = %q, want accepts", def.PermissionMode)
	}
	if def.Color != "#E17055" {
		t.Errorf("Color = %q, want #E17055", def.Color)
	}
	if def.MaxTurns != 50 {
		t.Errorf("MaxTurns = %d, want 50", def.MaxTurns)
	}
	if def.Source != "user" {
		t.Errorf("Source = %q, want user", def.Source)
	}
	if def.Filename != "my-reviewer" {
		t.Errorf("Filename = %q, want my-reviewer", def.Filename)
	}
	if len(def.Tools) != 3 {
		t.Errorf("Tools count = %d, want 3", len(def.Tools))
	}
	if def.SystemPrompt == "" {
		t.Error("SystemPrompt should not be empty")
	}
}

func TestAgentDefStore_LoadUserAgents_EmptyToolDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `agent_type: minimal-agent
name: "Minimal"
description: "Minimal agent"
when_to_use: "Minimal tasks"
`
	err := os.WriteFile(filepath.Join(tmpDir, "minimal.yaml"), []byte(yamlContent), 0644)
	if err != nil {
		t.Fatal(err)
	}

	s := NewAgentDefStore()
	err = s.LoadUserAgents(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	def, ok := s.Get("minimal-agent")
	if !ok {
		t.Fatal("minimal-agent not found")
	}
	if !def.HasAllTools() {
		t.Error("minimal-agent should default to all tools (*)")
	}
	if def.MaxTurns != 200 {
		t.Errorf("MaxTurns = %d, want default 200", def.MaxTurns)
	}
}

func TestAgentDefStore_LoadUserAgents_NonexistentDir(t *testing.T) {
	s := NewAgentDefStore()
	err := s.LoadUserAgents("/nonexistent/directory/for/agents")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestAgentDefStore_LoadUserAgents_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	err := os.WriteFile(filepath.Join(tmpDir, "bad.yaml"), []byte("{{{invalid yaml!!!"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	s := NewAgentDefStore()
	err = s.LoadUserAgents(tmpDir)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestAgentDefStore_PluginRegistration(t *testing.T) {
	s := NewAgentDefStore()

	pluginAgent := &AgentDefinition{
		AgentType:   "plugin-agent",
		Name:        "Plugin Agent",
		Source:      "plugin",
		Tools:       []string{"*"},
		MaxTurns:    100,
		Description: "Plugin",
		WhenToUse:   "Plugin tasks",
	}
	s.Register(pluginAgent)

	def, ok := s.Get("plugin-agent")
	if !ok {
		t.Fatal("plugin-agent not found")
	}
	if def.Source != "plugin" {
		t.Errorf("Source = %q, want plugin", def.Source)
	}

	// Plugin should be in ListBySource
	plugins := s.ListBySource("plugin")
	if len(plugins) != 1 {
		t.Errorf("ListBySource(plugin) = %d, want 1", len(plugins))
	}
}

func TestAgentDefStore_Count(t *testing.T) {
	s := NewAgentDefStore()
	if s.Count() != 5 {
		t.Errorf("Count() = %d, want 5", s.Count())
	}

	s.Register(&AgentDefinition{
		AgentType: "extra", Name: "Extra", Source: "user",
		Tools: []string{"*"}, MaxTurns: 100, Description: "Extra", WhenToUse: "Extra",
	})
	if s.Count() != 6 {
		t.Errorf("Count() = %d, want 6 after registration", s.Count())
	}
}
