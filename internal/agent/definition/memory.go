package definition

import (
	"github.com/tetexu/tlaude-code/internal/memory"
)

// LoadAgentMemoryPrompt returns the memory system prompt for a specific agent type and scope.
// It integrates with the existing memory.MemDir system to provide scope-specific instructions.
func LoadAgentMemoryPrompt(agentType string, scope MemoryScope) string {
	var memScope memory.AgentMemoryScope
	switch scope {
	case MemProject:
		memScope = memory.ScopeProject
	case MemLocal:
		memScope = memory.ScopeLocal
	default:
		memScope = memory.ScopeUser
	}

	return memory.LoadAgentMemoryPrompt(agentType, memScope)
}

// GetAgentMemoryDir returns the filesystem path for agent memory given a type and scope.
func GetAgentMemoryDir(agentType string, scope MemoryScope) string {
	var memScope memory.AgentMemoryScope
	switch scope {
	case MemProject:
		memScope = memory.ScopeProject
	case MemLocal:
		memScope = memory.ScopeLocal
	default:
		memScope = memory.ScopeUser
	}
	return memory.GetAgentMemoryDir(agentType, memScope)
}
