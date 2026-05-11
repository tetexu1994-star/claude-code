# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.2.0] - 2026-05-11

### Added
- **CC-Source Agent System Prompts** — All 5 built-in agents now use CC-source-accurate system prompts
  - GeneralPurpose: task-oriented with research strengths (ported from agent_generalPurpose.ts)
  - Explore: READ-ONLY search specialist with parallel search guidance (ported from agent_explore.ts)
  - Plan: Software architect with 4-step planning process + critical files output (ported from agent_planAgent.ts)
  - Verification: NEW adversarial testing agent with strategy matrix, anti-rationalization checks, strict PASS/FAIL/PARTIAL output format (ported from agent_verification.ts, 152 lines)
  - Guide: Tlaude Code usage reference agent
- **Agent Definition Loading System** — Ported CC's loadAgentsDir.ts (755 lines) + agentMemory.ts (177 lines)
  - Markdown frontmatter parser: define agents via agents/*.md files
  - JSON agent parser: define agents programmatically
  - Directory scanner + priority-based merge + memoized cache
  - MemDir integration: scope-specific persistent memory (user/project/local)
- **Fork Subagent System** — Enhanced with full CC-source-accurate fork logic
  - BuildForkedMessages: prompt cache sharing via placeholder tool_results
  - IsInForkChild: recursive fork detection
  - BuildChildMessage: 10 strict fork rules + structured output format
  - BuildWorktreeNotice: isolated git worktree path translation
- **Coordinator Mode** — New internal/coordinator/ package (ported from coordinatorMode.ts, 369 lines)
  - Feature gate via TLAUDE_CODE_COORDINATOR_MODE environment variable
  - Session mode matching for resume compatibility
  - Worker tool configuration (simple/full mode)
  - Full coordinator system prompt: Research → Synthesis → Implementation → Verification workflow
  - Continue vs Spawn decision matrix
  - Worker prompt quality guidelines ("always synthesize — your most important job")
- **TUI Enhancement** — Welcome screen, coordinator panel, markdown rendering
  - Welcome screen: version banner + 8 quick-start tips, centered layout
  - Coordinator panel: sub-agent status list with color-coded states
  - Markdown rendering: headers (h1/h2/h3), code blocks, inline code, bold, lists
  - Enhanced status bar: model:provider | mode | tokens | cost display
  - Spinner: "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏" frame animation during streaming
- **Memory/MemDir Persistence** — Full CC-source port (6 files)
  - 4-type memory taxonomy: session, conversation, project, global
  - 3-scope agent memory: user (~/.tlaude-code/agent-memory/), project, local
  - File-based knowledge base with MEMORY.md index
  - Auto-compaction with threshold triggers
- **Swarm/Teams System** — Multi-agent team orchestration (5 files)
  - InProcessBackend: teammates share AgentRuntime
  - Mailbox-based message passing (Send/Receive with timeout)
  - Permission-aware tool delegation
  - Bridge pattern avoids circular imports
- **LSP Integration** — JSON-RPC LSP client (2 files)
  - Transport interface (stdio, extensible)
  - Client: request/response routing + async response handling
  - ServerManager: process lifecycle (spawn, health-check, shutdown)
  - Used for IDE integration (future)

### Changed
- CC source coverage: 46/100 → 72/100 files ported
- README.md updated with new feature documentation
- Example agent definitions added (agents/researcher.md, agents/tester.md)
- All system prompts reference Tlaude Code (not Claude Code)

### Infrastructure
- 26 Go packages, all -race clean (go test -race ./...)
- 1,625+ lines of new code across this release cycle

## [2.0.0] - 2026-05-10

### Added
- **Phase 3 Step 1: Plan Mode**
  - Plan data structures: Plan, PlanStep, ToolCallSpec, PlanStatus, StepStatus, PlanProgress
  - PlanStore with JSON file persistence: Create, Get, List, Update, Delete, Save, Load, LoadAll
  - Manager: BuildFromDescription (parse numbered/bullet steps), BuildFromMessages (extract from LLM output), Submit, Approve, Reject, ExecuteStep, ExecuteAll, GetProgress, IsInPlanMode, FormatPlan
  - Plan lifecycle: draft → pending_approval → approved → executing → completed (with rejection path)
  - Progress reporting: per-step tracking with visual progress bars and percentage calculation
  - LLM integration: EnterPlanMode/ExitPlanMode tool definitions (internal/tool/)
  - TUI integration: `/plan create/show/approve/reject/execute` slash commands
  - Plan directory: `~/.tlaude-code/plans/<id>.json`

- **Phase 3 Step 2: Distribution**
  - GoReleaser configuration (`.goreleaser.yaml`) for multi-platform builds (darwin/linux, amd64/arm64)
  - Homebrew formula (`homebrew/tlaude-code.rb`) via GoReleaser
  - GitHub Actions release workflow (`.github/workflows/release.yml`) triggered by tags
  - `--version` flag with build-time version injection via ldflags
  - Makefile enhancements: `build`, `install`, `release`, `clean` targets
  - Shell completion scripts (bash/zsh)

- **Phase 3 Step 3: Plugin System Core**
  - Plugin interface: Name, Version, Description, Type, Provides, Enabled, Load, Unload
  - Plugin types: Lua (embedded gopher-lua scripts), MCP (subprocess servers), Hybrid (both)
  - Extension points: tool, provider, sandbox, hook, agent
  - Manifest system: YAML-based `plugin.yaml` with Validate()
  - Registry: CRUD operations with lookup by name or by provided extension point
  - Loader: directory-scanning discovery (`~/.tlaude-code/plugins/<name>/`), manifest parsing, plugin instantiation
  - Manager: LoadAll/Unload/Reload with duplicate-skip and error-isolation
  - Hook system: event-based lifecycle hooks (ToolBefore, ToolAfter, SessionStart, etc.) with ordered dispatch and result aggregation (allow/deny/pass-through)

- **Phase 3 Step 4: Testing & Documentation**
  - Comprehensive plan package tests: 45+ test cases covering PlanStore (Create/Get/List/Update/Delete/SaveLoad/LoadAll/UniqueID/Concurrency), Manager (Build/Submit/Approve/Reject/Execute/Progress/FormatPlan/FormatProgress), helpers (GenerateSlug/BuildProgressBar)
  - Race-detector-clean concurrent tests for PlanStore
  - SPEC.md updated to v2.0.0: Plan Mode + Plugin System architecture, interfaces, design decisions
  - CHANGELOG.md updated with full Phase 3 history

### Changed
- Project target version upgraded from 1.0.0 to 2.0.0
- SPEC.md status updated from "Active Development" to "Stable"

## [1.0.0] - 2026-05-10

### Added
- **Phase 1: Tool System + Permission System**
  - Tool interface + Registry with permission-aware filtering and MCP tool pool assembly
  - Permission system: 6 modes (accepts/plan/edit/bypass_permissions/auto/chat), rule DSL parser, 5-step decision flow
  - StreamingToolExecutor: concurrent-safe tool execution with ordered result delivery
  - 14 tools: Bash, FileRead, FileWrite, FileEdit, Glob, Grep, WebFetch, WebSearch, TodoWrite, Agent, TaskCreate/Get/List/Stop
  - Permission rule DSL: `ToolName`, `ToolName(path)`, `Bash(prefix:git)`, `WebFetch(domain:*.com)`, `mcp__server`, `mcp__server__*`

- **Phase 2: Multi-Agent + MoA Fusion**
  - Agent definition system: YAML-based, 5 built-in agents (general/explore/code/review/moa)
  - Agent runtime: sync/async execution, LLM message loop, tool pool filtering, lifecycle management
  - Agent×Model matrix: each agent uses a different model/provider
  - MoA multi-model parallel: same task across multiple models, results aggregated
  - External agent backends: Claude Code CLI and Hermes Agent as subprocess agents
  - Fork sub-agent: context inheritance with API cache optimization
  - CLI flags: `--agent`, `--moa`, `--moa-strategy`, `--list-agents`
  - TUI commands: `/agent`, `/moa`, `/tasks`
  - Config: `agent:` section with `backends:`, `moa:`, `agents_dir:`

### Changed
- Project grew from 35 files / 6,900 lines to 75 files / 18,500+ lines
- All tool execution unified through tool.Registry and StreamingToolExecutor
- Deprecated `tools.GetToolDefinitions()` in favor of `tools.GetToolDefinitionsFromRegistry()`
- External backend paths made configurable in config.yaml

### Fixed
- TUI tool execution: hardcoded switch replaced with registry-based dynamic dispatch
- TUI tool definitions: switched from 4-tool subset to full registry
- External backends: properly registered in main.go initialization
- MoA implementations unified: agent/moa.go reuses moa/synthesizer.go
- Executor context: `New()` now accepts external context, cancellation propagates to tools
- TaskManager: `StartAgent` uses `WithCancel` instead of `Background`
- Model selection: unavailable providers are filtered out, graceful degradation
- `delete_file` definition removed (had no implementation)
- SafetyMode integration: `ShouldAutoApprove()` respects `ask`/`allow`/`reject` + `AlwaysAllowPatterns`
- `SPEC.md`: comprehensive architecture specification document
- CI/CD: GitHub Actions workflow (build, vet, test, race, lint)
- Makefile + .editorconfig + shell completion scripts
- Comprehensive test coverage for moa, sandbox, cost, memory, tools, tui, logging packages

## [0.3.0] - 2026-05-10

### Fixed
- Data race: copies `m.cfg` values to locals before `tea.Cmd` closures
- Dead code removed: `ListFactories()`, `computeDiff()`, `isWhitespaceOnly()`, unused logging config methods, unused custom error types
- `CLAUDE.md` updated to reflect current implementation status
- `context.Background()` replaced with proper `Model.ctx` lifecycle in TUI; MCP init uses `context.WithTimeout`
- `DetectEnvAPIKeys()` now called on first run (was only called when config file existed)
- FirstRunWizard detects pipe/redirect and skips interactive prompts
- OpenRouter API URL: fixed double `/v1` (`/api/v1/v1/...` → `/api/v1/...`)
- Provider-specific default models: DeepSeek→deepseek-chat, OpenRouter→openrouter/auto, etc.

### Security
- Approval flow now reads `SafetyMode` config instead of always prompting

## [0.2.0] - 2026-05-10

### Added
- MoA (Mixture of Agents): parallel multi-provider invocation with 4 modes
  - `fastest`: return first response
  - `consensus`: require all providers to agree
  - `majority`: return most common response
  - `synthesize`: merge responses via LLM
- Sandbox system: 3 modes (`off`/`restricted`/`wasm-stub`)
- MCP JSON-RPC client with stdio + SSE transports
- Cost tracking: per-provider token usage and cost estimation
- Smart routing: auto-route tasks to cheapest/strongest provider based on complexity
- Memory search: keyword search across historical sessions
- Session persistence: UUID v4 sessions, save/load/resume
- Slash commands: `/help`, `/config`, `/clear`, `/save`, `/moa`, `/sandbox`, `/mcp`, `/cost`, `/route`, `/search`, `/quit`

### Changed
- Approval flow redesigned: Y/N/D/A keyboard shortcuts + diff view
- CC Switch registration scripts added

## [0.1.0] - 2026-05-10

### Added
- Go project scaffold with module structure
- 7 LLM providers: Anthropic, DeepSeek, OpenAI, OpenRouter, SiliconFlow, Tongyi, Zhipu
- Provider registry with factory pattern, health probes, priority selection
- Bubble Tea TUI with chat view, streaming, code blocks, status bar
- Bash tool + Filesystem tool
- Structured logging (slog-based)
- YAML configuration with first-run wizard
- CLI flags: `--provider`, `--model`, `--temperature`, `--max-tokens`, `--version`, `--print`, `--resume`, `--session`
- Panic recovery and graceful signal handling (SIGINT/SIGTERM)
- Token estimation with CJK character support
- Print mode for non-interactive pipe usage
- README with quick start and configuration guide

[2.2.0]: https://github.com/tetexu1994-star/claude-code/compare/v2.0.0...v2.2.0
