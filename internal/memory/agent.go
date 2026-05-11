package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AgentMemoryScope represents the persistence scope for agent memory.
type AgentMemoryScope string

const (
	ScopeUser    AgentMemoryScope = "user"    // ~/.tlaude-code/agent-memory/<type>/
	ScopeProject AgentMemoryScope = "project" // .claude/agent-memory/<type>/
	ScopeLocal   AgentMemoryScope = "local"   // .claude/agent-memory-local/<type>/
)

// GetAgentMemoryDir returns the directory for an agent type + scope.
func GetAgentMemoryDir(agentType string, scope AgentMemoryScope) string {
	dirName := sanitizeAgentTypeForPath(agentType)
	var base string
	switch scope {
	case ScopeProject:
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		base = filepath.Join(cwd, ".claude", "agent-memory", dirName)
	case ScopeLocal:
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		base = filepath.Join(cwd, ".claude", "agent-memory-local", dirName)
	case ScopeUser:
		fallthrough
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		base = filepath.Join(home, ".tlaude-code", "agent-memory", dirName)
	}
	return base + string(filepath.Separator)
}

// IsAgentMemoryPath checks if a path belongs to agent memory (any scope).
func IsAgentMemoryPath(absPath string) bool {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	userPrefix := filepath.Join(home, ".tlaude-code", "agent-memory") + string(filepath.Separator)
	projectPrefix := filepath.Join(cwd, ".claude", "agent-memory") + string(filepath.Separator)
	localPrefix := filepath.Join(cwd, ".claude", "agent-memory-local") + string(filepath.Separator)

	return strings.HasPrefix(absPath, userPrefix) ||
		strings.HasPrefix(absPath, projectPrefix) ||
		strings.HasPrefix(absPath, localPrefix)
}

// LoadAgentMemoryPrompt returns the memory prompt for a specific agent.
func LoadAgentMemoryPrompt(agentType string, scope AgentMemoryScope) string {
	memoryDir := GetAgentMemoryDir(agentType, scope)
	store := NewStore(memoryDir)
	_ = store.EnsureDir()

	var scopeNote string
	switch scope {
	case ScopeUser:
		scopeNote = "- Since this memory is user-scope, keep learnings general since they apply across all projects"
	case ScopeProject:
		scopeNote = "- Since this memory is project-scope and shared with your team via version control, tailor your memories to this project"
	case ScopeLocal:
		scopeNote = "- Since this memory is local-scope (not checked into version control), tailor your memories to this project and machine"
	}

	prompt := store.BuildMemoryPrompt()
	if scopeNote != "" {
		prompt = prompt + "\n" + scopeNote
	}

	return prompt
}

// GetMemoryScopeDisplay returns a human-readable description of the scope.
func GetMemoryScopeDisplay(scope AgentMemoryScope) string {
	switch scope {
	case ScopeUser:
		home, _ := os.UserHomeDir()
		return fmt.Sprintf("User (%s/agent-memory/)", filepath.Join(home, ".tlaude-code"))
	case ScopeProject:
		return "Project (.claude/agent-memory/)"
	case ScopeLocal:
		return "Local (.claude/agent-memory-local/)"
	default:
		return "None"
	}
}

func sanitizeAgentTypeForPath(agentType string) string {
	return strings.ReplaceAll(agentType, ":", "-")
}
