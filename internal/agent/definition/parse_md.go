package definition

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// agentFrontmatter represents the expected YAML frontmatter in agent .md files.
type agentFrontmatter struct {
	Name            string               `yaml:"name"`
	Description     string               `yaml:"description"`
	AgentType       string               `yaml:"agent_type"`
	WhenToUse       string               `yaml:"when_to_use"`
	Tools           []string             `yaml:"tools"`
	DisallowedTools []string             `yaml:"disallowed_tools"`
	Skills          []string             `yaml:"skills"`
	McpServers      []AgentMcpServerSpec `yaml:"mcp_servers"`
	Color           string               `yaml:"color"`
	Model           string               `yaml:"model"`
	Effort          *EffortValue         `yaml:"effort"`
	PermissionMode  *string              `yaml:"permission_mode"`
	MaxTurns        *int                 `yaml:"max_turns"`
	Background      bool                 `yaml:"background"`
	InitialPrompt   string               `yaml:"initial_prompt"`
	Memory          *MemoryScope         `yaml:"memory"`
	Isolation       string               `yaml:"isolation"`
}

// ParseAgentFromMarkdown parses an agent definition from markdown content.
// The markdown must have YAML frontmatter between --- delimiters.
// The body text after the frontmatter becomes the SystemPrompt.
func ParseAgentFromMarkdown(content string) (*AgentDefinition, error) {
	fm, body, err := extractFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}

	var parsed agentFrontmatter
	if err := yaml.Unmarshal([]byte(fm), &parsed); err != nil {
		return nil, fmt.Errorf("yaml: %w", err)
	}

	if parsed.Name == "" {
		return nil, fmt.Errorf("required field 'name' is missing")
	}
	if parsed.Description == "" {
		return nil, fmt.Errorf("required field 'description' is missing")
	}

	agentType := parsed.AgentType
	if agentType == "" {
		agentType = strings.ToLower(strings.ReplaceAll(parsed.Name, " ", "-"))
	}

	return &AgentDefinition{
		AgentType:       agentType,
		Name:            parsed.Name,
		Description:     parsed.Description,
		WhenToUse:       parsed.WhenToUse,
		Tools:           parsed.Tools,
		DisallowedTools: parsed.DisallowedTools,
		Skills:          parsed.Skills,
		McpServers:      parsed.McpServers,
		Color:           parsed.Color,
		Model:           parsed.Model,
		Effort:          parsed.Effort,
		PermissionMode:  parsed.PermissionMode,
		MaxTurns:        parsed.MaxTurns,
		Background:      parsed.Background,
		InitialPrompt:   parsed.InitialPrompt,
		Memory:          parsed.Memory,
		Isolation:       parsed.Isolation,
		SystemPrompt:    strings.TrimSpace(body),
	}, nil
}

// extractFrontmatter extracts YAML frontmatter between --- delimiters.
// Returns the raw frontmatter string and the remaining body.
func extractFrontmatter(content string) (frontmatter, body string, err error) {
	const delim = "---"
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, delim) {
		return "", "", fmt.Errorf("content must start with ---")
	}

	rest := trimmed[len(delim):]
	endIdx := strings.Index(rest, delim)
	if endIdx < 0 {
		return "", "", fmt.Errorf("closing --- not found")
	}

	fm := rest[:endIdx]
	bd := rest[endIdx+len(delim):]
	return strings.TrimSpace(fm), bd, nil
}
