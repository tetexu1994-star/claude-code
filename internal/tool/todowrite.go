package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TodoWriteTool writes todos to a persistent JSON file.
type TodoWriteTool struct {
	storePath string
	mu        sync.Mutex
}

func NewTodoWriteTool() *TodoWriteTool {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".tlaude-code")
	os.MkdirAll(dir, 0755)
	return &TodoWriteTool{
		storePath: filepath.Join(dir, "todos.json"),
	}
}

func (t *TodoWriteTool) Name() string        { return "TodoWrite" }
func (t *TodoWriteTool) Description() string { return "Create and manage a structured task list for the current coding session." }
func (t *TodoWriteTool) IsEnabled() bool     { return true }
func (t *TodoWriteTool) IsConcurrencySafe() bool { return false }

func (t *TodoWriteTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "content": {"type": "string"},
          "status": {"type": "string", "enum": ["pending", "in_progress", "completed"]},
          "activeForm": {"type": "string"}
        },
        "required": ["content", "status", "activeForm"]
      },
      "description": "The updated todo list"
    }
  },
  "required": ["todos"]
}`)
	return ToolDefinition{Name: "TodoWrite", Description: t.Description(), InputSchema: schema}
}

// TodoItem represents a single todo entry.
type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm"`
}

func (t *TodoWriteTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Todos []TodoItem `json:"todos"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.MarshalIndent(params.Todos, "", "  ")
	if err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("marshal failed: %v", err)}, nil
	}

	if err := os.WriteFile(t.storePath, data, 0644); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("write failed: %v", err)}, nil
	}

	var sb strings.Builder
	sb.WriteString("Todos updated:\n")
	for _, todo := range params.Todos {
		icon := "[ ]"
		switch todo.Status {
		case "in_progress":
			icon = "[~]"
		case "completed":
			icon = "[x]"
		}
		sb.WriteString(fmt.Sprintf("  %s %s\n", icon, todo.Content))
	}
	return &ToolResult{Content: sb.String()}, nil
}
