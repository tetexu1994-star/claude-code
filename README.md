# Tlaude Code (Go)

A production-grade Tlaude Code alternative CLI tool built in Go, with multi-provider support, MoA parallel invocation, MCP integration, and sandboxed execution.

## Features

- **7 LLM Providers**: DeepSeek, Anthropic, OpenAI, OpenRouter, SiliconFlow, Tongyi, Zhipu
- **Bubble Tea TUI**: Chat interface with streaming responses, code blocks, diff view
- **MoA (Mixture of Agents)**: Parallel multi-provider invocation with smart synthesis
- **MCP Integration**: Connect to any MCP-compatible server (stdio or SSE)
- **Sandbox**: Restricted execution environment for safe command running
- **Cost Tracking**: Per-provider token usage and cost estimation
- **Smart Routing**: Auto-route tasks to cheapest or strongest provider
- **Session Persistence**: Save and resume conversations
- **Approval Flow**: User confirmation before tool execution
- **Memory Search**: Keyword search across historical sessions
- **Plan Mode**: "Plan first, then execute" workflow (create → approve → execute)
- **Agent System**: Multi-agent orchestration with configurable backends (Claude Code, Hermes)
- **Plugin System**: Lua scripting and MCP subprocess plugins for extensibility
- **Lua Scripting Engine**: Embedded gopher-lua, sandboxed execution, tool and hook registration
- **Distribution**: Homebrew installable, GoReleaser CI/CD, cross-platform binary
- **CC Switch Compatible**: Register with CC Switch for easy switching

## Installation

```bash
# Homebrew (recommended)
brew install tetexu/tlaude-code/tlaude-code

# Go install
go install github.com/tetexu/tlaude-code/cmd/tlaude-code@latest

# Direct download
# Download the appropriate tar.gz from GitHub Releases:
# https://github.com/tetexu/tlaude-code/releases
```

## Quick Start

```bash
# Build from source
make build

# Or build all platforms
make build-all

# First run (creates ~/.tlaude-code/config.yaml)
./tlaude-code

# Pipe mode
echo "Hello" | ./tlaude-code --print

# TUI mode
./tlaude-code

# Resume last session
./tlaude-code --resume
```

### Quick Configuration

After first run, edit `~/.tlaude-code/config.yaml` to set your preferred provider and API key:

```yaml
provider: deepseek
model: deepseek-chat
api_keys:
  deepseek: sk-your-api-key
```

Or use environment variables:
```bash
export DEEPSEEK_API_KEY=sk-your-api-key
export ANTHROPIC_API_KEY=sk-ant-your-key
# ... etc.
```

The following directories are created automatically on first run:
- `~/.tlaude-code/` — configuration and data
- `~/.tlaude-code/sessions/` — conversation history
- `~/.tlaude-code/agents/` — custom agent definitions
- `~/.tlaude-code/plans/` — plan files
- `~/.tlaude-code/plugins/` — Lua and MCP plugins

## Configuration

Config file: `~/.tlaude-code/config.yaml`

```yaml
provider: deepseek
model: deepseek-chat
temperature: 0.7
max_tokens: 4096

moa:
  enabled: false
  mode: synthesize
  max_parallel: 3
  timeout_sec: 30

sandbox:
  mode: restricted
  timeout_sec: 30
  max_memory_mb: 128

safety_mode: ask
enable_cost_tracking: true
smart_routing: false
enable_memory_search: true
```

## Slash Commands

- `/help` — Show help
- `/config` — Show current configuration
- `/clear` — Clear chat history
- `/save` — Save session to disk
- `/cost` — Show cost report (estimated)
- `/route` — Show routing configuration
- `/route smart` — Enable smart (cost-aware) routing
- `/route fixed` — Disable smart routing
- `/search <query>` — Search historical sessions
- `/moa` — Show MoA status and last result
- `/moa on` — Enable MoA (requires config)
- `/moa off` — Disable MoA
- `/moa strategy <mode>` — Set MoA mode (fastest/consensus/majority/synthesize)
- `/mcp` — Show MCP server status
- `/sandbox` — Show sandbox mode
- `/sandbox restricted|off` — Switch sandbox mode
- `/agent` — Show agent configuration
- `/agent list` — List all available agents
- `/agent <type>` — Switch agent type
- `/tasks` — Show background tasks
- `/plan` — Show current plan status
- `/plan create <desc>` — Create a new plan
- `/plan approve` — Approve current plan
- `/plan reject [reason]` — Reject current plan
- `/plan execute` — Execute approved plan
- `/plugins` — List loaded plugins
- `/quit` — Exit

## Architecture

```
cmd/tlaude-code/main.go    — CLI entry point
internal/
  agent/                   — Multi-agent system
  config/                  — YAML config
  cost/                    — Cost tracking & routing
  hook/                    — Lifecycle hook types
  llm/                     — Provider interface + 7 implementations
  logging/                 — Structured logging
  mcp/                     — MCP JSON-RPC client
  memory/                  — Session memory search
  moa/                     — Multi-provider orchestrator
  plan/                    — Plan Mode (create → approve → execute)
  plugin/                  — Plugin system (manifest, loader, registry)
  plugin/lua/              — Lua scripting engine (gopher-lua sandbox)
  sandbox/                 — Restricted + WASM sandbox
  session/                 — Session persistence
  tool/                    — Tool system (registry, permissions, executor)
  tools/                   — Built-in tool implementations
  tui/                     — Bubble Tea TUI
scripts/
  install.sh               — Install to system PATH
  register-cc-switch.sh    — Register with CC Switch
```

## Development

```bash
# Build
make build

# Test
make test

# Vet
make vet

# Cross-compile
make build-all

# Local snapshot build (requires goreleaser)
make dist
```

## Requirements

- Go 1.23+
- macOS/Linux (Windows untested)
- At least one LLM provider API key
