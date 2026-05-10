# Plugin System — Architecture Decision

## Context

Tlaude Code needs a plugin system that allows third-party extensions for:
- **Tools** (new capabilities)
- **LLM Providers** (new backends)
- **Sandbox implementations** (execution isolation)
- **Lifecycle Hooks** (pre/post execution)
- **Agent implementations** (custom agent logic)
- **UI customizers** (TUI enhancements)

The system must be cross-platform (macOS + Linux), distributable via Homebrew,
and safe for user-installed third-party code.

## Decision: 2-Tier Plugin Backend

### Tier 1: Embedded Lua Scripts (Primary — In-Process)

**Engine**: [gopher-lua](https://github.com/yuin/gopher-lua) (pure Go, no CGo)

**Why not Go native `plugin` package:**
| Factor | Go `plugin` (.so) | gopher-lua |
|--------|-------------------|------------|
| macOS support | ❌ CGo needed, fragile | ✅ Pure Go |
| Hot-reload | ❌ Once loaded, forever | ✅ Re-parse on reload |
| Go version lock | ❌ Must match exact version | ✅ None |
| Security | ❌ Full native access | ✅ Sandboxed VM |
| Cross-platform | ❌ Unix-only | ✅ All Go targets |
| Distribution size | ❌ Separate .so per platform | ✅ Single script file |

**Use cases:**
- Custom tools with simple logic (data transform, API call, file processing)
- Pre/post hooks (validate input, log output, notify webhook)
- Custom agent decision logic
- User automation scripts

### Tier 2: MCP Subprocess (Existing — Out-of-Process)

Already implemented in `internal/mcp/`. Handles:
- Language-agnostic (Python, JS, Rust, Go)
- Process isolation (plugin crash ≠ host crash)
- Heavy computation
- External service integration

**No change needed** — MCP already works. The plugin system will register MCP
servers automatically from plugin manifests.

## Extension Points

Each plugin declares what it provides via a YAML manifest:

```
provides:
  - tool           → registered in tool.Registry
  - provider       → registered in llm.RegisterFactory
  - sandbox        → registered as a Sandboxer
  - hook           → lifecycle hooks (see below)
  - agent          → custom agent
```

### Hook System

Lifecycle hooks allow plugins to intercept key events:

| Hook | Trigger | Use Case |
|------|---------|----------|
| `on_tool_before` | Before tool execution | Validation, rate limiting |
| `on_tool_after` | After tool execution | Logging, notification |
| `on_session_start` | Session begins | initialize plugin state |
| `on_session_end` | Session ends | Cleanup, summary |
| `on_message` | User message received | Custom preprocessing |

Hooks are Lua functions registered with specific names.

## Manifest Format

```yaml
# ~/.tlaude-code/plugins/my-tools/plugin.yaml
name: my-tools
version: 1.0.0
description: "Custom data tools"
author: "user"

type: lua  # lua | mcp | hybrid
entrypoint: main.lua

# For MCP plugins (when type=mcp or hybrid)
mcp:
  command: npx
  args: ["@my/mcp-server"]

provides:
  - tool
  - hook

config:
  my_setting: "default"

# Optional: tools this plugin adds (for discovery)
tools:
  - name: my_transform
    description: "Transform data"
    schema:
      type: object
      properties:
        input:
          type: string
```

## Plugin Directory Structure

```
~/.tlaude-code/plugins/
├── my-tools/
│   ├── plugin.yaml      # Manifest
│   ├── main.lua          # Entrypoint script
│   └── lib/              # Supporting Lua modules
│       └── helpers.lua
├── web-search/
│   ├── plugin.yaml
│   └── server.py         # MCP server (type: mcp)
├── disabled/
│   └── old-plugin/       # Disabled by rename
└── system/               # Built-in Lua libs
    ├── tool_api.lua      # Tool execution API for Lua
    └── hook_api.lua      # Hook registration API
```

## Lua Sandbox API

Plugins get a controlled set of Go functions exposed to Lua:

```lua
-- Tool plugin
tools.register({
    name = "my_transform",
    description = "Transform input data",
    handler = function(args)
        local result = string.upper(args.input)
        return {content = result}
    end
})

-- Hook plugin
hooks.on_tool_before(function(tool_name, args)
    if tool_name == "bash" and args.command:match("rm -rf") then
        return {deny = true, reason = "rm -rf not allowed by plugin policy"}
    end
    return {allow = true}
end)

-- Access to Go functions
local json = require("json")
local http = require("http")  -- Limited HTTP client
local fs   = require("fs")    -- Limited to plugin directory
```

## Security Model

| Aspect | Lua Plugin | MCP Plugin |
|--------|------------|------------|
| File access | Plugin directory only | Full based on OS permissions |
| Network | Allowed (timeout-limited) | Full based on subprocess |
| Execution | gopher-lua sandbox | OS-level process |
| Permission | Inherits host SafetyMode | Inherits host SafetyMode |
| Timeout | Configurable (default 30s) | Configurable (default 120s) |

## Implementation Plan

### Phase 1: Plugin Core (`internal/plugin/`)
1. **Manifest** — parser for `plugin.yaml` (Marshal/Unmarshal)
2. **Loader** — discover plugins in `~/.tlaude-code/plugins/`, validate manifests
3. **Manager** — lifecycle (enable/disable/load/unload/reload)
4. **Registry** — stores loaded plugins, provides lookup by name/type

### Phase 2: Lua Engine (`internal/plugin/lua/`)
1. **VM Pool** — gopher-lua state pool for concurrent use
2. **Sandbox** — secure Go API exposed to Lua
3. **Tool Bridge** — Lua-registered tools → tool.Tool interface
4. **Hook Bridge** — Lua hooks → internal hook system

### Phase 3: MCP Integration (`internal/plugin/mcp/`)
1. **Auto-registration** — MCP servers from plugin manifests
2. **Lifecycle syncing** — start/stop MCP servers with plugins

### Phase 4: Integration
1. **Config** — `plugins.enabled`, `plugins.disabled`, `plugins.dir`
2. **TUI** — `/plugins` command (list, enable, disable, reload)
3. **Startup** — auto-load enabled plugins on app start

## Files to Create/Modify

### New Files
```
internal/plugin/
├── manifest.go        — Plugin manifest types + parser
├── loader.go          — Plugin discovery + validation
├── manager.go         — Lifecycle management
├── registry.go        — Plugin registry
├── types.go           — Plugin interface + types
├── hook.go            — Hook system types + dispatch
├── lua/
│   ├── engine.go      — gopher-lua sandboxed VM
│   ├── tool_bridge.go — Lua tool → tool.Tool adapter
│   └── hook_bridge.go — Lua hook → Go hook adapter
└── plugin_test.go     — Tests
```

### Modified Files
```
internal/config/config.go       — Add plugin config section
internal/tool/registry.go       — Optional: explicit plugin tool registration
cmd/tlaude-code/main.go         — Initialize plugin manager
internal/tui/tui.go             — Add /plugins command
```

## Rejected Alternatives

### Go `plugin` package (buildmode=plugin)
- macOS: `.so` loading with CGo is fragile on Apple Silicon
- Go version lock: plugin must be built with exact same Go toolchain
- No hot-reload: once loaded, can't unload or update
- Distribution complexity: per-platform `.so` files
- **Verdict**: Not suitable for a distributed CLI tool

### WASM plugins (wazero)
- We already have a WASM sandbox stub, but:
  - WASM introduces significant complexity (host function imports, memory management)
  - WASM is overkill for most plugin use cases (custom tools, hooks)
  - Lua is simpler, lighter, and sufficient
- WASM could be added as a third tier later if needed
- **Verdict**: Defer — start with Lua + MCP

### Pure subprocess plugins (JSON-RPC over stdio)
- This is essentially what MCP already provides
- Adding another custom subprocess protocol would be redundant
- **Verdict**: Use MCP for subprocess — it's the industry standard
