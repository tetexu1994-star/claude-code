package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tetexu/tlaude-code/internal/tool/permission"
)

func TestRegistryRegister(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	if reg.Len() != 0 {
		t.Errorf("expected empty registry, got %d tools", reg.Len())
	}

	tl := &mockTool{name: "test"}
	if err := reg.Register(tl); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.Len() != 1 {
		t.Errorf("expected 1 tool, got %d", reg.Len())
	}

	// Duplicate registration
	if err := reg.Register(tl); err == nil {
		t.Error("expected error for duplicate registration")
	}

	// Nil tool
	if err := reg.Register(nil); err == nil {
		t.Error("expected error for nil tool")
	}

	// Empty name
	emptyTool := &mockTool{name: ""}
	if err := reg.Register(emptyTool); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestRegistryGet(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	reg.Register(&mockTool{name: "bash"})
	reg.Register(&mockTool{name: "read_file"})

	tl, ok := reg.Get("bash")
	if !ok {
		t.Error("expected to find bash tool")
	}
	if tl.Name() != "bash" {
		t.Errorf("expected name 'bash', got %q", tl.Name())
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent tool")
	}
}

func TestRegistryGetAll(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	reg.Register(&mockTool{name: "bash", enabled: true})
	reg.Register(&mockTool{name: "read_file", enabled: true})
	reg.Register(&mockTool{name: "disabled_tool", enabled: false})
	reg.Register(&mockTool{name: "write_file", enabled: true})

	t.Run("nil context returns all enabled", func(t *testing.T) {
		tools := reg.GetAll(nil)
		if len(tools) != 3 {
			t.Errorf("expected 3 enabled tools, got %d", len(tools))
		}
	})

	t.Run("bypass mode returns all enabled", func(t *testing.T) {
		ctx := permission.NewContext(permission.ModeBypassPermissions)
		tools := reg.GetAll(ctx)
		if len(tools) != 3 {
			t.Errorf("expected 3 enabled tools in bypass mode, got %d", len(tools))
		}
	})

	t.Run("denied tool is excluded", func(t *testing.T) {
		ctx := permission.NewContext(permission.ModeAccepts)
		ctx.AddDenyRule(permission.SourceUser, "write_file", nil)
		tools := reg.GetAll(ctx)
		if len(tools) != 2 {
			t.Errorf("expected 2 tools after deny, got %d", len(tools))
		}
		for _, tool := range tools {
			if tool.Name() == "write_file" {
				t.Error("write_file should have been excluded")
			}
		}
	})
}

func TestRegistryAssembleToolPool(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	reg.Register(&mockTool{name: "bash", enabled: true})
	reg.Register(&mockTool{name: "read_file", enabled: true})

	mcpTools := []Tool{
		&mockTool{name: "mcp__server1__tool1", enabled: true},
		&mockTool{name: "mcp__server1__tool2", enabled: true},
	}

	pool := reg.AssembleToolPool(nil, mcpTools)
	if len(pool) != 4 {
		t.Errorf("expected 4 tools in pool, got %d", len(pool))
	}

	// With deny on MCP tool
	ctx := permission.NewContext(permission.ModeAccepts)
	ctx.AddDenyRule(permission.SourceUser, "mcp__server1__tool1", nil)
	pool = reg.AssembleToolPool(ctx, mcpTools)
	if len(pool) != 3 {
		t.Errorf("expected 3 tools after MCP deny, got %d", len(pool))
	}
}

func TestDefaultRegistry(t *testing.T) {
	t.Parallel()

	reg := DefaultRegistry()
	if reg.Len() != 16 {
		t.Errorf("expected 16 default tools, got %d", reg.Len())
	}

	expected := []string{
		"bash", "read_file", "write_file", "edit_file", "Glob", "Grep",
		"WebFetch", "WebSearch", "TodoWrite", "Agent",
		"TaskCreate", "TaskGet", "TaskList", "TaskStop", "EnterPlanMode", "ExitPlanMode",
	}
	for _, name := range expected {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("expected tool %q in default registry", name)
		}
	}
}

func TestDefaultTools(t *testing.T) {
	t.Parallel()

	tools := DefaultTools()
	if len(tools) != 16 {
		t.Fatalf("expected 16 default tools, got %d", len(tools))
	}

	toolNames := make(map[string]bool)
	for _, tool := range tools {
		if tool.Name() == "" {
			t.Error("tool name must not be empty")
		}
		if tool.Description() == "" {
			t.Errorf("tool %q: description must not be empty", tool.Name())
		}
		if !tool.IsEnabled() {
			t.Errorf("tool %q: expected enabled by default", tool.Name())
		}
		td := tool.ToolDefinition()
		if td.Name != tool.Name() {
			t.Errorf("tool %q: definition name mismatch %q", tool.Name(), td.Name)
		}
		if td.InputSchema == nil {
			t.Errorf("tool %q: input schema must not be nil", tool.Name())
		}
		toolNames[tool.Name()] = true
	}

	expectedTools := []string{
		"bash", "read_file", "write_file", "edit_file", "Glob", "Grep",
		"WebFetch", "WebSearch", "TodoWrite", "Agent",
		"TaskCreate", "TaskGet", "TaskList", "TaskStop", "EnterPlanMode", "ExitPlanMode",
	}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("expected tool %q in defaults", name)
		}
	}
}

func TestToolDefinitionJSON(t *testing.T) {
	t.Parallel()

	tools := DefaultTools()
	for _, tool := range tools {
		td := tool.ToolDefinition()
		var m map[string]interface{}
		if err := json.Unmarshal(td.InputSchema, &m); err != nil {
			t.Errorf("tool %q: invalid InputSchema JSON: %v", tool.Name(), err)
		}
		if m["type"] != "object" {
			t.Errorf("tool %q: schema type should be 'object', got %v", tool.Name(), m["type"])
		}
		if _, ok := m["properties"]; !ok {
			t.Errorf("tool %q: schema missing 'properties'", tool.Name())
		}
	}
}

// --- mock tool for testing ---

type mockTool struct {
	name        string
	enabled     bool
	concurrency bool
}

func (m *mockTool) Name() string           { return m.name }
func (m *mockTool) Description() string     { return "mock tool: " + m.name }
func (m *mockTool) IsEnabled() bool         { return m.enabled }
func (m *mockTool) IsConcurrencySafe() bool { return m.concurrency }
func (m *mockTool) ToolDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        m.name,
		Description: m.Description(),
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}
}
func (m *mockTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	return &ToolResult{Content: "ok"}, nil
}
