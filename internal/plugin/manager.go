package plugin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/tetexu/tlaude-code/internal/plugin/lua"
	"github.com/tetexu/tlaude-code/internal/tool"
)

// Manager orchestrates plugin discovery, loading, and lifecycle.
type Manager struct {
	pluginsDir string
	loader     *Loader
	registry   *Registry
	luaEngine  *lua.Engine
	toolBridge *lua.ToolBridge
	hookBridge *lua.HookBridge
}

// NewManager creates a plugin manager with the given components.
func NewManager(pluginsDir string, loader *Loader, registry *Registry, luaOpts lua.Options) *Manager {
	engine := lua.NewEngine(luaOpts)
	tb := lua.NewToolBridge(engine)
	engine.SetToolBridge(tb)
	hb := lua.NewHookBridge(engine)
	engine.SetHookBridge(hb)

	return &Manager{
		pluginsDir: pluginsDir,
		loader:     loader,
		registry:   registry,
		luaEngine:  engine,
		toolBridge: tb,
		hookBridge: hb,
	}
}

// Start initializes the Lua engine (pre-warms VM pool).
func (m *Manager) Start(ctx context.Context) error {
	if err := m.luaEngine.Start(); err != nil {
		return fmt.Errorf("starting lua engine: %w", err)
	}
	return m.LoadAll(ctx)
}

// Stop tears down the Lua engine.
func (m *Manager) Stop() {
	m.luaEngine.Stop()
}

// LoadAll discovers, validates, and loads all enabled plugins from the
// plugins directory.
func (m *Manager) LoadAll(ctx context.Context) error {
	manifests, err := m.loader.Discover()
	if err != nil {
		return fmt.Errorf("discovering plugins: %w", err)
	}

	slog.Info("discovered plugins", "count", len(manifests))

	for _, manifest := range manifests {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := m.loadOne(ctx, manifest); err != nil {
			slog.Warn("failed to load plugin", "name", manifest.Name, "error", err)
			continue
		}
	}

	return nil
}

// LoadOne loads a single plugin by name from the plugins directory.
func (m *Manager) LoadOne(ctx context.Context, name string) error {
	manifests, err := m.loader.Discover()
	if err != nil {
		return fmt.Errorf("discovering plugins: %w", err)
	}

	for _, manifest := range manifests {
		if manifest.Name == name {
			return m.loadOne(ctx, manifest)
		}
	}

	return fmt.Errorf("plugin %q not found in %s", name, m.pluginsDir)
}

// Unload unloads a plugin by name and removes it from the registry.
func (m *Manager) Unload(ctx context.Context, name string) error {
	p, ok := m.registry.Get(name)
	if !ok {
		return fmt.Errorf("plugin %q is not loaded", name)
	}

	if err := p.Unload(ctx); err != nil {
		return fmt.Errorf("unloading plugin %q: %w", name, err)
	}

	m.registry.Remove(name)
	return nil
}

// Reload reloads a plugin (unload + load).
func (m *Manager) Reload(ctx context.Context, name string) error {
	if err := m.Unload(ctx, name); err != nil {
		// If it wasn't loaded, just try to load it fresh.
		if _, notLoaded := m.registry.Get(name); notLoaded {
			// was loaded, unload failed — propagate
			return err
		}
	}
	return m.LoadOne(ctx, name)
}

// LuaEngineStatus returns pool size and active VM count for the Lua engine.
func (m *Manager) LuaEngineStatus() (poolSize, activeVMs int) {
	return m.luaEngine.PoolSize(), m.luaEngine.ActiveVMs()
}

// List returns all loaded plugins.
func (m *Manager) List() []Plugin {
	return m.registry.List()
}

// loadOne validates and registers a single plugin.
func (m *Manager) loadOne(ctx context.Context, manifest *Manifest) error {
	if _, exists := m.registry.Get(manifest.Name); exists {
		slog.Debug("plugin already loaded, skipping", "name", manifest.Name)
		return nil
	}

	plugin, err := m.loader.LoadPlugin(ctx, manifest)
	if err != nil {
		return fmt.Errorf("loading plugin %q: %w", manifest.Name, err)
	}

	// Set Lua engine on the plugin before loading (no-op for non-Lua plugins).
	if bp, ok := plugin.(*basePlugin); ok {
		bp.luaEngine = m.luaEngine
		bp.hookRegistry = NewHookRegistry()
		m.hookBridge.SetHookRegistry(manifest.Name, bp.hookRegistry)
	}

	if err := plugin.Load(ctx); err != nil {
		return fmt.Errorf("initializing plugin %q: %w", manifest.Name, err)
	}

	if err := m.registry.Register(plugin); err != nil {
		return fmt.Errorf("registering plugin %q: %w", manifest.Name, err)
	}

	slog.Info("plugin registered", "name", plugin.Name(), "type", plugin.Type(), "version", plugin.Version())
	return nil
}

// RegisterPluginTools registers all Lua plugin tools into the given Go tool registry.
func (m *Manager) RegisterPluginTools(goRegistry *tool.Registry) error {
	for _, lt := range m.toolBridge.GetGoTools() {
		if err := goRegistry.Register(lt); err != nil {
			return fmt.Errorf("registering plugin tool %q: %w", lt.Name(), err)
		}
	}
	return nil
}
