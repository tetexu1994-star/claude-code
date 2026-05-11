package definition

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// sourcePriority maps source to a numeric priority (higher = wins on conflict).
var sourcePriority = map[Source]int{
	SourceBuiltIn: 0,
	SourcePlugin:  1,
	SourceUser:    2,
	SourceProject: 3,
	SourcePolicy:  4,
}

var (
	loadCache   *AgentDefinitionsResult
	loadCacheMu sync.Mutex
	lastCWD     string
)

// ClearCache clears the memoized LoadAgentsDir result.
func ClearCache() {
	loadCacheMu.Lock()
	defer loadCacheMu.Unlock()
	loadCache = nil
	lastCWD = ""
}

// LoadAgentsDir scans <cwd>/agents/*.md for custom agents and merges them
// with built-in agents. Results are memoized per cwd.
func LoadAgentsDir(cwd string) (*AgentDefinitionsResult, error) {
	loadCacheMu.Lock()
	if loadCache != nil && lastCWD == cwd {
		result := loadCache
		loadCacheMu.Unlock()
		return result, nil
	}
	loadCacheMu.Unlock()

	result, err := loadAgentsDirImpl(cwd)
	if err != nil {
		return nil, err
	}

	loadCacheMu.Lock()
	loadCache = result
	lastCWD = cwd
	loadCacheMu.Unlock()

	return result, nil
}

func loadAgentsDirImpl(cwd string) (*AgentDefinitionsResult, error) {
	var all []*AgentDefinition
	var failed []ParseError

	// Layer 1: Built-in agents (lowest priority).
	builtins := GetBuiltInAgents()
	for _, b := range builtins {
		all = append(all, b)
	}

	// Layer 2: Project agents from <cwd>/agents/*.md.
	agentsDir := filepath.Join(cwd, "agents")
	projectAgents, projectFailed := scanAgentsDir(agentsDir, SourceProject)
	for _, a := range projectAgents {
		a.BaseDir = cwd
		all = append(all, a)
	}
	failed = append(failed, projectFailed...)

	// Merge by agentType: last-wins by priority.
	merged := mergeByPriority(all)

	return &AgentDefinitionsResult{
		ActiveAgents: merged,
		AllAgents:    all,
		FailedFiles:  failed,
	}, nil
}

// scanAgentsDir reads all .md files from a directory and parses them as agent definitions.
func scanAgentsDir(dir string, src Source) ([]*AgentDefinition, []ParseError) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil // directory doesn't exist is not an error
	}

	var agents []*AgentDefinition
	var failed []ParseError

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			failed = append(failed, ParseError{Path: fullPath, Error: fmt.Sprintf("read: %v", err)})
			continue
		}

		def, err := ParseAgentFromMarkdown(string(data))
		if err != nil {
			failed = append(failed, ParseError{Path: fullPath, Error: err.Error()})
			continue
		}

		baseName := strings.TrimSuffix(entry.Name(), ".md")
		def.Filename = baseName
		def.Source = src
		agents = append(agents, def)
	}

	return agents, failed
}

// mergeByPriority deduplicates by agentType. Higher source priority wins.
func mergeByPriority(all []*AgentDefinition) []*AgentDefinition {
	best := make(map[string]*AgentDefinition)

	for _, def := range all {
		existing, ok := best[def.AgentType]
		if !ok || sourcePriority[def.Source] >= sourcePriority[existing.Source] {
			best[def.AgentType] = def
		}
	}

	result := make([]*AgentDefinition, 0, len(best))
	for _, def := range best {
		result = append(result, def)
	}
	return result
}
