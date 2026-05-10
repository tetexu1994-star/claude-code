package executor

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tetexu/tlaude-code/internal/tool"
	"github.com/tetexu/tlaude-code/internal/tool/permission"
)

type testTool struct {
	name          string
	safe          bool
	execDelay     time.Duration
	execCount     *atomic.Int32
	concurrentMax *atomic.Int32
	concurrent    *atomic.Int32
}

func (t *testTool) Name() string             { return t.name }
func (t *testTool) Description() string      { return "test tool" }
func (t *testTool) IsEnabled() bool          { return true }
func (t *testTool) IsConcurrencySafe() bool  { return t.safe }
func (t *testTool) ToolDefinition() tool.ToolDefinition {
	return tool.ToolDefinition{
		Name:        t.name,
		Description: t.Description(),
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}
}

func (t *testTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
	if t.execCount != nil {
		t.execCount.Add(1)
	}
	if t.concurrent != nil {
		cur := t.concurrent.Add(1)
		defer t.concurrent.Add(-1)
		for {
			max := t.concurrentMax.Load()
			if cur <= max || t.concurrentMax.CompareAndSwap(max, cur) {
				break
			}
		}
	}
	if t.execDelay > 0 {
		select {
		case <-time.After(t.execDelay):
		case <-ctx.Done():
			return &tool.ToolResult{IsError: true, Content: "cancelled"}, nil
		}
	}
	return &tool.ToolResult{Content: t.name + ":ok"}, nil
}

