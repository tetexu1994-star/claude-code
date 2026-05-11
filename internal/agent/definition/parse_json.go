package definition

import (
	"encoding/json"
	"fmt"
	"strings"
)

// agentJSONDef mirrors agentFrontmatter but uses JSON tags for parsing.
type agentJSONDef struct {
	Name            string               `json:"name"`
	Description     string               `json:"description"`
	AgentType       string               `json:"agent_type"`
	WhenToUse       string               `json:"when_to_use"`
	Tools           []string             `json:"tools"`
	DisallowedTools []string             `json:"disallowed_tools"`
	Skills          []string             `json:"skills"`
	McpServers      []AgentMcpServerSpec `json:"mcp_servers"`
	Color           string               `json:"color"`
	Model           string               `json:"model"`
	Effort          *EffortValue         `json:"effort"`
	PermissionMode  *string              `json:"permission_mode"`
	MaxTurns        *int                 `json:"max_turns"`
	Background      bool                 `json:"background"`
	InitialPrompt   string               `json:"initial_prompt"`
	Memory          *MemoryScope         `json:"memory"`
	Isolation       string               `json:"isolation"`
	SystemPrompt    string               `json:"system_prompt"`
}

// ParseAgentFromJson parses a single agent definition from a JSON byte slice.
func ParseAgentFromJson(data []byte) (*AgentDefinition, error) {
	var raw agentJSONDef
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	if raw.Name == "" {
		return nil, fmt.Errorf("required field 'name' is missing")
	}
	if raw.Description == "" {
		return nil, fmt.Errorf("required field 'description' is missing")
	}

	agentType := raw.AgentType
	if agentType == "" {
		agentType = strings.ToLower(strings.ReplaceAll(raw.Name, " ", "-"))
	}

	return &AgentDefinition{
		AgentType:       agentType,
		Name:            raw.Name,
		Description:     raw.Description,
		WhenToUse:       raw.WhenToUse,
		Tools:           raw.Tools,
		DisallowedTools: raw.DisallowedTools,
		Skills:          raw.Skills,
		McpServers:      raw.McpServers,
		Color:           raw.Color,
		Model:           raw.Model,
		Effort:          raw.Effort,
		PermissionMode:  raw.PermissionMode,
		MaxTurns:        raw.MaxTurns,
		Background:      raw.Background,
		InitialPrompt:   raw.InitialPrompt,
		Memory:          raw.Memory,
		Isolation:       raw.Isolation,
		SystemPrompt:    raw.SystemPrompt,
	}, nil
}

// ParseAgentsFromJson parses an array of agent definitions from a JSON byte slice.
func ParseAgentsFromJson(data []byte) ([]*AgentDefinition, error) {
	var raw []agentJSONDef
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	result := make([]*AgentDefinition, 0, len(raw))
	for i, r := range raw {
		if r.Name == "" {
			return nil, fmt.Errorf("agent at index %d: required field 'name' is missing", i)
		}
		if r.Description == "" {
			return nil, fmt.Errorf("agent at index %d: required field 'description' is missing", i)
		}

		agentType := r.AgentType
		if agentType == "" {
			agentType = strings.ToLower(strings.ReplaceAll(r.Name, " ", "-"))
		}

		result = append(result, &AgentDefinition{
			AgentType:       agentType,
			Name:            r.Name,
			Description:     r.Description,
			WhenToUse:       r.WhenToUse,
			Tools:           r.Tools,
			DisallowedTools: r.DisallowedTools,
			Skills:          r.Skills,
			McpServers:      r.McpServers,
			Color:           r.Color,
			Model:           r.Model,
			Effort:          r.Effort,
			PermissionMode:  r.PermissionMode,
			MaxTurns:        r.MaxTurns,
			Background:      r.Background,
			InitialPrompt:   r.InitialPrompt,
			Memory:          r.Memory,
			Isolation:       r.Isolation,
			SystemPrompt:    r.SystemPrompt,
		})
	}
	return result, nil
}
