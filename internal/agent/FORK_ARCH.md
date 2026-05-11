# Fork Subagent Enhancement — Architecture

## Source
`forkSubagent.ts` (210 lines) from CC source at `/Users/tetexu/claude-code-analysis/forkSubagent.ts`

## Current State
Already have `internal/agent/fork.go` with basic fork message building.
Need to enhance with full CC-source parity.

## Changes

### 1. Read the current fork.go first
Read and understand: `internal/agent/fork.go`

### 2. Types — Add to internal/agent/types.go
```go
const ForkSubagentType = "fork"

// IsInForkChild checks if messages contain the fork boilerplate tag.
// Prevents recursive forking.
func IsInForkChild(messages []llm.Message) bool

// BuildForkedMessages builds the conversation context for a fork child.
// Keeps full parent assistant message (all tool_use blocks).
// Builds a single user message with placeholder tool_results + per-child directive.
// This ensures prompt cache sharing across all fork children.
func BuildForkedMessages(directive string, assistantMsg llm.Message) []llm.Message

func BuildChildMessage(directive string) string
func BuildWorktreeNotice(parentCwd, worktreeCwd string) string
```

### 3. Constants — Add to internal/agent/fork.go
```go
const (
    ForkBoilerplateTag = "fork-boilerplate"
    ForkDirectivePrefix = "DIRECTIVE: "
    ForkPlaceholderResult = "Fork started - processing in background"
)
```

### 4. Fork AGENT definition — Add to internal/agent/definition/builtin.go
Add a fork agent (not in GetBuiltInAgents(), used implicitly):
- agentType: "fork"
- tools: ["*"]
- maxTurns: 200
- model: "inherit"
- permissionMode: "bubble"
- source: "built-in"

### 5. Feature gate
- IsForkSubagentEnabled() — checks env var CLAUDE_CODE_FORK_SUBAGENT
- Mutually exclusive with coordinator mode

### 6. Tests
Write tests for:
- BuildForkedMessages with tool_use blocks
- BuildForkedMessages without tool_use blocks
- IsInForkChild detection
- BuildChildMessage format
- BuildWorktreeNotice

### Do NOT modify
- internal/agent/definition/ (add fork agent to builtin, don't change other files)
- internal/agent/store.go
- internal/agent/runtime.go
