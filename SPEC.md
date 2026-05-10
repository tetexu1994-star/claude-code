# Tlaude Code — Architecture Specification

> Version: 2.0.0 · Status: Stable  
> Repository: [tetexu1994-star/tlaude-code](https://github.com/tetexu1994-star/tlaude-code)

---

## 1. Overview

### 1.1 Purpose

Tlaude Code is a production-grade CLI alternative to Anthropic's Tlaude Code, built in Go. It provides a rich terminal UI for interacting with multiple LLM providers simultaneously, executing tools, and managing sessions — all with sandboxed execution, cost awareness, plan-mode workflow, and an extensible plugin system.

### 1.2 Design Goals

| Goal | Priority | Rationale |
|------|----------|-----------|
| Multi-provider support | P0 | Avoid vendor lock-in; users bring their own API keys |
| Rich TUI experience | P0 | Parity with Tlaude Code; streaming, code blocks, approval flow |
| Sandboxed execution | P0 | Safe command execution without system exposure |
| Plan Mode workflow | P0 | "Plan first, then execute" pattern; draft → approve → execute lifecycle |
| Plugin extensibility | P0 | Lua/MCP/Hybrid plugins for tools, providers, sandboxes, hooks |
| Cost-aware routing | P1 | Optimize provider selection based on task complexity |
| MCP integration | P1 | Extend tool capabilities via MCP-compatible servers |
| Session persistence | P1 | Resume conversations across invocations |
| MoA (Mixture of Agents) | P2 | Parallel multi-provider invocation with synthesis |

### 1.3 Target Users

- Developers who want a Tlaude Code-like experience with their own API keys
- Teams that need cost-optimized multi-provider access
- Users who require sandboxed tool execution for safety

### 1.4 System Requirements

- **OS**: macOS (primary), Linux (compatible)
- **Runtime**: Go 1.23+
- **Storage**: ~50MB binary; session data at `~/.tlaude-code/`

---

## 2. Architecture

### 2.1 High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    cmd/tlaude-code/main.go                    │
│  ┌─────────┐ ┌──────────┐ ┌────────┐ ┌─────┐ ┌──────────┐  │
│  │  Flags  │ │  Config  │ │Session │ │ MCP │ │  Signal   │  │
│  │  Parse  │ │  Wizard  │ │Manager │ │Init │ │  Handler  │  │
│  └────┬────┘ └────┬─────┘ └────┬───┘ └──┬──┘ └─────┬────┘  │
│       └───────────┴────────────┴────────┴────────────┘       │
│                            │                                 │
│                     ┌──────▼──────┐                          │
│                     │  tea.NewProgram(&model)                │
│                     └──────┬──────┘                          │
└────────────────────────────┼─────────────────────────────────┘
                             │
┌────────────────────────────▼─────────────────────────────────┐
│                     internal/tui/tui.go                       │
│  ┌──────────────────────────────────────────────────────┐    │
│  │  Model (Bubble Tea)                                   │    │
│  │  ┌────────┐ ┌──────────┐ ┌─────────┐ ┌──────────┐    │    │
│  │  │  Chat  │ │  Input   │ │ Status  │ │  Help /  │    │    │
│  │  │ View   │ │  Area    │ │  Bar    │ │  Diff    │    │    │
│  │  └────────┘ └──────────┘ └─────────┘ └──────────┘    │    │
│  │                                                      │    │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────────────┐     │    │
│  │  │Approval  │ │  MoA     │ │  Slash Commands  │     │    │
│  │  │Flow      │ │  Display │ │  Handler         │     │    │
│  │  └──────────┘ └──────────┘ └──────────────────┘     │    │
│  └──────────────────────────────────────────────────────┘    │
└────────────────────────────┬─────────────────────────────────┘
                             │
        ┌────────────────────┼────────────────────┐
        ▼                    ▼                    ▼
┌───────────────┐  ┌────────────────┐  ┌────────────────┐
│  llm.Provider │  │ sandbox.Sand-  │  │  tools.Bash /  │
│  (7 implem-   │  │ boxer (3 modes)│  │  Filesystem    │
│  entations)   │  └────────────────┘  └────────────────┘
└───────┬───────┘
        │
┌───────▼─────────────────────────────────────────────┐
│  Cross-Cutting Packages                              │
│  ┌──────────┐ ┌──────────┐ ┌────────┐ ┌─────────┐   │
│  │  cost/   │ │ memory/  │ │  mcp/  │ │ moa/    │   │
│  │  tracker │ │ searcher │ │ client │ │ orches- │   │
│  │  +router │ │          │ │+manager│ │ trator  │   │
│  └──────────┘ └──────────┘ └────────┘ └─────────┘   │
│  ┌──────────┐ ┌──────────┐ ┌──────────────────┐     │
│  │  config/ │ │ session/ │ │  logging/        │     │
│  │  YAML    │ │  JSON    │ │  structured slog │     │
│  │  wizard  │ │  store   │ │  + error types   │     │
│  └──────────┘ └──────────┘ └──────────────────┘     │
│  ┌──────────┐ ┌────────────────────────────────┐    │
│  │  plan/   │ │  plugin/                        │    │
│  │  Plan    │ │  Lua · MCP · Hybrid             │    │
│  │  Manager │ │  Registry · Loader · Hooks      │    │
│  └──────────┘ └────────────────────────────────┘    │
└─────────────────────────────────────────────────────┘
```

### 2.2 Module Responsibility Matrix

| Package | Path | Responsibility | Dependencies |
|---------|------|----------------|-------------|
| **main** | `cmd/tlaude-code/` | CLI entry: flags, config load, provider registration, signal handling, TUI launch | All internal packages |
| **tui** | `internal/tui/` | Bubble Tea TUI: chat view, input, status bar, approval flow, diff view, MoA display, slash commands | config, llm, sandbox, tools, moa, mcp, cost, session, memory |
| **llm** | `internal/llm/` | Provider interface + factory registry + global registry with health probes | config, logging |
| **config** | `internal/config/` | YAML config: load/save, API key management, first-run wizard, SafetyMode, AlwaysAllowPatterns | — |
| **sandbox** | `internal/sandbox/` | Sandbox interface + 3 modes (Restricted, Passthrough, WASM stub) | config |
| **tools** | `internal/tools/` | Tool definitions for LLM tool calling + bash/filesystem implementations | llm |
| **moa** | `internal/moa/` | Multi-provider orchestrator: 4 modes (fastest/consensus/majority/synthesize) | llm |
| **mcp** | `internal/mcp/` | JSON-RPC client: stdio + SSE transports, Manager for multi-server | logging |
| **session** | `internal/session/` | UUID v4 session + JSON persistence: save/load/list/latest/delete | llm |
| **cost** | `internal/cost/` | Cost tracker (per-provider) + smart router (task complexity → provider) | config |
| **memory** | `internal/memory/` | Keyword search across historical session files | — |
| **plan** | `internal/plan/` | Plan Mode: plan lifecycle (draft → approve → execute), step tracking, progress reporting, file persistence | llm |
| **plugin** | `internal/plugin/` | Plugin system: Lua/MCP/Hybrid plugins, manifest parsing, registry, loader, manager, lifecycle hooks | config, mcp, tool |
| **logging** | `internal/logging/` | Structured logging (slog-based) + custom error types | — |

### 2.3 Data Flow

#### 2.3.1 Standard Chat Flow

```
User Input → [sendMessage] → [Provider.ChatStream] → chunks → [streamChunkMsg]
                                                                   │
                         ┌─────────────────────────────────────────┘
                         ▼
                   Tool Calls? ──No──→ Display response
                         │
                        Yes
                         ▼
              [handleToolCalls]
                ├── Auto-approved → [executeToolCall] → tool result
                └── Needs approval → [Approval Request] → Y/N/D/A
                                                                  │
                          Tool result → [sendToolResultsToModel]
                                          │
                                    [Provider.ChatStream]
                                          │
                                    More tool calls or final response
```

#### 2.3.2 MoA Flow

```
User Input → [sendMessage]
                  │
           MoA enabled?
           ├── No → Standard streaming (single provider)
           └── Yes → [moaOrchestrator.Execute]
                        │
                  Parallel calls to N providers
                        │
                  ┌─────┼─────┐
                  ▼     ▼     ▼
              [Provider1] [Provider2] [Provider3]
                  │     │     │
                  └─────┼─────┘
                        ▼
              4 Modes:
              ├── Fastest → pick first response
              ├── Majority → pick most common response
              ├── Consensus → check for agreement
              └── Synthesize → merge responses via LLM
                        │
                  [moaResultMsg] → Display
```

#### 2.3.3 Smart Routing Flow

```
User Input → [sendMessage]
                  │
           Smart Routing enabled?
           ├── No → Use configured provider
           └── Yes → [cost.Router.Select]
                        │
                  [cost.ClassifyPromptText]
                  ├── Simple → fast/cheap provider
                  ├── Medium → balanced provider
                  └── Complex → powerful provider
                        │
                  Switch provider if needed
                  │
                  ┌── Display route info to user
                  └── Proceed with selected provider
```

---

## 3. Interface Definitions

### 3.1 Provider Interface

```go
type Provider interface {
    Name() string
    IsAvailable() bool
    Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
    ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error)
    Models() ([]string, error)
}