func TestStreamingToolExecutor_AddAndNext(t *testing.T) {
	t.Parallel()

	pool := []tool.Tool{
		&testTool{name: "read_file", safe: true},
		&testTool{name: "Glob", safe: true},
	}
	e := New(context.Background(), pool, nil, nil)

	if err := e.AddTool("read_file", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("AddTool failed: %v", err)
	}
	if err := e.AddTool("Glob", json.RawMessage(`{"pattern":"*.go"}`)); err != nil {
		t.Fatalf("AddTool failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count := 0
	for {
		result, ok := e.NextResult(ctx)
		if !ok {
			break
		}
		if result.IsError {
			t.Errorf("unexpected error result: %s", result.Content)
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 results, got %d", count)
	}
}

func TestStreamingToolExecutor_PermissionDeny(t *testing.T) {
	t.Parallel()

	pool := []tool.Tool{
		&testTool{name: "bash", safe: false},
	}
	permCtx := permission.NewContext(permission.ModeAccepts)
	permCtx.AddDenyRule(permission.SourceUser, "bash", nil)

	e := New(context.Background(), pool, permCtx, nil)

	err := e.AddTool("bash", json.RawMessage(`{"command":"rm -rf /"}`))
	if err == nil {
		t.Error("expected permission denied error")
	}
}

func TestStreamingToolExecutor_NotFound(t *testing.T) {
	t.Parallel()

	pool := []tool.Tool{
		&testTool{name: "read_file", safe: true},
	}
	e := New(context.Background(), pool, nil, nil)

	err := e.AddTool("nonexistent", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestStreamingToolExecutor_ConcurrencyLimit(t *testing.T) {
	// Not parallel: uses time-sensitive concurrency measurement.
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	pool := []tool.Tool{}
	for i := 0; i < 20; i++ {
		name := "t" + string(rune('a'+i))
		pool = append(pool, &testTool{
			name:          name,
			safe:          true,
			execDelay:     30 * time.Millisecond,
			concurrent:    &concurrent,
			concurrentMax: &maxConcurrent,
		})
	}

	e := New(context.Background(), pool, nil, nil)
	e.SetMaxConcurrency(5)

	for _, tl := range pool {
		if err := e.AddTool(tl.Name(), json.RawMessage(`{}`)); err != nil {
			t.Fatalf("AddTool failed: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count := 0
	for {
		_, ok := e.NextResult(ctx)
		if !ok {
			break
		}
		count++
	}
	if count != 20 {
		t.Errorf("expected 20 results, got %d", count)
	}

	// With max concurrency 5, should not exceed 5 concurrent.
	if maxConcurrent.Load() > 5 {
		t.Errorf("max concurrency exceeded: %d > 5", maxConcurrent.Load())
	}
}

func TestStreamingToolExecutor_UnsafeSerial(t *testing.T) {
	// Not parallel: shares testTool concurrency counter.
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	pool := []tool.Tool{
		&testTool{name: "cmd_a", safe: false, execDelay: 20 * time.Millisecond, concurrent: &concurrent, concurrentMax: &maxConcurrent},
		&testTool{name: "cmd_b", safe: false, execDelay: 20 * time.Millisecond, concurrent: &concurrent, concurrentMax: &maxConcurrent},
		&testTool{name: "cmd_c", safe: false, execDelay: 20 * time.Millisecond, concurrent: &concurrent, concurrentMax: &maxConcurrent},
	}

	e := New(context.Background(), pool, nil, nil)

	for _, tl := range pool {
		if err := e.AddTool(tl.Name(), json.RawMessage(`{}`)); err != nil {
			t.Fatalf("AddTool failed: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count := 0
	for {
		_, ok := e.NextResult(ctx)
		if !ok {
			break
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 results, got %d", count)
	}

	// Unsafe tools should never exceed 1 concurrent.
	if maxConcurrent.Load() > 1 {
		t.Errorf("unsafe tools had concurrent execution: %d > 1", maxConcurrent.Load())
	}
}

func TestStreamingToolExecutor_MixedConcurrency(t *testing.T) {
	// Not parallel: shares testTool concurrency counter.
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	pool := []tool.Tool{
		&testTool{name: "rd_a", safe: true, execDelay: 20 * time.Millisecond, concurrent: &concurrent, concurrentMax: &maxConcurrent},
		&testTool{name: "wr_a", safe: false, execDelay: 20 * time.Millisecond, concurrent: &concurrent, concurrentMax: &maxConcurrent},
		&testTool{name: "rd_b", safe: true, execDelay: 10 * time.Millisecond, concurrent: &concurrent, concurrentMax: &maxConcurrent},
	}

	e := New(context.Background(), pool, nil, nil)

	for _, tl := range pool {
		if err := e.AddTool(tl.Name(), json.RawMessage(`{}`)); err != nil {
			t.Fatalf("AddTool failed: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count := 0
	for {
		_, ok := e.NextResult(ctx)
		if !ok {
			break
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 results, got %d", count)
	}
}

func TestStreamingToolExecutor_OrderedResults(t *testing.T) {
	t.Parallel()

	pool := []tool.Tool{
		&testTool{name: "tool_a", safe: true, execDelay: 30 * time.Millisecond},
		&testTool{name: "tool_b", safe: true, execDelay: 10 * time.Millisecond},
		&testTool{name: "tool_c", safe: true, execDelay: 20 * time.Millisecond},
	}

	e := New(context.Background(), pool, nil, nil)

	e.AddTool("tool_a", json.RawMessage(`{}`))
	e.AddTool("tool_b", json.RawMessage(`{}`))
	e.AddTool("tool_c", json.RawMessage(`{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var names []string
	for {
		result, ok := e.NextResult(ctx)
		if !ok {
			break
		}
		names = append(names, result.Content)
	}

	expected := []string{"tool_a:ok", "tool_b:ok", "tool_c:ok"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d results, got %d", len(expected), len(names))
	}
	for i, exp := range expected {
		if names[i] != exp {
			t.Errorf("result[%d]: expected %q, got %q", i, exp, names[i])
		}
	}
}

func TestStreamingToolExecutor_CloseClearsPending(t *testing.T) {
	t.Parallel()

	pool := []tool.Tool{
		&testTool{name: "t1", safe: true, execDelay: 10 * time.Millisecond},
	}

	e := New(context.Background(), pool, nil, nil)
	e.AddTool("t1", json.RawMessage(`{}`))

	// Wait for the result to be available.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e.NextResult(ctx) // consume the result

	// After consuming all results, pending should be 0.
	if e.PendingCount() != 0 {
		t.Errorf("expected 0 pending after consuming all, got %d", e.PendingCount())
	}
}

func TestStreamingToolExecutor_SetMaxConcurrency(t *testing.T) {
	t.Parallel()

	e := New(context.Background(), nil, nil, nil)

	e.SetMaxConcurrency(3)
	if e.maxSafe != 3 {
		t.Errorf("expected maxSafe=3, got %d", e.maxSafe)
	}

	// Clamp to 1.
	e.SetMaxConcurrency(0)
	if e.maxSafe != 1 {
		t.Errorf("expected maxSafe=1 for 0 input, got %d", e.maxSafe)
	}
}

func TestStreamingToolExecutor_ClosedAddReturnsError(t *testing.T) {
	t.Parallel()

	pool := []tool.Tool{
		&testTool{name: "t1", safe: true},
	}
	e := New(context.Background(), pool, nil, nil)
	e.Discard()

	err := e.AddTool("t1", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error when adding to closed executor")
	}
}
