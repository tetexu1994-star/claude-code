package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"log/slog"

	"github.com/tetexu/tlaude-code/internal/logging"
	"github.com/tetexu/tlaude-code/internal/plugin/lua"
)

// Loader discovers and loads plugins from a directory.
type Loader struct {
	dir string
}

// NewLoader creates a plugin loader that watches the given directory.
func NewLoader(pluginsDir string) *Loader {
	return &Loader{dir: pluginsDir}
}

// Discover scans the plugins directory for subdirectories containing
// plugin.yaml files. Returns manifests without loading them.
func (l *Loader) Discover() ([]*Manifest, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugins dir %s: %w", l.dir, err)
	}

	var manifests []*Manifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(l.dir, entry.Name(), "plugin.yaml")
		m, err := ParseManifest(manifestPath)
		if err != nil {
			logging.Warn("skipping invalid plugin manifest", "path", manifestPath, "error", err)
			continue
		}

		manifests = append(manifests, m)
	}

	return manifests, nil
}

// LoadPlugin creates a Plugin instance from a manifest. For Lua plugins it
// reads the entrypoint script (doesn't execute yet). For MCP plugins it
// creates an MCP plugin descriptor.
func (l *Loader) LoadPlugin(ctx context.Context, m *Manifest) (Plugin, error) {
	switch m.Type {
	case TypeLua:
		return l.newLuaPlugin(m)
	case TypeMCP:
		return l.newMCPPlugin(m)
	case TypeHybrid:
		return l.newHybridPlugin(m)
	default:
		return nil, fmt.Errorf("unsupported plugin type: %s", m.Type)
	}
}

func (l *Loader) newLuaPlugin(m *Manifest) (*basePlugin, error) {
	// In Phase 1 we only validate the entrypoint exists; execution is Phase 2.
	entrypoint := filepath.Join(l.dir, m.Name, m.Entrypoint)
	if _, err := os.Stat(entrypoint); os.IsNotExist(err) {
		return nil, fmt.Errorf("lua entrypoint not found: %s", entrypoint)
	}

	return &basePlugin{
		name:        m.Name,
		version:     m.Version,
		description: m.Description,
		pluginType:  m.Type,
		provides:    m.Provides,
		enabled:     true,
		config:      m.Config,
		manifestPath: filepath.Join(l.dir, m.Name, "plugin.yaml"),
		luaScript:   entrypoint,
	}, nil
}

func (l *Loader) newMCPPlugin(m *Manifest) (*basePlugin, error) {
	return &basePlugin{
		name:        m.Name,
		version:     m.Version,
		description: m.Description,
		pluginType:  m.Type,
		provides:    m.Provides,
		enabled:     true,
		config:      m.Config,
		manifestPath: filepath.Join(l.dir, m.Name, "plugin.yaml"),
		mcpCfg:      m.MCP,
	}, nil
}

func (l *Loader) newHybridPlugin(m *Manifest) (*basePlugin, error) {
	// Hybrid: validate both entrypoint and MCP config exist.
	entrypoint := filepath.Join(l.dir, m.Name, m.Entrypoint)
	if _, err := os.Stat(entrypoint); os.IsNotExist(err) {
		return nil, fmt.Errorf("lua entrypoint not found for hybrid plugin: %s", entrypoint)
	}

	return &basePlugin{
		name:        m.Name,
		version:     m.Version,
		description: m.Description,
		pluginType:  m.Type,
		provides:    m.Provides,
		enabled:     true,
		config:      m.Config,
		manifestPath: filepath.Join(l.dir, m.Name, "plugin.yaml"),
		luaScript:   entrypoint,
		mcpCfg:      m.MCP,
	}, nil
}

// basePlugin is the concrete Plugin implementation for all types.
type basePlugin struct {
	mu           sync.Mutex
	name         string
	version      string
	description  string
	pluginType   Type
	provides     []Provides
	enabled      bool
	config       map[string]interface{}
	manifestPath string
	luaScript    string
	mcpCfg       *MCPConfig
	luaEngine    *lua.Engine
	compiled     *lua.CompiledScript
	hookRegistry *HookRegistry
}

func (p *basePlugin) Name() string          { return p.name }
func (p *basePlugin) Version() string       { return p.version }
func (p *basePlugin) Description() string   { return p.description }
func (p *basePlugin) Type() Type            { return p.pluginType }
func (p *basePlugin) Provides() []Provides  { return p.provides }
func (p *basePlugin) Enabled() bool         { p.mu.Lock(); defer p.mu.Unlock(); return p.enabled }
func (p *basePlugin) SetEnabled(v bool)     { p.mu.Lock(); defer p.mu.Unlock(); p.enabled = v }

func (p *basePlugin) Load(ctx context.Context) error {
	if p.pluginType == TypeLua && p.luaScript != "" && p.luaEngine != nil {
		code, err := os.ReadFile(p.luaScript)
		if err != nil {
			return fmt.Errorf("reading lua script %s: %w", p.luaScript, err)
		}

		pluginDir := filepath.Dir(p.luaScript)
		compiled, err := p.luaEngine.Compile(p.name, string(code))
		if err != nil {
			return fmt.Errorf("compiling lua script %s: %w", p.name, err)
		}
		compiled.PluginDir = pluginDir
		compiled.Config = p.config
		compiled.Plugins = []string{p.name}
		p.compiled = compiled

		if err := p.luaEngine.Execute(ctx, compiled); err != nil {
			return fmt.Errorf("executing lua script %s: %w", p.name, err)
		}
	}

	slog.Info("plugin loaded", "name", p.name, "type", p.pluginType)
	return nil
}

func (p *basePlugin) Unload(ctx context.Context) error {
	if p.compiled != nil {
		p.compiled.Close()
		p.compiled = nil
	}
	slog.Info("plugin unloaded", "name", p.name, "type", p.pluginType)
	return nil
}