type ProviderFactory func(cfg ProviderConfig) (Provider, error)
```

**Contract:**
- `Chat` is a blocking call returning a complete response
- `ChatStream` returns a channel consumed by the TUI's streaming engine
- `IsAvailable` is a lightweight health check (no network call)
- Factory functions must be registered via `RegisterFactory()` before use

### 3.2 Sandbox Interface

```go
type Sandboxer interface {
    Name() string
    ExecuteScript(ctx context.Context, interpreter, script string) (*Result, error)
    Close() error
}

type Result struct {
    Stdout   string
    Stderr   string
    ExitCode int
}
```

**Modes:**
| Mode | Execution | Network | Write | When to Use |
|------|-----------|---------|-------|-------------|
| `off` (Passthrough) | Direct `os/exec` | Full | Full | Development, trusted commands |
| `restricted` | Subprocess with env isolation | Configurable | Configurable | Production, untrusted input |
| `wasm` | WASM runtime (stub) | — | — | Future: maximum isolation |

### 3.3 MoA Interface

```go
type Orchestrator struct { /* ... */ }
func NewOrchestrator(reg *Registry, cfg MoAConfig) *Orchestrator
func (o *Orchestrator) Execute(ctx context.Context, req ChatRequest) (*MoAResult, error)
```

**Modes:**
| Mode | Behavior | Latency | Quality |
|------|----------|---------|---------|
| `fastest` | Return first available response | Fastest | Single provider |
| `consensus` | Return only if all agree | Slowest | Highest confidence |
| `majority` | Return most common response | Moderate | High confidence |
| `synthesize` | Merge all responses via LLM | Slowest | Highest quality |

### 3.4 TUI Model Interface

```go
type Model struct { /* Bubble Tea model — 20+ fields */ }
func NewModel(...) Model
func (m *Model) Init() tea.Cmd
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)
func (m *Model) View() string
func (m *Model) SetSession(sess *session.Session)
func (m *Model) SaveSession()
func (m *Model) Quitting() bool
```

### 3.5 MCP Client Interface

```go
type Client struct { /* JSON-RPC 2.0 */ }
func NewClient(transport Transport) *Client
func (c *Client) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error)

