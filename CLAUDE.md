# Tlaude Code — Project Context

> This file provides Tlaude Code CLI with project context for the class-tlaude-code project.

## Overview
A production-grade Tlaude Code alternative CLI tool built in Go, with multi-provider support, MoA parallel invocation, MCP integration, sandboxed execution, and cost-aware routing.

## Architecture
```
cmd/tlaude-code/main.go         — CLI entry point (Bubble Tea TUI, flags, signal handling)
internal/
  config/config.go              — YAML config, API key management, SafetyMode, wizard
  llm/
    provider.go                 — Provider interface + Factory pattern
    registry.go                 — Global registry, health probes, priority selection
    anthropic/provider.go       — Anthropic API (implemented)
    deepseek/provider.go        — DeepSeek API (implemented)
    openai/provider.go          — OpenAI API (implemented)
    openrouter/provider.go      — OpenRouter API (implemented)
    siliconflow/provider.go     — SiliconFlow API (implemented)
    tongyi/provider.go          — Tongyi/Qwen API (implemented)
    zhipu/provider.go           — Zhipu/GLM API (implemented)
  logging/errors.go             — Custom error types (ConfigError, ProviderError, etc.)
  logging/logging.go            — Structured logging (slog-based)
  mcp/client.go                 — MCP JSON-RPC client (stdio + SSE transports, Manager)
  moa/
    orchestrator.go              — MoA orchestrator (parallel multi-provider calls, 4 modes)
    synthesizer.go               — Synthesis prompt builder + majority/consensus checking
  sandbox/
    sandbox.go                   — Sandbox interface + factory (WASM, Restricted, Direct modes)
    restricted.go                — Restricted subprocess sandbox (isolated env, timeout, output limit)
    passthrough.go               — Direct execution mode (sandbox off)
    wasm.go                      — WASM sandbox stub (requires wazero dependency)
  session/session.go            — Session persistence (~/.tlaude-code/sessions/*.json)
  agent/
    store.go                    — Agent definition store (built-in + user)
    runtime.go                  — Agent runtime (orchestration, async execution)
    backend.go                  — External subprocess backends (Claude Code, Hermes)
  hook/
    types.go                    — Lifecycle hook types (before/after tool, provider)
  plan/
    types.go                    — Plan data structures (Plan, PlanStep, PlanStore with file persistence)
    manager.go                  — Plan lifecycle management (Build, Approve, Reject, Execute, Progress)
  plugin/
    types.go                    — Plugin interface (Lua, MCP, Hybrid)
    manifest.go                 — Plugin manifest (plugin.yaml parsing)
    manager.go                  — Plugin lifecycle (load, unload, reload, list)
    loader.go                   — Plugin discovery and loading
    registry.go                 — Thread-safe plugin registry
    hook.go                     — Plugin hook registry
    lua/
      engine.go                 — Lua engine (VM pool, compile, execute)
      api.go                    — Lua APIs (JSON, HTTP, FS, tools, hooks)
      tool_bridge.go            — Lua → Go tool bridge
      hook_bridge.go            — Lua → Go hook bridge
      pool.go                   — VM pool (get/put)
      sandbox.go                — Sandbox configuration
  sandbox/
    sandbox.go                   — Sandbox interface + factory (WASM, Restricted, Direct modes)
    restricted.go                — Restricted subprocess sandbox (isolated env, timeout, output limit)
    passthrough.go               — Direct execution mode (sandbox off)
    wasm.go                      — WASM sandbox stub (requires wazero dependency)
  session/session.go            — Session persistence (~/.tlaude-code/sessions/*.json)
  tool/
    default.go                  — Built-in tool definitions (16 tools)
    tool.go                     — Tool interface and types
    registry.go                 — Tool registry with permission-aware queries
    enterplan.go                — EnterPlanModeTool (LLM calls to enter plan mode)
    exitaplan.go                — ExitPlanModeTool (LLM calls to exit plan mode)
    agent_tool.go               — AgentTool (multi-agent orchestration)
    task_manager.go             — Background task management
  tools/
    bash/bash.go                — Bash execution tool
    filesystem/filesystem.go    — File system tool
  tui/tui.go                    — Bubble Tea TUI (chat, streaming, input, status bar, approval flow)
  tui/approval.go               — Approval request rendering (Y/N/D/A keyboard shortcuts)
  tui/diff.go                   — Diff view renderer (full-screen mode)
  cost/tracker.go               — Per-provider token usage and cost estimation
  cost/router.go                — Smart routing based on prompt complexity
  cost/rates.go                 — Provider pricing rates
  memory/searcher.go            — Keyword search across historical sessions
scripts/
  install.sh                    — Install to system PATH
  register-cc-switch.sh         — Register with CC Switch
```

## Build & Run
```bash
export PATH="$HOME/.local/go/bin:$PATH"
go build ./cmd/tlaude-code/      # Verify compilation
go vet ./...                     # Code quality
go test ./...                    # Run tests
go build -o tlaude-code ./cmd/tlaude-code/
./tlaude-code --help
```

## Development Rules
1. **All 7 LLM providers use `llm.RegisterFactory()`** — do NOT use `GlobalRegistry().Register()` directly
2. **Interfaces must match exactly** — `ChatStream` not `Stream`
3. **All providers follow OpenAI-compatible API pattern** (see deepseek/provider.go as template)
4. **TUI uses Bubble Tea** (github.com/charmbracelet/bubbletea)
5. **CLAUDE.md must be kept updated** as architecture changes
6. **Delta (strategist) handles docs/specs** — Tlaude Code handles code implementation only
7. **Build + vet + test must pass before any commit**

