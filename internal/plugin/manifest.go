package plugin

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest describes a plugin from its plugin.yaml file.
type Manifest struct {
	Name        string     `yaml:"name"`
	Version     string     `yaml:"version"`
	Description string     `yaml:"description"`
	Author      string     `yaml:"author,omitempty"`
	Type        Type       `yaml:"type"`
	Entrypoint  string     `yaml:"entrypoint,omitempty"` // for lua plugins
	MCP         *MCPConfig `yaml:"mcp,omitempty"`        // for mcp plugins
	Provides    []Provides `yaml:"provides"`
	Config      map[string]interface{} `yaml:"config,omitempty"`
	Tools       []ManifestTool         `yaml:"tools,omitempty"`
}

// MCPConfig describes how to launch an MCP subprocess server.
type MCPConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args,omitempty"`
}

// ManifestTool describes a tool declaration in a plugin manifest.
type ManifestTool struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	Schema      map[string]interface{} `yaml:"schema"`
}

// ParseManifest reads and unmarshals a plugin.yaml file at the given path.
func ParseManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest %s: %w", path, err)
	}

	m := &Manifest{}
	if err := yaml.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("parsing manifest %s: %w", path, err)
	}

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("validating manifest %s: %w", path, err)
	}

	return m, nil
}

// Validate checks that required fields are present and values are valid.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if m.Description == "" {
		return fmt.Errorf("description is required")
	}

	switch m.Type {
	case TypeLua, TypeMCP, TypeHybrid:
		// valid
	case "":
		return fmt.Errorf("type is required (lua, mcp, or hybrid)")
	default:
		return fmt.Errorf("invalid type %q: must be lua, mcp, or hybrid", m.Type)
	}

	if (m.Type == TypeLua || m.Type == TypeHybrid) && m.Entrypoint == "" {
		return fmt.Errorf("entrypoint is required for lua and hybrid plugins")
	}

	if (m.Type == TypeMCP || m.Type == TypeHybrid) && (m.MCP == nil || m.MCP.Command == "") {
		return fmt.Errorf("mcp.command is required for mcp and hybrid plugins")
	}

	if len(m.Provides) == 0 {
		return fmt.Errorf("at least one provides value is required")
	}

	for _, p := range m.Provides {
		switch p {
		case ProvidesTool, ProvidesProvider, ProvidesSandbox, ProvidesHook, ProvidesAgent:
			// valid
		default:
			return fmt.Errorf("invalid provides value %q", p)
		}
	}

	return nil
}