type Manager struct { /* Multi-server management */ }
func NewManager() *Manager
func (m *Manager) Add(ctx context.Context, name string, client *Client) error
```

### 3.6 Plan Mode Interface

```go
type Plan struct {
    ID          string
    Title       string
    Description string
    Steps       []PlanStep
    Status      PlanStatus
    CreatedAt   time.Time
    ApprovedAt  *time.Time
}

type PlanStore struct { /* JSON-file-backed persistence */ }
func NewPlanStore() *PlanStore
func (s *PlanStore) Create(title, description string) *Plan
func (s *PlanStore) Get(id string) (*Plan, bool)
func (s *PlanStore) List() []*Plan
func (s *PlanStore) Update(plan *Plan)
func (s *PlanStore) Delete(id string)
func (s *PlanStore) Save(plan *Plan) error
func (s *PlanStore) Load(id string) (*Plan, error)
func (s *PlanStore) LoadAll() error

type Manager struct { /* Plan lifecycle management */ }
func NewManager(store *PlanStore) *Manager
func (m *Manager) BuildFromDescription(title, description string) *Plan
func (m *Manager) BuildFromMessages(messages []llm.Message) *Plan
func (m *Manager) Submit(planID string) error
func (m *Manager) Approve(planID string) error
func (m *Manager) Reject(planID string, reason string) error
func (m *Manager) ExecuteStep(ctx context.Context, planID string, stepIndex int, exec func(...) error) error
func (m *Manager) ExecuteAll(ctx context.Context, planID string, exec func(...) error) error
func (m *Manager) GetProgress(planID string) (PlanProgress, error)
func (m *Manager) IsInPlanMode() bool
```

**Plan Status Lifecycle:**
```
draft → pending_approval → approved → executing → completed
  │         │                               │
  └─────────┴──────────→ rejected ←─────────┘
