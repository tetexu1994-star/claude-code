package definition

import (
	"testing"
)

func TestParseAgentFromMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    func(*AgentDefinition) bool
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid with all fields",
			content: `---
name: Test Agent
description: A test agent for validation
agent_type: test-agent
when_to_use: When testing
tools:
  - Read
  - Write
disallowed_tools:
  - Bash
skills:
  - test-skill
color: "#FF0000"
model: claude-sonnet-4
effort: high
permission_mode: bypassPermissions
max_turns: 50
background: true
initial_prompt: Start by analyzing the task.
memory: project
isolation: worktree
---
You are a test agent that helps with testing.`,
			want: func(d *AgentDefinition) bool {
				return d.Name == "Test Agent" &&
					d.Description == "A test agent for validation" &&
					d.AgentType == "test-agent" &&
					d.WhenToUse == "When testing" &&
					len(d.Tools) == 2 &&
					d.Tools[0] == "Read" &&
					len(d.DisallowedTools) == 1 &&
					len(d.Skills) == 1 &&
					d.Color == "#FF0000" &&
					d.Model == "claude-sonnet-4" &&
					d.Effort != nil && *d.Effort == EffortHigh &&
					d.PermissionMode != nil && *d.PermissionMode == "bypassPermissions" &&
					d.MaxTurns != nil && *d.MaxTurns == 50 &&
					d.Background &&
					d.InitialPrompt == "Start by analyzing the task." &&
					d.Memory != nil && *d.Memory == MemProject &&
					d.Isolation == "worktree" &&
					d.SystemPrompt == "You are a test agent that helps with testing."
			},
		},
		{
			name: "minimal valid",
			content: `---
name: Minimal
description: Just the essentials
---
Do your best.`,
			want: func(d *AgentDefinition) bool {
				return d.Name == "Minimal" &&
					d.Description == "Just the essentials" &&
					d.AgentType == "minimal" && // derived from name
					d.SystemPrompt == "Do your best."
			},
		},
		{
			name: "auto-generates agent_type from name",
			content: `---
name: My Custom Agent
description: Auto-generated type
---
`,
			want: func(d *AgentDefinition) bool {
				return d.AgentType == "my-custom-agent"
			},
		},
		{
			name: "preserves explicit agent_type",
			content: `---
name: My Custom Agent
description: Has explicit type
agent_type: explicit-type
---
`,
			want: func(d *AgentDefinition) bool {
				return d.AgentType == "explicit-type"
			},
		},
		{
			name: "nil tools means all tools",
			content: `---
name: All Tools
description: No tools field means all tools
---
`,
			want: func(d *AgentDefinition) bool {
				return d.Tools == nil
			},
		},
		{
			name:    "missing name",
			content: `---\ndescription: No name here\n---\n`,
			wantErr: true,
			errMsg:  "required field 'name' is missing",
		},
		{
			name:    "missing description",
			content: "---\nname: No Desc\n---\n",
			wantErr: true,
			errMsg:  "required field 'description' is missing",
		},
		{
			name:    "no frontmatter",
			content: "Just plain text without frontmatter.",
			wantErr: true,
			errMsg:  "frontmatter: content must start with ---",
		},
		{
			name:    "unclosed frontmatter",
			content: "---\nname: Unclosed\n",
			wantErr: true,
			errMsg:  "frontmatter: closing --- not found",
		},
		{
			name:    "invalid yaml",
			content: "---\nname: [unclosed\n---\n",
			wantErr: true,
			errMsg:  "yaml:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAgentFromMarkdown(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if tt.errMsg != "" {
					if !contains(err.Error(), tt.errMsg) {
						t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.want(got) {
				t.Errorf("ParseAgentFromMarkdown() unexpected result: %+v", got)
			}
		})
	}
}

func TestParseAgentFromMarkdown_EmptyBody(t *testing.T) {
	def, err := ParseAgentFromMarkdown("---\nname: Empty Body\ndescription: No body\n---\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.SystemPrompt != "" {
		t.Errorf("expected empty SystemPrompt, got %q", def.SystemPrompt)
	}
}

func TestParseAgentFromMarkdown_MultiLineBody(t *testing.T) {
	def, err := ParseAgentFromMarkdown(`---
name: Multi
description: Multi-line body
---
Line one.
Line two.
Line three.
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if def.SystemPrompt != "Line one.\nLine two.\nLine three." {
		t.Errorf("unexpected SystemPrompt: %q", def.SystemPrompt)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
