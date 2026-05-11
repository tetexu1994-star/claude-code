package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tetexu/tlaude-code/internal/agent/definition"
	"gopkg.in/yaml.v3"
)

// AgentDefStore stores and manages all agent definitions.
// It supports built-in agents, user-defined agents loaded from YAML files,
// and plugin agents (future).
type AgentDefStore struct {
	builtIns map[string]*AgentDefinition
	userDefs map[string]*AgentDefinition
	plugins  map[string]*AgentDefinition
	mu       sync.RWMutex
}

// NewAgentDefStore creates a new store with built-in agents pre-registered.
func NewAgentDefStore() *AgentDefStore {
	s := &AgentDefStore{
		builtIns: make(map[string]*AgentDefinition),
		userDefs: make(map[string]*AgentDefinition),
		plugins:  make(map[string]*AgentDefinition),
	}
	for _, def := range BuiltInAgents() {
		d := def // capture
		s.builtIns[d.AgentType] = &d
	}
	return s
}

// Register adds an agent definition to the store under the appropriate source.
func (s *AgentDefStore) Register(def *AgentDefinition) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch def.Source {
	case "built-in":
		s.builtIns[def.AgentType] = def
	case "plugin":
		s.plugins[def.AgentType] = def
	default:
		// user, project, etc.
		if def.Filename == "" {
			def.Source = "user"
		}
		s.userDefs[def.AgentType] = def
	}
}

// RegisterAgents converts and registers multiple definition.AgentDefinition instances.
// This is the bridge between the definition package loader and the agent store.
func (s *AgentDefStore) RegisterAgents(defs []*definition.AgentDefinition) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range defs {
		ad := convertDef(d)
		switch ad.Source {
		case "built-in":
			s.builtIns[ad.AgentType] = ad
		case "plugin":
			s.plugins[ad.AgentType] = ad
		default:
			s.userDefs[ad.AgentType] = ad
		}
	}
}

// convertDef converts a definition.AgentDefinition to an agent.AgentDefinition.
func convertDef(d *definition.AgentDefinition) *AgentDefinition {
	out := &AgentDefinition{
		AgentType:       d.AgentType,
		Name:            d.Name,
		Description:     d.Description,
		WhenToUse:       d.WhenToUse,
		Tools:           d.Tools,
		DisallowedTools: d.DisallowedTools,
		Skills:          d.Skills,
		Color:           d.Color,
		Model:           d.Model,
		Source:          string(d.Source),
		Background:      d.Background,
		Isolation:       d.Isolation,
		SystemPrompt:    d.SystemPrompt,
		Filename:        d.Filename,
	}

	if out.Source == "" {
		out.Source = "user"
	}

	if d.MaxTurns != nil {
		out.MaxTurns = *d.MaxTurns
	} else {
		out.MaxTurns = 200
	}

	if d.PermissionMode != nil {
		out.PermissionMode = *d.PermissionMode
	}

	if d.Memory != nil {
		out.Memory = string(*d.Memory)
	}

	return out
}

// Get retrieves an agent definition by type name.
// User definitions override plugin definitions, which override built-ins.
func (s *AgentDefStore) Get(agentType string) (*AgentDefinition, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Priority: userDefs > plugins > builtIns
	if def, ok := s.userDefs[agentType]; ok {
		return def, true
	}
	if def, ok := s.plugins[agentType]; ok {
		return def, true
	}
	def, ok := s.builtIns[agentType]
	return def, ok
}

// List returns all agent definitions (deduplicated, user overrides built-in).
func (s *AgentDefStore) List() []*AgentDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]bool)
	var result []*AgentDefinition
	// User defs first (highest priority)
	for _, d := range s.userDefs {
		seen[d.AgentType] = true
		result = append(result, d)
	}
	for _, d := range s.plugins {
		if !seen[d.AgentType] {
			seen[d.AgentType] = true
			result = append(result, d)
		}
	}
	for _, d := range s.builtIns {
		if !seen[d.AgentType] {
			result = append(result, d)
		}
	}
	return result
}

// ListBySource returns agent definitions filtered by source.
func (s *AgentDefStore) ListBySource(source string) []*AgentDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*AgentDefinition
	switch source {
	case "built-in":
		for _, d := range s.builtIns {
			result = append(result, d)
		}
	case "plugin":
		for _, d := range s.plugins {
			result = append(result, d)
		}
	default:
		for _, d := range s.userDefs {
			if d.Source == source {
				result = append(result, d)
			}
		}
	}
	return result
}

// LoadUserAgents loads YAML agent definitions from a directory.
// Each .yaml or .yml file should contain a single agent definition.
func (s *AgentDefStore) LoadUserAgents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read agents dir %q: %w", dir, err)
	}
	var loaded int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		def, err := parseAgentYAML(fullPath)
		if err != nil {
			return fmt.Errorf("parse %q: %w", fullPath, err)
		}
		if def == nil {
			continue
		}
		if def.Source == "" {
			def.Source = "user"
		}
		def.Filename = strings.TrimSuffix(entry.Name(), ext)
		s.Register(def)
		loaded++
	}
	_ = loaded
	return nil
}

// parseAgentYAML reads and unmarshals a single agent YAML file.
func parseAgentYAML(path string) (*AgentDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	var def AgentDefinition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("yaml unmarshal: %w", err)
	}
	if def.AgentType == "" {
		return nil, nil
	}
	// Convert relative tools to canonical names if needed
	if len(def.Tools) == 0 {
		def.Tools = []string{"*"}
	}
	if def.MaxTurns == 0 {
		def.MaxTurns = 200
	}
	return &def, nil
}

// Count returns the total number of registered agent definitions.
func (s *AgentDefStore) Count() int {
	return len(s.List())
}

// CountBuiltIn returns the number of built-in agent definitions.
func (s *AgentDefStore) CountBuiltIn() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.builtIns)
}
