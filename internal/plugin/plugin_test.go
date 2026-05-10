package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tetexu/tlaude-code/internal/plugin/lua"
)

func TestParseManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "plugin.yaml")

	yamlContent := `
name: test-plugin
version: 1.0.0
description: "A test plugin"
author: "test"
type: lua
entrypoint: main.lua
provides:
  - tool
  - hook
config:
  key: value
tools:
  - name: my_tool
    description: "A test tool"
    schema:
      type: object
`
	if err := os.WriteFile(manifestPath, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := ParseManifest(manifestPath)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}

	if m.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", m.Version)
	}
	if m.Type != TypeLua {
		t.Errorf("expected type lua, got %q", m.Type)
	}
	if m.Author != "test" {
		t.Errorf("expected author 'test', got %q", m.Author)
	}
	if len(m.Provides) != 2 {
		t.Errorf("expected 2 provides, got %d", len(m.Provides))
	}
	if m.Entrypoint != "main.lua" {
		t.Errorf("expected entrypoint 'main.lua', got %q", m.Entrypoint)
	}
	if len(m.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(m.Tools))
	}
	if m.Tools[0].Name != "my_tool" {
		t.Errorf("expected tool name 'my_tool', got %q", m.Tools[0].Name)
	}
}

func TestManifestValidate(t *testing.T) {
	tests := []struct {
		name    string
		m       Manifest
		wantErr bool
	}{
		{
			name: "valid lua",
			m: Manifest{
				Name: "test", Version: "1.0", Description: "desc",
				Type: TypeLua, Entrypoint: "main.lua",
				Provides: []Provides{ProvidesTool},
			},
			wantErr: false,
		},
		{
			name: "valid mcp",
			m: Manifest{
				Name: "test", Version: "1.0", Description: "desc",
				Type: TypeMCP, MCP: &MCPConfig{Command: "npx"},
				Provides: []Provides{ProvidesTool},
			},
			wantErr: false,
		},
		{
			name: "valid hybrid",
			m: Manifest{
				Name: "test", Version: "1.0", Description: "desc",
				Type: TypeHybrid, Entrypoint: "main.lua",
				MCP: &MCPConfig{Command: "npx"},
				Provides: []Provides{ProvidesTool},
			},
			wantErr: false,
		},
		{
			name:    "missing name",
			m:       Manifest{Version: "1.0", Description: "desc", Type: TypeLua, Entrypoint: "main.lua", Provides: []Provides{ProvidesTool}},
			wantErr: true,
		},
		{
			name:    "missing version",
			m:       Manifest{Name: "test", Description: "desc", Type: TypeLua, Entrypoint: "main.lua", Provides: []Provides{ProvidesTool}},
			wantErr: true,
		},
		{
			name:    "missing description",
			m:       Manifest{Name: "test", Version: "1.0", Type: TypeLua, Entrypoint: "main.lua", Provides: []Provides{ProvidesTool}},
			wantErr: true,
		},
		{
			name:    "missing type",
			m:       Manifest{Name: "test", Version: "1.0", Description: "desc", Entrypoint: "main.lua", Provides: []Provides{ProvidesTool}},
			wantErr: true,
		},
		{
			name:    "invalid type",
			m:       Manifest{Name: "test", Version: "1.0", Description: "desc", Type: "bad", Entrypoint: "main.lua", Provides: []Provides{ProvidesTool}},
			wantErr: true,
		},
		{
			name:    "lua missing entrypoint",
			m:       Manifest{Name: "test", Version: "1.0", Description: "desc", Type: TypeLua, Provides: []Provides{ProvidesTool}},
			wantErr: true,
		},
		{
			name:    "mcp missing command",
			m:       Manifest{Name: "test", Version: "1.0", Description: "desc", Type: TypeMCP, MCP: &MCPConfig{}, Provides: []Provides{ProvidesTool}},
			wantErr: true,
		},
		{
			name:    "missing provides",
			m:       Manifest{Name: "test", Version: "1.0", Description: "desc", Type: TypeLua, Entrypoint: "main.lua"},
			wantErr: true,
		},
		{
			name:    "invalid provides value",
			m:       Manifest{Name: "test", Version: "1.0", Description: "desc", Type: TypeLua, Entrypoint: "main.lua", Provides: []Provides{"bad"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegistryCRUD(t *testing.T) {
	r := NewRegistry()

	p := &basePlugin{
		name:        "test",
		version:     "1.0",
		description: "test plugin",
		pluginType:  TypeLua,
		provides:    []Provides{ProvidesTool, ProvidesHook},
		enabled:     true,
	}

	// Register
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if r.Len() != 1 {
		t.Errorf("expected len 1, got %d", r.Len())
	}

	// Duplicate register
	if err := r.Register(p); err == nil {
		t.Error("expected error on duplicate register")
	}

	// Get
	got, ok := r.Get("test")
	if !ok {
		t.Fatal("Get: plugin not found")
	}
	if got.Name() != "test" {
		t.Errorf("expected name 'test', got %q", got.Name())
	}

	// List
	list := r.List()
	if len(list) != 1 {
		t.Errorf("expected 1 plugin in list, got %d", len(list))
	}

	// ListByProvides
	toolPlugins := r.ListByProvides(ProvidesTool)
	if len(toolPlugins) != 1 {
		t.Errorf("expected 1 tool plugin, got %d", len(toolPlugins))
	}

	hookPlugins := r.ListByProvides(ProvidesHook)
	if len(hookPlugins) != 1 {
		t.Errorf("expected 1 hook plugin, got %d", len(hookPlugins))
	}

	agentPlugins := r.ListByProvides(ProvidesAgent)
	if len(agentPlugins) != 0 {
		t.Errorf("expected 0 agent plugins, got %d", len(agentPlugins))
	}

	// Get non-existent
	_, ok = r.Get("nonexistent")
	if ok {
		t.Error("expected false for non-existent plugin")
	}

	// Remove
	if !r.Remove("test") {
		t.Error("expected Remove to return true")
	}
	if r.Len() != 0 {
		t.Errorf("expected len 0 after remove, got %d", r.Len())
	}

	// Remove non-existent
	if r.Remove("test") {
		t.Error("expected Remove to return false for non-existent plugin")
	}
}

func TestPluginDiscovery(t *testing.T) {
	pluginsDir := t.TempDir()

	// Create a valid plugin subdirectory
	validDir := filepath.Join(pluginsDir, "valid-plugin")
	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifestYAML := `
name: valid-plugin
version: 1.0.0
description: "A valid plugin"
type: lua
entrypoint: main.lua
provides:
  - tool
`
	if err := os.WriteFile(filepath.Join(validDir, "plugin.yaml"), []byte(manifestYAML), 0644); err != nil {
		t.Fatal(err)
	}
	// Also create the entrypoint so LoadPlugin succeeds
	if err := os.WriteFile(filepath.Join(validDir, "main.lua"), []byte("-- placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create an invalid plugin subdirectory (missing required fields)
	invalidDir := filepath.Join(pluginsDir, "invalid-plugin")
	if err := os.MkdirAll(invalidDir, 0755); err != nil {
		t.Fatal(err)
	}
	invalidYAML := `
name: invalid-plugin
# missing version, description, type, entrypoint, provides
`
	if err := os.WriteFile(filepath.Join(invalidDir, "plugin.yaml"), []byte(invalidYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a plain directory (no plugin.yaml) — should be skipped
	plainDir := filepath.Join(pluginsDir, "not-a-plugin")
	if err := os.MkdirAll(plainDir, 0755); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(pluginsDir)
	manifests, err := loader.Discover()
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(manifests) != 1 {
		t.Errorf("expected 1 valid manifest, got %d", len(manifests))
	}
	if len(manifests) > 0 && manifests[0].Name != "valid-plugin" {
		t.Errorf("expected 'valid-plugin', got %q", manifests[0].Name)
	}

	// Test LoadPlugin
	ctx := context.Background()
	if len(manifests) > 0 {
		plugin, err := loader.LoadPlugin(ctx, manifests[0])
		if err != nil {
			t.Fatalf("LoadPlugin: %v", err)
		}
		if plugin.Name() != "valid-plugin" {
			t.Errorf("expected name 'valid-plugin', got %q", plugin.Name())
		}
	}
}

func TestHookDispatch(t *testing.T) {
	hr := NewHookRegistry()

	called := false
	hr.Register(HookToolBefore, "test-hook", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
		called = true
		if event.ToolName != "bash" {
			t.Errorf("expected tool name 'bash', got %q", event.ToolName)
		}
		return &HookResult{Allow: true}, nil
	})

	results, err := hr.Dispatch(context.Background(), HookToolBefore, &HookEvent{
		Point:    HookToolBefore,
		ToolName: "bash",
		Args:     map[string]interface{}{"command": "ls"},
	})

	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if !called {
		t.Error("hook was not called")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Allow {
		t.Error("expected result.Allow = true")
	}

	// Test dispatching to a point with no handlers
	results, err = hr.Dispatch(context.Background(), HookSessionStart, &HookEvent{
		Point:    HookSessionStart,
		SessionID: "session-1",
	})
	if err != nil {
		t.Errorf("Dispatch on empty hook point should not error: %v", err)
	}
	if results != nil {
		t.Error("expected nil results for empty hook point")
	}
}

func TestHookMultipleHandlers(t *testing.T) {
	hr := NewHookRegistry()

	var order []int
	hr.Register(HookToolBefore, "first", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
		order = append(order, 1)
		return &HookResult{Allow: true}, nil
	})
	hr.Register(HookToolBefore, "second", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
		order = append(order, 2)
		return &HookResult{Deny: true, Reason: "blocked"}, nil
	})
	hr.Register(HookToolBefore, "third", func(ctx context.Context, event *HookEvent) (*HookResult, error) {
		order = append(order, 3)
		return nil, nil // pass-through with nil result
	})

	results, err := hr.Dispatch(context.Background(), HookToolBefore, &HookEvent{
		Point:    HookToolBefore,
		ToolName: "bash",
	})

	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(order))
	}
	if order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("expected call order [1,2,3], got %v", order)
	}
	// "third" returned nil, so results should have 2 entries
	if len(results) != 2 {
		t.Fatalf("expected 2 results (third returned nil), got %d", len(results))
	}
	if !results[1].Deny {
		t.Error("expected second result to deny")
	}
}

func TestPluginInterface(t *testing.T) {
	p := &basePlugin{
		name:        "test-plugin",
		version:     "2.0.0",
		description: "A test plugin",
		pluginType:  TypeLua,
		provides:    []Provides{ProvidesTool},
		enabled:     false,
	}

	if p.Name() != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", p.Name())
	}
	if p.Version() != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", p.Version())
	}
	if p.Description() != "A test plugin" {
		t.Errorf("expected description, got %q", p.Description())
	}
	if p.Type() != TypeLua {
		t.Errorf("expected TypeLua, got %q", p.Type())
	}
	if !p.Enabled() {
		p.SetEnabled(true)
	}
	if !p.Enabled() {
		t.Error("expected Enabled() to be true after SetEnabled(true)")
	}

	provides := p.Provides()
	if len(provides) != 1 || provides[0] != ProvidesTool {
		t.Error("expected Provides() to return [tool]")
	}

	// Load and Unload should be no-ops in Phase 1
	ctx := context.Background()
	if err := p.Load(ctx); err != nil {
		t.Errorf("Load should not error: %v", err)
	}
	if err := p.Unload(ctx); err != nil {
		t.Errorf("Unload should not error: %v", err)
	}
}

func TestManagerLoadAll(t *testing.T) {
	pluginsDir := t.TempDir()

	// Create a valid plugin
	pluginDir := filepath.Join(pluginsDir, "test-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	yamlContent := `
name: test-plugin
version: 1.0.0
description: "Test plugin"
type: lua
entrypoint: main.lua
provides:
  - hook
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "main.lua"), []byte("-- placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	loader := NewLoader(pluginsDir)
	registry := NewRegistry()
	manager := NewManager(pluginsDir, loader, registry, lua.Options{Timeout: 5})

	ctx := context.Background()
	if err := manager.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	list := manager.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 loaded plugin, got %d", len(list))
	}
	if list[0].Name() != "test-plugin" {
		t.Errorf("expected 'test-plugin', got %q", list[0].Name())
	}

	// LoadAll again should skip already-loaded plugin
	if err := manager.LoadAll(ctx); err != nil {
		t.Fatalf("LoadAll second time: %v", err)
	}

	// Unload
	if err := manager.Unload(ctx, "test-plugin"); err != nil {
		t.Fatalf("Unload: %v", err)
	}
	if len(manager.List()) != 0 {
		t.Errorf("expected 0 plugins after unload, got %d", len(manager.List()))
	}

	// Reload
	if err := manager.Reload(ctx, "test-plugin"); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(manager.List()) != 1 {
		t.Errorf("expected 1 plugin after reload, got %d", len(manager.List()))
	}
}
