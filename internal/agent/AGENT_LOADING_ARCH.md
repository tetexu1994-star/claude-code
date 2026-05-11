# Agent Definition Loading System — Architecture Design

## Motivation
Port CC's `loadAgentsDir.ts` (755 lines), `builtInAgents.ts` (73 lines),
and `agentMemory.ts` (177 lines) to Go. This is the glue layer that connects
our existing `agent/store.go`, `internal/memory/` (MemDir), and
`internal/swarm/` — allowing users to define custom agents via markdown
frontmatter, JSON config, or directory scanning.

## Source Files to Port
1. **loadAgentsDir.ts** (755 lines) — Core: types, parsers, directory scanner
2. **builtInAgents.ts** (73 lines) — Built-in agent registry
3. **agentMemory.ts** (177 lines) — Memory scope integration
4. **forkSubagent.ts** (210 lines) — Fork subagent (Phase 2)

## Architecture

### New Package: `internal/agent/definition/`

```
internal/agent/definition/
├── types.go          # AgentDefinition, BuiltInAgentDef, CustomAgentDef
├── parse_md.go       # parseAgentFromMarkdown — YAML frontmatter parser
├── parse_json.go     # parseAgentFromJson, parseAgentsFromJson
├── loader.go         # LoadAgentsDir — directory scanner + merge logic
├── builtin.go        # GetBuiltInAgents — built-in agent registry
└── memory.go         # Agent memory prompt integration
```

### Types (types.go)
```go
type Source string
const (
    SourceBuiltIn  Source = "built-in"
    SourceUser     Source = "userSettings"
    SourceProject  Source = "projectSettings"
    SourcePolicy   Source = "policySettings"
    SourcePlugin   Source = "plugin"
)

type MemoryScope string
const (
    MemUser    MemoryScope = "user"
    MemProject MemoryScope = "project"
    MemLocal   MemoryScope = "local"
)

type AgentMcpServerSpec struct {
    Name          string // reference by name
    InlineConfig  *McpServerConfig // inline definition
}

type AgentDefinition struct {
    AgentType        string
    WhenToUse        string
    Tools            []string // nil = all tools
    DisallowedTools  []string
    Skills           []string
    McpServers       []AgentMcpServerSpec
    Hooks            *HooksSettings
    Color            string
    Model            string // "inherit" or model name
    Effort           *EffortValue
    PermissionMode   *string // "bubble", "bypassPermissions", etc.
    MaxTurns         *int
    Background       bool
    InitialPrompt    string
    Memory           *MemoryScope
    Isolation        string // "worktree"
    Source           Source
    Filename         string // for user/project agents (without .md)
    BaseDir          string // for file-sourced agents
    Plugin           string // for plugin agents
    SystemPrompt     string // static prompt
}

type AgentDefinitionsResult struct {
    ActiveAgents []*AgentDefinition
    AllAgents    []*AgentDefinition
    FailedFiles  []ParseError
}

type ParseError struct {
    Path  string
    Error string
}
```

### Markdown Parser (parse_md.go)
- Read YAML frontmatter from `.md` files (between `---` delimiters)
- Body text becomes `SystemPrompt`
- Required frontmatter fields: `name`, `description`
- Optional: tools, disallowedTools, skills, memory, model, color, effort,
  permissionMode, maxTurns, background, initialPrompt, isolation, mcpServers
- Validation with sensible error messages
- Uses `gopkg.in/yaml.v3` (no new dep — already used for config)

### JSON Parser (parse_json.go)
- Parse agent definitions from JSON objects
- Same validation as markdown parser
- Used for config-based agents (settings files)

### Directory Loader (loader.go)
- `LoadAgentsDir(cwd string) (*AgentDefinitionsResult, error)`
- Scans `<cwd>/agents/*.md` for custom agents
- Merges with built-in agents from `builtin.go`
- Priority ordering: built-in → plugin → user → project → policy → flag
- Deduplication by `agentType` (last wins by priority)
- Memoized with cache clearing support

### Built-in Agents (builtin.go)
- `GetBuiltInAgents() []*AgentDefinition`
- Returns: GeneralPurpose agent, CLI Guide agent, Explore agent, Plan agent
- Each has a static SystemPrompt and agentType
- Feature-gated (e.g., Explore/Plan enabled by config flag)

### Memory Integration (memory.go)
- `LoadAgentMemoryPrompt(agentType string, scope MemoryScope) string`
- Uses existing `internal/memory/` for MemDir
- Scope-specific instructions (user = cross-project, project = team-shared, local = machine-local)
- Auto-creates memory directory on access
- Injects memory prompt into system prompt at agent spawn time

## Integration Points

### With internal/agent/store.go
- `definition.LoadAgentsDir()` → feeds into `agent/store.RegisterAgent()`
- Store uses `GetActiveAgents()` for all agent lookups

### With internal/memory/ (MemDir)
- `definition.LoadAgentMemoryPrompt()` → uses `memory.NewMemDir()`
- Path: `<memoryBase>/agent-memory/<agentType>/`

### With internal/swarm/
- Agent definitions include `memory` scope for persistent memory
- Teams can reference agents by `agentType`

## Files to Modify
- `internal/agent/store.go` — Add `RegisterAgent(*AgentDefinition)` method
- `internal/agent/runtime.go` — Use AgentDefinition instead of bare AgentConfig

## Implementation Plan
1. Create `types.go` with all types
2. Create `parse_md.go` with YAML frontmatter parser
3. Create `parse_json.go` with JSON parser
4. Create `builtin.go` with built-in agents
5. Create `loader.go` with directory scanner + merge logic
6. Create `memory.go` with MemDir integration
7. Update `store.go` to accept AgentDefinition
8. Tests for each package
