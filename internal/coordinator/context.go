package coordinator

import (
	"os"
	"strings"
)

// MCPServerInfo provides basic MCP server metadata for the coordinator context.
type MCPServerInfo struct {
	Name string
}

// internalWorkerTools are tools that workers should NOT have access to.
var internalWorkerTools = map[string]bool{
	"TeamCreate":      true,
	"TeamDelete":      true,
	"SendMessage":     true,
	"SyntheticOutput": true,
}

// WorkerTools returns the list of tool names available to worker sub-agents.
//
// In simple mode (TLAUDE_CODE_SIMPLE=1): Bash, Read, Edit only.
// In full mode: standard tools minus internal worker tools.
func WorkerTools(simpleMode bool) []string {
	if simpleMode {
		return []string{"Bash", "Read", "Edit"}
	}

	// Full worker tool set — standard async agent tools minus internal ones
	allTools := []string{
		"Agent",
		"Bash",
		"Edit",
		"ExitPlanMode",
		"Glob",
		"Grep",
		"Read",
		"Skill",
		"TaskStop",
		"WebFetch",
		"WebSearch",
		"Write",
	}

	var tools []string
	for _, t := range allTools {
		if !internalWorkerTools[t] {
			tools = append(tools, t)
		}
	}
	return tools
}

// IsSimpleMode checks whether simple worker mode is enabled.
func IsSimpleMode() bool {
	return os.Getenv("TLAUDE_CODE_SIMPLE") == "1"
}

// GetCoordinatorUserContext returns user-facing context about workers
// to be included in the coordinator's context.
//
// Returns a map with key "workerToolsContext" containing:
//   - The list of tools workers have access to
//   - MCP server info (if any)
//   - Scratchpad directory info (if enabled)
//
// Returns an empty map if coordinator mode is not active.
func GetCoordinatorUserContext(mcpClients []MCPServerInfo, scratchpadDir string) map[string]string {
	if !IsCoordinatorMode() {
		return nil
	}

	simpleMode := IsSimpleMode()
	workerTools := WorkerTools(simpleMode)

	var b strings.Builder
	b.WriteString("Workers spawned via the Agent tool have access to these tools: ")
	for i, t := range workerTools {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(t)
	}

	if len(mcpClients) > 0 {
		var names []string
		for _, c := range mcpClients {
			names = append(names, c.Name)
		}
		b.WriteString("\n\nWorkers also have access to MCP tools from connected MCP servers: ")
		b.WriteString(strings.Join(names, ", "))
	}

	if scratchpadDir != "" {
		b.WriteString("\n\nScratchpad directory: ")
		b.WriteString(scratchpadDir)
		b.WriteString("\nWorkers can read and write here without permission prompts. Use this for durable cross-worker knowledge — structure files however fits the work.")
	}

	return map[string]string{
		"workerToolsContext": b.String(),
	}
}
