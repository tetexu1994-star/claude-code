# TUI Enhancement — Architecture

## Current State
Single monolithic file: `internal/tui/tui.go` (1800 lines).
Reference: `tunsuy/claude-code-go/internal/tui/` (21 files, cleanly split).

## Goals
1. **Welcome screen** — Version, quick help, recent sessions on startup
2. **Coordinator panel** — Sub-agent status overlay (list running/complete workers)
3. **Markdown rendering** — Use glamour or lipgloss-based markdown
4. **Status bar** — Model, provider, token count, cost info
5. **Spinner / activity indicator** — Show during LLM streaming/tool execution

## Implementation Plan

### 1. Split into files (no behavior change)
Read current `internal/tui/tui.go` first. Keep the full content but split:

```
internal/tui/
├── model.go      — Model + global state (extract from tui.go lines 1-200)
├── view.go       — View() rendering
├── update.go     — Update() message handling
├── init.go       — Init() + initial commands
├── styles.go     — All lipgloss styles (extract from tui.go)
├── welcome.go    — Welcome screen rendering
├── coordinator.go — Coordinator sub-agent panel
├── statusbar.go  — Status bar rendering  
├── spinner.go    — Spinner model
├── messages.go   — Custom message types (TickMsg, StreamMsg, etc.)
├── tui.go        — Package tui + TUIConfig + Run() entry point
├── diff.go       — (existing, keep)
├── approval.go   — (existing, keep)
└── tui_test.go   — (existing, keep)
```

### 2. Welcome screen
Shown on first render, before any messages:
- Tlaude Code logo/version
- Quick tips: /help, /config, /clear
- Recent sessions list (from internal/session/)
- Press any key to dismiss

### 3. Coordinator panel (in coordinator.go)
When coordinator mode is on:
- Shows list of running/completed sub-agents
- Each agent: name, status (running/done/failed), elapsed time, description
- Color-coded by status
- Updates via TickMsg every 500ms

### 4. Markdown rendering  
Use lipgloss-based rendering (no new dependency):
- Headers (h1/h2/h3) → bold + color
- Code blocks → background color + monospace
- Inline code → colored background
- Lists → proper indentation with bullet markers
- Links → underlined + colored

### 5. Status bar (in statusbar.go)
Bottom bar showing:
- Left: model name, provider
- Center: current mode (normal/coordinator/plan)
- Right: token count, cost estimate

### 6. Spinner (in spinner.go)
Simple text-based spinner:
- "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏" frame sequence
- Visible during LLM streaming and tool execution
- Pulsing "thinking..." text

### 7. Tests
Update `tui_test.go` to cover:
- Welcome screen rendering
- Coordinator panel rendering
- Status bar rendering
- Style consistency

## Do NOT modify
- Any files outside internal/tui/
- The core Bubble Tea message loop logic
- The approval flow (approval.go)
- The diff view (diff.go)

## Reference
- tunsuy/claude-code-go TUI: internal/tui/model.go, view.go, update.go, coordinator.go
- Our current TUI: internal/tui/tui.go (read first)