```

### 3.7 Plugin Interface

```go
type Plugin interface {
    Name() string
    Version() string
    Description() string
    Type() Type              // lua | mcp | hybrid
    Provides() []Provides    // tool | provider | sandbox | hook | agent
    Enabled() bool
    SetEnabled(bool)
    Load(ctx context.Context) error
    Unload(ctx context.Context) error
}

type Manifest struct {
    Name        string
    Version     string
    Description string
    Author      string
    Type        Type
    Entrypoint  string    // Lua script path (lua/hybrid only)
    MCP         *MCPConfig // MCP command config (mcp/hybrid only)
    Provides    []Provides
    Config      map[string]interface{}
    Tools       []ToolDef
    Hooks       []HookDef
}
```

**Plugin Types:**
| Type | Backend | Entrypoint | Runtime |
|------|---------|------------|---------|
| `lua` | Embedded gopher-lua | `.lua` script | In-process Lua VM |
| `mcp` | MCP subprocess | MCP command | Child process via stdio/SSE |
| `hybrid` | Lua + MCP | `.lua` script + MCP command | Both in-process and subprocess |

---

## 4. Configuration Reference

### 4.1 Config File (`~/.tlaude-code/config.yaml`)

```yaml
# Primary provider selection
provider: deepseek
model: deepseek-chat
temperature: 0.7
max_tokens: 4096

# MoA (Mixture of Agents) — disabled by default
moa:
  enabled: false
  mode: synthesize              # fastest | consensus | majority | synthesize
  max_parallel: 3
  timeout_sec: 30
  synthesizer: auto             # auto = use the strongest provider
  provider_names: []            # empty = use all registered providers

# Sandbox configuration
sandbox:
  mode: restricted              # off | restricted | wasm
  timeout_sec: 30
  max_memory_mb: 128
  allow_network: true
  allow_write: true
  allowed_paths: []             # empty = current dir only

# Safety settings
safety_mode: ask                # ask | allow | reject
always_allow_patterns:          # auto-approved command/file patterns
  - "bash:ls"
  - "bash:cat"
  - "bash:pwd"
  - "bash:echo"

# Feature toggles
enable_cost_tracking: true
smart_routing: false
enable_memory_search: true

# MCP servers
mcp_servers:
  - name: my-server
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "."]
    enabled: false
```

### 4.2 Environment Variables

| Variable | Provider | Example |
|----------|----------|---------|
| `ANTHROPIC_API_KEY` | Anthropic | `sk-ant-...` |
| `OPENAI_API_KEY` | OpenAI | `sk-...` |
| `DEEPSEEK_API_KEY` | DeepSeek | `sk-...` |
| `OPENROUTER_API_KEY` | OpenRouter | `sk-or-...` |
| `SILICONFLOW_API_KEY` | SiliconFlow | `sk-...` |
| `DASHSCOPE_API_KEY` | Tongyi (Qwen) | `sk-...` |
| `ZHIPUAI_API_KEY` | Zhipu (GLM) | `sk-...` |

---

## 5. CLI Reference

### 5.1 Usage

```
tlaude-code [flags] [prompt...]
```

### 5.2 Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--provider` | string | (config) | Override LLM provider |
| `--model` | string | (config) | Override model name |
| `--temperature` | float | (config) | Override temperature |
| `--max-tokens` | int | (config) | Override max tokens |
| `--version` | bool | false | Print version and exit |
| `--print` | bool | false | Non-interactive pipe mode |
| `--resume` | bool | false | Resume most recent session |
| `--session` | string | "" | Resume specific session by ID |

### 5.3 Exit Codes

| Code | Meaning |
|:----:|---------|
| 0 | Success |
| 1 | General error (config, provider, runtime) |

---

## 6. Design Decisions

### 6.1 Why Go?

- Static typing catches interface mismatches at compile time
- Single binary deployment (no runtime dependency)
- Excellent cross-platform support (macOS → Linux)
- Bubble Tea ecosystem for TUIs
- Strong standard library (no framework lock-in)

### 6.2 Why Bubble Tea (not Bubble Gum / ncurses / termbox)?

- Elm architecture (Model → Update → View) is clean and testable
- Active maintenance by Charmbracelet
- Rich ecosystem of widgets (textarea, viewport, table, spinner)
- Cross-platform terminal support

### 6.3 Multi-Provider Architecture

- **Factory pattern**: each provider registers a factory function; the registry creates instances
- **Health probes**: providers are probed on startup and before selection
- **Auto-failover**: if preferred provider is down, fall back to next by priority
- **Config-driven**: API keys from file or environment variables, no hard-coded credentials

