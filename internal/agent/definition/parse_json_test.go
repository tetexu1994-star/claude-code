package definition

import (
	"encoding/json"
	"testing"
)

func TestParseAgentFromJson(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		want    func(*AgentDefinition) bool
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid full definition",
			json: `{
				"name": "JSON Agent",
				"description": "Loaded from JSON",
				"agent_type": "json-agent",
				"when_to_use": "For JSON testing",
				"tools": ["Read", "Glob"],
				"disallowed_tools": ["Bash"],
				"skills": ["json-skill"],
				"color": "#00FF00",
				"model": "claude-haiku-4",
				"effort": "medium",
				"permission_mode": "acceptEdits",
				"max_turns": 75,
				"background": false,
				"initial_prompt": "Start with JSON parsing.",
				"memory": "local",
				"isolation": "",
				"system_prompt": "You are a JSON-defined agent."
			}`,
			want: func(d *AgentDefinition) bool {
				return d.Name == "JSON Agent" &&
					d.Description == "Loaded from JSON" &&
					d.AgentType == "json-agent" &&
					d.WhenToUse == "For JSON testing" &&
					len(d.Tools) == 2 &&
					len(d.DisallowedTools) == 1 &&
					len(d.Skills) == 1 &&
					d.Color == "#00FF00" &&
					d.Model == "claude-haiku-4" &&
					d.Effort != nil && *d.Effort == EffortMedium &&
					d.PermissionMode != nil && *d.PermissionMode == "acceptEdits" &&
					d.MaxTurns != nil && *d.MaxTurns == 75 &&
					!d.Background &&
					d.InitialPrompt == "Start with JSON parsing." &&
					d.Memory != nil && *d.Memory == MemLocal &&
					d.SystemPrompt == "You are a JSON-defined agent."
			},
		},
		{
			name: "minimal valid",
			json: `{"name": "MinJSON", "description": "Minimal JSON agent"}`,
			want: func(d *AgentDefinition) bool {
				return d.Name == "MinJSON" &&
					d.Description == "Minimal JSON agent" &&
					d.AgentType == "minjson"
			},
		},
		{
			name:    "missing name",
			json:    `{"description": "No name"}`,
			wantErr: true,
			errMsg:  "required field 'name' is missing",
		},
		{
			name:    "missing description",
			json:    `{"name": "No Desc"}`,
			wantErr: true,
			errMsg:  "required field 'description' is missing",
		},
		{
			name:    "invalid json",
			json:    `{invalid}`,
			wantErr: true,
			errMsg:  "json unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAgentFromJson([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.want(got) {
				t.Errorf("ParseAgentFromJson() unexpected result: %+v", got)
			}
		})
	}
}

func TestParseAgentsFromJson(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantLen int
		wantErr bool
	}{
		{
			name: "multiple agents",
			json: `[
				{"name": "Agent One", "description": "First agent"},
				{"name": "Agent Two", "description": "Second agent"}
			]`,
			wantLen: 2,
		},
		{
			name:    "empty array",
			json:    `[]`,
			wantLen: 0,
		},
		{
			name: "missing name in array item",
			json: `[
				{"name": "Good", "description": "Valid"},
				{"description": "Missing name"}
			]`,
			wantErr: true,
		},
		{
			name:    "invalid json array",
			json:    `[invalid]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAgentsFromJson([]byte(tt.json))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Errorf("got %d agents, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestParseAgentsFromJson_Roundtrip(t *testing.T) {
	orig := []*AgentDefinition{
		{
			AgentType:   "roundtrip-agent",
			Name:        "Roundtrip",
			Description: "Roundtrip test",
			Tools:       []string{"Read"},
			Color:       "#ABCDEF",
		},
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := ParseAgentsFromJson(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(parsed) != 1 {
		t.Fatalf("got %d agents, want 1", len(parsed))
	}
	if parsed[0].AgentType != "roundtrip-agent" {
		t.Errorf("AgentType = %q", parsed[0].AgentType)
	}
	if parsed[0].Name != "Roundtrip" {
		t.Errorf("Name = %q", parsed[0].Name)
	}
}
