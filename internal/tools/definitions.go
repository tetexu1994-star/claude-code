// Deprecated: Use tool.Registry and tools.GetToolDefinitionsFromRegistry() instead.
package tools

import (
	"encoding/json"

	"github.com/tetexu/tlaude-code/internal/llm"
	"github.com/tetexu/tlaude-code/internal/tool"
)

// GetToolDefinitions returns the current set of available tools as llm.ToolDefinition.
// These definitions are sent in ChatRequest.Tools so the LLM knows what it can call.
//
// Deprecated: prefer GetToolDefinitionsFromRegistry for new code.
func GetToolDefinitions() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{
			Name:        "bash",
			Description: "Execute a shell command in a sandboxed environment. Returns stdout, stderr, and exit code.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The shell command to execute",
					},
				},
				"required": []string{"command"},
			},
		},
		{
			Name:        "write_file",
			Description: "Write content to a file. Creates parent directories if needed.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file",
					},
					"content": map[string]interface{}{
						"type":        "string",
						"description": "Content to write",
					},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			Name:        "read_file",
			Description: "Read the contents of a file.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the file",
					},
				},
				"required": []string{"path"},
			},
		},
	}
}

// GetToolDefinitionsFromRegistry generates LLM-facing tool definitions from a tool.Registry,
// converting the json.RawMessage schemas into map[string]interface{} used by the llm package.
func GetToolDefinitionsFromRegistry(reg *tool.Registry) []llm.ToolDefinition {
	var tools []tool.Tool
	if reg != nil {
		tools = reg.GetAll(nil)
	} else {
		tools = tool.DefaultTools()
	}

	defs := make([]llm.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		td := t.ToolDefinition()
		def := llm.ToolDefinition{
			Name:        td.Name,
			Description: td.Description,
			InputSchema: rawToMap(td.InputSchema),
		}
		defs = append(defs, def)
	}
	return defs
}

// rawToMap converts json.RawMessage to map[string]interface{} for the llm package.
func rawToMap(raw json.RawMessage) map[string]interface{} {
	if raw == nil {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
