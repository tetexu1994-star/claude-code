// Package plugin provides the plugin system core for Tlaude Code.
// It supports Lua scripts (embedded), MCP subprocess servers, and hybrid plugins
// that combine both.
package plugin

import "context"

// Type identifies what kind of plugin backend.
type Type string

const (
	// TypeLua is an embedded gopher-lua script plugin.
	TypeLua Type = "lua"
	// TypeMCP is an MCP subprocess server plugin (uses existing mcp.Client).
	TypeMCP Type = "mcp"
	// TypeHybrid combines Lua and MCP capabilities.
	TypeHybrid Type = "hybrid"
)

// Provides identifies what extension points a plugin offers.
type Provides string

const (
	// ProvidesTool means the plugin registers new tools in the tool registry.
	ProvidesTool Provides = "tool"
	// ProvidesProvider means the plugin registers new LLM providers.
	ProvidesProvider Provides = "provider"
	// ProvidesSandbox means the plugin registers a sandbox implementation.
	ProvidesSandbox Provides = "sandbox"
	// ProvidesHook means the plugin registers lifecycle hooks.
	ProvidesHook Provides = "hook"
	// ProvidesAgent means the plugin provides a custom agent implementation.
	ProvidesAgent Provides = "agent"
)

// Plugin is the interface all loaded plugins must satisfy.
type Plugin interface {
	// Name returns the plugin's unique name.
	Name() string
	// Version returns the plugin's semantic version.
	Version() string
	// Description returns a human-readable summary.
	Description() string
	// Type returns the plugin backend type (lua, mcp, or hybrid).
	Type() Type
	// Provides returns the extension points this plugin implements.
	Provides() []Provides
	// Enabled returns whether the plugin is currently active.
	Enabled() bool
	// SetEnabled changes the plugin's active state.
	SetEnabled(bool)
	// Load initializes the plugin. For Lua plugins this parses the script;
	// for MCP plugins this validates the MCP command is available.
	Load(ctx context.Context) error
	// Unload tears down the plugin, releasing any resources.
	Unload(ctx context.Context) error
}
