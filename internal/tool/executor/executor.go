// Package executor provides the StreamingToolExecutor for executing tool calls
// with permission checks, concurrency control, and ordered result delivery.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/tetexu/tlaude-code/internal/tool"
	"github.com/tetexu/tlaude-code/internal/tool/permission"
)

// ToolStatus represents the execution status of a tracked tool.
type ToolStatus string

const (
	StatusQueued    ToolStatus = "queued"
	StatusExecuting ToolStatus = "executing"
	StatusCompleted ToolStatus = "completed"
)

// TrackedTool wraps a tool invocation with execution state.
type TrackedTool struct {
	ID     string
	Tool   tool.Tool
	Input  json.RawMessage
	Status ToolStatus
	Result chan *tool.ToolResult
}

// StreamingToolExecutor manages ordered, permission-aware execution of tool calls.
type StreamingToolExecutor struct {
	pool    []tool.Tool
	permCtx *permission.PermissionContext
	toolCtx *tool.ToolContext

	maxSafe int // max concurrent safe tools (default 10)

	// Per-instance concurrency control.
	safeSem chan struct{} // semaphore for safe tools (capacity = maxSafe)
	rwMu    sync.RWMutex  // reader-writer: safe tools RLock, unsafe tools Lock

	mu        sync.Mutex
	queue     []*TrackedTool     // ordered queue of tools to execute
	results   []*tool.ToolResult // results indexed by queue position
	nextIdx   int                // next index to emit via NextResult
	nextReady chan struct{}      // buffered signal for new results (capacity 1)
	ctx       context.Context
	cancel    context.CancelFunc
	closed    bool
}

// New creates a new StreamingToolExecutor.
func New(ctx context.Context, pool []tool.Tool, permCtx *permission.PermissionContext, toolCtx *tool.ToolContext) *StreamingToolExecutor {
	ctx, cancel := context.WithCancel(ctx)
	maxSafe := 10
	safeSem := make(chan struct{}, maxSafe)
	for i := 0; i < maxSafe; i++ {
		safeSem <- struct{}{}
	}
	e := &StreamingToolExecutor{
		pool:      pool,
		permCtx:   permCtx,
		toolCtx:   toolCtx,
		maxSafe:   maxSafe,
		safeSem:   safeSem,
		ctx:       ctx,
		cancel:    cancel,
		nextReady: make(chan struct{}, 1),
	}
	return e
}

// SetMaxConcurrency sets the max number of concurrent safe tools.
// Must be called before any AddTool calls.
func (e *StreamingToolExecutor) SetMaxConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.maxSafe = n
	e.safeSem = make(chan struct{}, n)
	for i := 0; i < n; i++ {
		e.safeSem <- struct{}{}
	}
}

// findTool looks up a tool by name in the pool.
func (e *StreamingToolExecutor) findTool(name string) tool.Tool {
	for _, t := range e.pool {
		if t.Name() == name {
			return t
		}
	}
	return nil
}

// AddTool queues a tool call for execution. Returns an error if the tool is
// not found, denied by permissions, or if the executor is already closed.
func (e *StreamingToolExecutor) AddTool(name string, input json.RawMessage) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return fmt.Errorf("executor is closed")
	}

	tl := e.findTool(name)
	if tl == nil {
		return fmt.Errorf("tool %q not found", name)
	}

	if e.permCtx != nil && !e.permCtx.IsBypassed() {
		decision := permission.Decide(e.permCtx, name, string(input))
		if decision.Behavior == permission.BehaviorDeny {
			return fmt.Errorf("tool %q denied: %s", name, decision.DecisionReason)
		}
	}

	id := fmt.Sprintf("call-%d", len(e.queue))
	tt := &TrackedTool{
		ID:     id,
		Tool:   tl,
		Input:  input,
		Status: StatusQueued,
		Result: make(chan *tool.ToolResult, 1),
	}
	idx := len(e.queue)
	e.queue = append(e.queue, tt)

	if len(e.results) <= idx {
		e.results = append(e.results, nil)
	}

	go e.execute(idx, tt)
	return nil
}

// execute runs a single tool and stores the result at the given index.
func (e *StreamingToolExecutor) execute(idx int, tt *TrackedTool) {
	e.acquireSlot(tt.Tool)
	defer e.releaseSlot(tt.Tool)

	e.mu.Lock()
	if tt.Status == StatusQueued {
		tt.Status = StatusExecuting
	}
	e.mu.Unlock()

	result, _ := tt.Tool.Execute(e.ctx, tt.Input, e.toolCtx)

	e.mu.Lock()
	tt.Status = StatusCompleted
	e.results[idx] = result
	// Signal that a new result is ready (non-blocking).
	select {
	case e.nextReady <- struct{}{}:
	default:
	}
	e.mu.Unlock()

	select {
	case tt.Result <- result:
	default:
	}
}

func (e *StreamingToolExecutor) acquireSlot(tl tool.Tool) {
	if tl.IsConcurrencySafe() {
		e.rwMu.RLock()
		<-e.safeSem
	} else {
		e.rwMu.Lock()
		for i := 0; i < e.maxSafe; i++ {
			<-e.safeSem
		}
	}
}

func (e *StreamingToolExecutor) releaseSlot(tl tool.Tool) {
	if tl.IsConcurrencySafe() {
		e.safeSem <- struct{}{}
		e.rwMu.RUnlock()
	} else {
		for i := 0; i < e.maxSafe; i++ {
			e.safeSem <- struct{}{}
		}
		e.rwMu.Unlock()
	}
}

// NextResult returns the next completed tool result in queue order.
// Returns (nil, false) when all results have been consumed or the context is done.
func (e *StreamingToolExecutor) NextResult(ctx context.Context) (*tool.ToolResult, bool) {
	for {
		e.mu.Lock()
		if e.nextIdx >= len(e.queue) {
			e.mu.Unlock()
			return nil, false
		}
		if e.results[e.nextIdx] != nil {
			result := e.results[e.nextIdx]
			e.nextIdx++
			e.mu.Unlock()
			return result, true
		}
		e.mu.Unlock()

		// Wait for a signal or context cancellation.
		select {
		case <-e.nextReady:
		case <-ctx.Done():
			return nil, false
		}
	}
}

// Discard cancels all pending and running tool executions.
func (e *StreamingToolExecutor) Discard() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return
	}
	e.closed = true
	e.cancel()
}

// Close cancels all pending work. It does not wait for in-flight goroutines.
func (e *StreamingToolExecutor) Close() {
	e.Discard()
}

// PendingCount returns the number of tools that have not yet been consumed via NextResult.
func (e *StreamingToolExecutor) PendingCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.queue) - e.nextIdx
}
