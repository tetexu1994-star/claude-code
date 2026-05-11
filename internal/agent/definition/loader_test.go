package definition

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAgentsDir_NoAgentsDir(t *testing.T) {
	ClearCache()
	dir := t.TempDir()

	result, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ActiveAgents) < 4 {
		t.Errorf("expected at least 4 built-in agents, got %d", len(result.ActiveAgents))
	}
	// Verify built-in types present.
	types := make(map[string]bool)
	for _, a := range result.ActiveAgents {
		types[a.AgentType] = true
	}
	for _, want := range []string{"general-purpose", "cli-guide", "explore", "plan"} {
		if !types[want] {
			t.Errorf("expected built-in agent %q not found", want)
		}
	}
}

func TestLoadAgentsDir_WithProjectAgents(t *testing.T) {
	ClearCache()
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	customMD := `---
name: Custom Agent
description: A custom project agent
agent_type: custom
tools:
  - Read
  - Write
color: "#123456"
---
You are a custom agent from the project directory.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "custom.md"), []byte(customMD), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have built-ins + custom.
	if len(result.ActiveAgents) < 5 {
		t.Errorf("expected at least 5 agents (4 built-in + 1 custom), got %d", len(result.ActiveAgents))
	}

	// Verify custom agent is present.
	found := false
	for _, a := range result.ActiveAgents {
		if a.AgentType == "custom" && a.Source == SourceProject {
			found = true
			if a.SystemPrompt != "You are a custom agent from the project directory." {
				t.Errorf("unexpected SystemPrompt: %q", a.SystemPrompt)
			}
			break
		}
	}
	if !found {
		t.Error("custom project agent not found in result")
	}
}

func TestLoadAgentsDir_ProjectOverridesBuiltin(t *testing.T) {
	ClearCache()
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Override the "explore" built-in.
	overrideMD := `---
name: Custom Explore
description: A project-level explore override
agent_type: explore
tools:
  - Read
color: "#OVERRIDE"
---
I am the overridden explore agent.
`
	if err := os.WriteFile(filepath.Join(agentsDir, "explore.md"), []byte(overrideMD), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the explore agent — should be the project one.
	for _, a := range result.ActiveAgents {
		if a.AgentType == "explore" {
			if a.Source != SourceProject {
				t.Errorf("expected explore agent to have source project, got %q", a.Source)
			}
			if a.SystemPrompt != "I am the overridden explore agent." {
				t.Errorf("expected overridden SystemPrompt, got %q", a.SystemPrompt)
			}
			return
		}
	}
	t.Error("explore agent not found")
}

func TestLoadAgentsDir_ParseErrors(t *testing.T) {
	ClearCache()
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	badMD := `This has no frontmatter`
	if err := os.WriteFile(filepath.Join(agentsDir, "bad.md"), []byte(badMD), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.FailedFiles) != 1 {
		t.Fatalf("expected 1 failed file, got %d", len(result.FailedFiles))
	}
	if result.FailedFiles[0].Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestLoadAgentsDir_Memoization(t *testing.T) {
	ClearCache()
	dir := t.TempDir()

	result1, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	// Create agents dir after first call — should NOT affect memoized result.
	agentsDir := filepath.Join(dir, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "late.md"), []byte(`---
name: Late Agent
description: Created after first load
---`), 0644)

	result2, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}

	if len(result2.ActiveAgents) != len(result1.ActiveAgents) {
		t.Error("memoization broken — results differ between calls")
	}
}

func TestLoadAgentsDir_CacheClear(t *testing.T) {
	ClearCache()
	dir := t.TempDir()

	result1, _ := LoadAgentsDir(dir)
	ClearCache()

	// Now create agents dir.
	agentsDir := filepath.Join(dir, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "new.md"), []byte(`---
name: New Agent
description: Created after cache clear
---`), 0644)

	result2, err := LoadAgentsDir(dir)
	if err != nil {
		t.Fatalf("after clear: %v", err)
	}

	if len(result2.ActiveAgents) <= len(result1.ActiveAgents) {
		t.Error("cache should have been refreshed after ClearCache")
	}
}

func TestScanAgentsDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	agents, failed := scanAgentsDir(dir, SourceProject)
	if len(agents) != 0 {
		t.Errorf("expected 0 agents from empty dir, got %d", len(agents))
	}
	if len(failed) != 0 {
		t.Errorf("expected 0 failed from empty dir, got %d", len(failed))
	}
}

func TestScanAgentsDir_NonExistent(t *testing.T) {
	agents, failed := scanAgentsDir("/nonexistent/path/agents", SourceProject)
	if agents != nil {
		t.Error("expected nil agents for nonexistent dir")
	}
	if failed != nil {
		t.Error("expected nil failed for nonexistent dir")
	}
}

func TestMergeByPriority(t *testing.T) {
	builtin := &AgentDefinition{AgentType: "test", Source: SourceBuiltIn, SystemPrompt: "built-in"}
	user := &AgentDefinition{AgentType: "test", Source: SourceUser, SystemPrompt: "user"}

	merged := mergeByPriority([]*AgentDefinition{builtin, user})
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged agent, got %d", len(merged))
	}
	if merged[0].SystemPrompt != "user" {
		t.Errorf("expected user to win, got %q", merged[0].SystemPrompt)
	}
}

func TestMergeByPriority_DifferentTypes(t *testing.T) {
	a := &AgentDefinition{AgentType: "alpha", Source: SourceBuiltIn}
	b := &AgentDefinition{AgentType: "beta", Source: SourceUser}

	merged := mergeByPriority([]*AgentDefinition{a, b})
	if len(merged) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(merged))
	}
}
