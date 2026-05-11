# Coordinator Mode — Architecture

## Source
`coordinatorMode.ts` (369 lines) from CC source at `/Users/tetexu/claude-code-analysis/coordinatorMode.ts`

## Purpose
Coordinator mode transforms an agent into an orchestrator that delegates work
to worker sub-agents. The coordinator's job is to: research → synthesize →
implement (via workers) → verify (via workers). Workers run asynchronously
and report back via task notifications.

## New Package: `internal/coordinator/`

```
internal/coordinator/
├── mode.go      — IsCoordinatorMode, MatchSessionMode, constants
├── context.go   — GetCoordinatorUserContext
├── prompt.go    — GetCoordinatorSystemPrompt (the big 369-line prompt)
└── mode_test.go — Tests
```

### mode.go
```go
const CoordinatorEnvVar = "TLAUDE_CODE_COORDINATOR_MODE"

// IsCoordinatorMode checks if coordinator mode is enabled via env var.
func IsCoordinatorMode() bool

// MatchSessionMode checks if stored session mode matches current env.
// If mismatched, updates env var to match session. Returns a warning
// message if switched.
func MatchSessionMode(sessionMode string) (string, bool)
```

### context.go
```go
// WorkerTools returns the list of tools available to worker sub-agents.
// In simple mode: Bash, Read, Edit.
// In full mode: standard async agent tools minus internal tools.
func WorkerTools() []string

// GetCoordinatorUserContext returns user context for workers.
// Includes worker tool list, MCP servers, and scratchpad dir.
func GetCoordinatorUserContext(mcpClients []MCPServerInfo, scratchpadDir string) map[string]string

type MCPServerInfo struct {
    Name string
}

// GetCoordinatorSystemPrompt returns the full coordinator system prompt.
// This is the main output — 369 lines of detailed orchestration guidance.
func GetCoordinatorSystemPrompt(simpleMode bool) string
```

### prompt.go
Contains the full CC coordinator system prompt adapted for Tlaude Code:
- Role: coordinator that orchestrates workers
- Tools: Agent (spawn), SendMessage (continue), TaskStop (stop), subscribe_pr_activity
- Agent tool results as task-notification XML
- Task workflow: Research → Synthesis → Implementation → Verification
- Concurrency guidance (parallel research, serial writes)
- Worker prompt quality guidelines (synthesize, don't delegate understanding)
- Continue vs spawn decision matrix
- Full example session

## Integration Points

### With internal/agent/runtime.go
- When coordinator mode is enabled, the main agent uses
  GetCoordinatorSystemPrompt() instead of the default system prompt
- The AgentTool passes user context from GetCoordinatorUserContext()
- Worker agents are spawned via the existing RunAgentAsync mechanism

### With internal/agent/types.go
- Add TaskNotification type for parsing XML task results
- WorkerResult struct matching <task-notification> format

### With internal/swarm/
- Coordinator mode uses swarm's in-process backend for worker spawning
- Workers report back via SendMessage mechanism

## Implementation Plan
1. Create `mode.go` — feature gate + session matching
2. Create `context.go` — worker tool configuration + user context
3. Create `prompt.go` — full coordinator system prompt
4. Create `mode_test.go` — env var toggle, session matching

## Reference
- CC source: /Users/tetexu/claude-code-analysis/coordinatorMode.ts
- Our existing agent: internal/agent/runtime.go
- Our existing swarm: internal/swarm/