### 6.4 Safety Model

- **Three-tier**: Allow / Ask / Reject
- **Pattern-based**: Always-allow patterns for quick approvals (e.g., `bash:ls`)
- **Sandboxed execution**: Restricted mode isolates commands via env cleanup, timeout, output limits
- **No data exfiltration**: API keys stored only in `~/.tlaude-code/config.yaml` with user-managed permissions

### 6.5 Session Persistence

- **Format**: JSON files in `~/.tlaude-code/sessions/`
- **Naming**: UUID v4 filenames (`<id>.json`)
- **Auto-save**: session saved on each assistant response and on graceful shutdown
- **Resume**: `--resume` loads most recent; `--session <id>` loads specific

### 6.6 Plan Mode (Inspired by Claude Code)

- **"Plan first, then execute"**: Plans are drafted by parsing LLM output or free-form descriptions, then submitted for user approval before any tool calls execute
- **Step parsing**: Numbered lines (`1.`, `1)`, `-`, `*`) are auto-extracted as individual steps; unstructured text becomes a single step
- **Status lifecycle**: `draft → pending_approval → approved → executing → completed` with explicit rejection path
- **Dual creation paths**:
  - `BuildFromDescription`: user-provided text via `/plan create <desc>`
  - `BuildFromMessages`: automatic extraction from `EnterPlanMode` tool calls or `## Plan` message content
- **File persistence**: Plans saved as JSON files at `~/.tlaude-code/plans/<id>.json`, with full Save/Load/LoadAll support
- **Progress reporting**: Per-step tracking (pending/running/completed/failed/skipped) with visual progress bars and percentage calculation
- **TUI integration**: `/plan` slash commands for create/show/approve/reject/execute; plan status visible in chat

### 6.7 Plugin System

- **Three plugin types**: Lua (embedded scripts via gopher-lua), MCP (subprocess servers via existing MCP client), Hybrid (both)
- **Manifest-driven**: `plugin.yaml` at each plugin root declares name, version, type, entrypoint, and extension points
- **Extension points**: tools (register new tools), providers (LLM backends), sandboxes (execution modes), hooks (lifecycle callbacks), agents (custom sub-agents)
- **Registry**: Central plugin registry with CRUD operations, lookup by name or by provided extension point
- **Loader**: Directory-scanning discovery (`~/tlaude-code/plugins/<name>/plugin.yaml`); validates manifests and instantiates plugins
- **Manager**: Orchestrates LoadAll/Unload/Reload across all discovered plugins, with duplicate-skip and error-isolation
- **Hook system**: Event-based lifecycle hooks (ToolBefore, ToolAfter, SessionStart, SessionEnd, etc.) with ordered dispatch and result aggregation (allow/deny/pass-through)

---

## 7. Security Considerations

| Concern | Mitigation |
|---------|------------|
| API key exposure | Stored in `~/.tlaude-code/config.yaml` (0600 permissions); read from env vars as fallback |
| Command injection | Sandbox `restricted` mode cleans environment, sets timeout, limits output |
| File system access | Sandbox `restricted` mode limits paths; approval flow requires user confirmation for writes |
| Data exfiltration via MCP | MCP servers are user-configured; approval flow covers all tool calls |
| Supply chain | Go modules pinned in `go.sum`; no runtime dependencies beyond Go binary |

---

## 8. Testing Strategy

| Level | Coverage | Tools |
|-------|----------|-------|
| Unit (config) | Load/save/validation/migration | `go test` |
| Unit (session) | CRUD operations on session store | `go test` |
| Unit (registry) | Registration/selection/health | `go test` |
| Unit (MCP) | JSON-RPC marshaling | `go test` |
| Integration | TUI model updates (planned) | Bubble Tea test helpers |
| Acceptance | Full chat flow (planned) | End-to-end with mock providers |

---

## 9. Version History

| Version | Date | Summary |
|---------|------|---------|
| 2.0.0 | 2026-05-10 | Plan Mode + Plugin System + Distribution (GoReleaser, Homebrew, CI) + Full test coverage |
| 1.0.0 | 2026-05-10 | Tool System + Permission System + Multi-Agent + MoA Fusion |
| 0.3.0 | 2026-05-10 | SafetyMode + Data race fixes + Documentation |
| 0.2.0 | 2026-05-10 | MoA + Sandbox + MCP + Cost routing |
| 0.1.0 | 2026-05-10 | Initial scaffold + 7 providers + TUI |