## Engineering Discipline

### TDD (Test-Driven Development)
- **Red-Green-Refactor**: Write failing test → minimal code to pass → refactor
- **Vertical slices**: ONE end-to-end behavior per cycle. Do NOT write all tests first (horizontal slicing).
- **Integration-style tests**: Test behavior, not implementation. Mock only at system boundaries.
- **Tracer bullet**: First test proves the wiring works end-to-end; subsequent tests add behaviors.

### Debugging Protocol
- **Feedback loop FIRST**: Never debug without a fast (<5s), deterministic pass/fail signal.
- **Hypothesize before instrumenting**: Generate 3-5 falsifiable hypotheses. Do NOT anchor on the first idea.
- **One variable at a time**: Change exactly one thing per run.
- **Write regression test BEFORE the fix**: At the correct seam, not at API level if the bug is internal.

### Architecture Principles
- **Deep modules**: Powerful functionality through simple interfaces. Consolidate shallow modules.
- **Deletion test**: "How much breaks if this package disappears?" — high deletion cost + small interface = deep (good).
- **Ports & Adapters**: Core domain logic depends only on interfaces; adapters implement them.
- **ADRs sparingly**: Create Architecture Decision Records only when ALL three hold: hard to reverse, surprising, result of trade-off.

### Feature Development Flow
1. **Understand** — Read relevant code, trace the data flow, understand existing patterns
2. **Prototype if uncertain** — Throwaway experiment first, delete after lesson learned
3. **Contract first** — Define the interface/function signature before implementation
4. **Implement** — One vertical slice at a time
5. **Verify** — `go build ./...` → `go vet ./...` → `go test -count=1 -race ./...` → `go test ./...`

### Change Management
- One change per commit. Small, frequent, focused.
- Never leave debug instrumentation behind.
- After any fix, check: does this pattern exist elsewhere? File a follow-up if so.

## Current Status
- ✅ 7 LLM providers (all implemented with ToolCall output support)
- ✅ Provider registry with factory pattern, health probes, priority selection, auto-failover
- ✅ Config (YAML) + CLI flags (--provider, --model, --temperature, --max-tokens, --version, --print, --resume, --session)
- ✅ Bubble Tea TUI (chat rendering, streaming, code blocks, status bar, approval, diff view)
- ✅ Session persistence (JSON files, Save/Load/List/Latest/Delete, auto-save on shutdown)
- ✅ MoA (Mixture of Agents): 4 modes (fastest, consensus, majority, synthesize)
- ✅ Sandbox: restricted + passthrough modes (WASM stub)
- ✅ MCP JSON-RPC client with Manager (stdio + SSE)
- ✅ Cost tracking + smart routing (complexity-based provider selection)
- ✅ Memory search (keyword-based across session files)
- ✅ Plan Mode (EnterPlanMode/ExitPlanMode tools, PlanStore, TUI integration, /plan commands)
- ✅ SafetyMode (ask/allow/reject + AlwaysAllowPatterns)
- ✅ Signal handling (SIGINT/SIGTERM graceful shutdown)
- ✅ Panic recovery in main()
- ✅ Approved data race fixes (cfg copy before closures)
- ✅ Plugin system (Lua scripting + MCP subprocess, discovery, registry, hooks)
- ✅ Agent system (multi-agent orchestration, external backends, async execution)
- ✅ README + SPEC.md + CHANGELOG.md + PLAN.md

## CLI Flags
```
--provider string     LLM provider (overrides config)
--model string        Model name (overrides config)
--temperature float   Temperature (overrides config)
--max-tokens int      Max tokens (overrides config)
--version             Print version and exit
--print               Print a single response to stdout (non-interactive pipe mode)
--resume              Resume the most recent session
--session string      Resume a specific session by ID
```

## TUI Slash Commands
- `/help` — show help overlay
- `/config` — show current configuration
- `/clear` — clear chat history
- `/save` — save session to disk
- `/cost` — show cost report (estimated)
- `/route` — show routing configuration
- `/route smart` — enable cost-aware routing
- `/route fixed` — disable smart routing
- `/search <q>` — search historical sessions
- `/moa` — show MoA status and last result
- `/moa on` — enable MoA (requires config)
- `/moa off` — disable MoA
- `/moa strategy <mode>` — set MoA mode (fastest/consensus/majority/synthesize)
- `/mcp` — show MCP server status
- `/sandbox` — show sandbox mode and config
- `/sandbox restricted` — switch to restricted sandbox mode
- `/sandbox off` — disable sandbox (direct execution)
- `/agent` — show agent configuration
- `/agent list` — list all available agents
- `/agent <type>` — switch agent type
- `/tasks` — show background tasks
- `/plan` — show current plan status and steps
- `/plan create <desc>` — create a new plan
- `/plan approve` — approve the current plan
- `/plan reject [reason]` — reject the current plan
- `/plan execute` — execute an approved plan
- `/plugins` — list loaded plugins
- `/quit` — exit

## TUI Keyboard Shortcuts
- Enter — send message / slash command
- Ctrl+C / Esc — quit
- Ctrl+H — show help overlay
- Y / N / D / A — approval flow (Yes / No / Diff / Always)

## Go Version
go 1.23.6 (installed at ~/.local/go/)
