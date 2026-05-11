package definition

import (
	"testing"
)

func TestAgentDefinition_Defaults(t *testing.T) {
	d := &AgentDefinition{
		AgentType: "test",
		Name:      "Test Agent",
	}
	if d.AgentType != "test" {
		t.Errorf("AgentType = %q", d.AgentType)
	}
	if d.Name != "Test Agent" {
		t.Errorf("Name = %q", d.Name)
	}
	if d.Description != "" {
		t.Errorf("Description should be empty by default")
	}
	if d.Tools != nil {
		t.Errorf("Tools should be nil (all tools) by default")
	}
}

func TestAgentDefinitionsResult(t *testing.T) {
	result := &AgentDefinitionsResult{
		ActiveAgents: []*AgentDefinition{
			{AgentType: "a"},
		},
		AllAgents: []*AgentDefinition{
			{AgentType: "a"},
			{AgentType: "b"},
		},
		FailedFiles: []ParseError{
			{Path: "/tmp/bad.md", Error: "invalid yaml"},
		},
	}
	if len(result.ActiveAgents) != 1 {
		t.Errorf("ActiveAgents length = %d, want 1", len(result.ActiveAgents))
	}
	if len(result.AllAgents) != 2 {
		t.Errorf("AllAgents length = %d, want 2", len(result.AllAgents))
	}
	if len(result.FailedFiles) != 1 {
		t.Errorf("FailedFiles length = %d, want 1", len(result.FailedFiles))
	}
}

func TestSourceConstants(t *testing.T) {
	tests := []struct {
		s    Source
		want string
	}{
		{SourceBuiltIn, "built-in"},
		{SourceUser, "userSettings"},
		{SourceProject, "projectSettings"},
		{SourcePolicy, "policySettings"},
		{SourcePlugin, "plugin"},
	}
	for _, tt := range tests {
		if string(tt.s) != tt.want {
			t.Errorf("Source %q = %q, want %q", tt.s, string(tt.s), tt.want)
		}
	}
}

func TestMemoryScopeConstants(t *testing.T) {
	tests := []struct {
		m    MemoryScope
		want string
	}{
		{MemUser, "user"},
		{MemProject, "project"},
		{MemLocal, "local"},
	}
	for _, tt := range tests {
		if string(tt.m) != tt.want {
			t.Errorf("MemoryScope %q = %q, want %q", tt.m, string(tt.m), tt.want)
		}
	}
}

func TestEffortValueConstants(t *testing.T) {
	tests := []struct {
		e    EffortValue
		want string
	}{
		{EffortLow, "low"},
		{EffortMedium, "medium"},
		{EffortHigh, "high"},
	}
	for _, tt := range tests {
		if string(tt.e) != tt.want {
			t.Errorf("EffortValue %q = %q, want %q", tt.e, string(tt.e), tt.want)
		}
	}
}

func TestMcpServerConfig(t *testing.T) {
	cfg := McpServerConfig{
		Command: "node",
		Args:    []string{"server.js"},
		Env:     map[string]string{"NODE_ENV": "production"},
	}
	if cfg.Command != "node" {
		t.Errorf("Command = %q", cfg.Command)
	}
}

func TestAgentMcpServerSpec(t *testing.T) {
	spec := AgentMcpServerSpec{
		Name: "my-server",
		InlineConfig: &McpServerConfig{
			Command: "python",
			Args:    []string{"-m", "mcp.server"},
		},
	}
	if spec.Name != "my-server" {
		t.Errorf("Name = %q", spec.Name)
	}
	if spec.InlineConfig == nil {
		t.Fatal("InlineConfig should not be nil")
	}
}

func TestHooksSettings(t *testing.T) {
	hooks := &HooksSettings{
		BeforeToolUse: []string{"validate.sh"},
		AfterToolUse:  []string{"log.sh", "notify.sh"},
	}
	if len(hooks.BeforeToolUse) != 1 {
		t.Error("expected 1 before hook")
	}
	if len(hooks.AfterToolUse) != 2 {
		t.Error("expected 2 after hooks")
	}
}

func TestParseError(t *testing.T) {
	pe := ParseError{Path: "/path/to/file.md", Error: "parse failed"}
	if pe.Path != "/path/to/file.md" {
		t.Errorf("Path = %q", pe.Path)
	}
	if pe.Error != "parse failed" {
		t.Errorf("Error = %q", pe.Error)
	}
}
