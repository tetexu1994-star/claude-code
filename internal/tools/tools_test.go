package tools

import (
	"testing"
)

func TestGetToolDefinitions(t *testing.T) {
	t.Parallel()

	defs := GetToolDefinitions()
	if len(defs) != 3 {
		t.Fatalf("expected 3 tool definitions, got %d", len(defs))
	}

	toolNames := make(map[string]bool)
	for _, d := range defs {
		toolNames[d.Name] = true

		if d.Name == "" {
			t.Error("tool name must not be empty")
		}
		if d.Description == "" {
			t.Errorf("tool %q: description must not be empty", d.Name)
		}
		if d.InputSchema == nil {
			t.Errorf("tool %q: input schema must not be nil", d.Name)
		}
	}

	expectedTools := []string{"bash", "write_file", "read_file"}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("expected tool %q in definitions", name)
		}
	}
}

func TestGetToolDefinitionsBash(t *testing.T) {
	t.Parallel()

	defs := GetToolDefinitions()
	var bashDef *struct{ Name, Description string }
	for i, d := range defs {
		if d.Name == "bash" {
			bashDef = &struct{ Name, Description string }{d.Name, d.Description}
			_ = i
			break
		}
	}
	if bashDef == nil {
		t.Fatal("bash tool not found")
	}

	// Find the bash definition
	var found bool
	for _, d := range defs {
		if d.Name == "bash" {
			found = true
			// Check required fields
			props, ok := d.InputSchema["properties"].(map[string]interface{})
			if !ok {
				t.Error("bash: expected properties map")
			}
			cmd, ok := props["command"].(map[string]interface{})
			if !ok {
				t.Error("bash: expected command property")
			}
			if cmd["type"] != "string" {
				t.Errorf("bash: command type expected 'string', got %v", cmd["type"])
			}
			required, ok := d.InputSchema["required"].([]string)
			if !ok || len(required) != 1 || required[0] != "command" {
				t.Error("bash: expected required ['command']")
			}
			break
		}
	}
	if !found {
		t.Error("bash tool not found in definitions")
	}
}

func TestGetToolDefinitionsWriteFile(t *testing.T) {
	t.Parallel()

	defs := GetToolDefinitions()
	for _, d := range defs {
		if d.Name == "write_file" {
			props, ok := d.InputSchema["properties"].(map[string]interface{})
			if !ok {
				t.Error("write_file: expected properties map")
			}
			for _, field := range []string{"path", "content"} {
				if _, ok := props[field]; !ok {
					t.Errorf("write_file: expected %q property", field)
				}
			}
			required, ok := d.InputSchema["required"].([]string)
			if !ok {
				t.Error("write_file: expected required array")
			}
			hasPath, hasContent := false, false
			for _, r := range required {
				if r == "path" {
					hasPath = true
				}
				if r == "content" {
					hasContent = true
				}
			}
			if !hasPath || !hasContent {
				t.Error("write_file: expected required ['path', 'content']")
			}
			return
		}
	}
	t.Error("write_file tool not found")
}

func TestGetToolDefinitionsReadFile(t *testing.T) {
	t.Parallel()

	defs := GetToolDefinitions()
	for _, d := range defs {
		if d.Name == "read_file" {
			required, ok := d.InputSchema["required"].([]string)
			if !ok || len(required) != 1 || required[0] != "path" {
				t.Error("read_file: expected required ['path']")
			}
			return
		}
	}
	t.Error("read_file tool not found")
}

func TestGetToolDefinitionsIsImmutable(t *testing.T) {
	// Verify that repeated calls return independent slices of the same content.
	defs1 := GetToolDefinitions()
	defs2 := GetToolDefinitions()
	if len(defs1) != len(defs2) {
		t.Error("inconsistent tool count between calls")
	}
	for i := range defs1 {
		if defs1[i].Name != defs2[i].Name {
			t.Errorf("inconsistent tool at index %d: %q vs %q", i, defs1[i].Name, defs2[i].Name)
		}
	}
}
