package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/tetexu/tlaude-code/internal/tool"
)

func TestNewToolBridge(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	if tb == nil {
		t.Fatal("expected non-nil tool bridge")
	}
	if tb.Count() != 0 {
		t.Errorf("expected 0 tools, got %d", tb.Count())
	}
}

func TestToolBridgeRegister(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "greet",
    description = "Greets a person",
    input_schema = {
        type = "object",
        properties = {
            name = { type = "string", description = "Who to greet" }
        },
        required = {"name"}
    },
    handler = function(args)
        return {content = "Hello, " .. args.name .. "!"}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	if tb.Count() != 1 {
		t.Fatalf("expected 1 tool, got %d", tb.Count())
	}

	goTools := tb.GetGoTools()
	if len(goTools) != 1 {
		t.Fatalf("expected 1 Go tool, got %d", len(goTools))
	}

	gt := goTools[0]
	if gt.Name() != "greet" {
		t.Errorf("expected name 'greet', got %q", gt.Name())
	}
	if gt.Description() != "Greets a person" {
		t.Errorf("unexpected description: %q", gt.Description())
	}
	if !gt.IsEnabled() {
		t.Error("expected tool to be enabled")
	}
	if !gt.IsConcurrencySafe() {
		t.Error("expected tool to be concurrency-safe")
	}

	// Verify ToolDefinition has the input schema.
	def := gt.ToolDefinition()
	if def.Name != "greet" {
		t.Errorf("expected def name 'greet', got %q", def.Name)
	}
	if len(def.InputSchema) == 0 {
		t.Error("expected non-empty input schema")
	}
}

func TestLuaToolExecute(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "greet",
    description = "Greets a person",
    input_schema = {
        type = "object",
        properties = {
            name = { type = "string" }
        },
        required = {"name"}
    },
    handler = function(args)
        return {content = "Hello, " .. args.name .. "!"}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	if len(goTools) != 1 {
		t.Fatalf("expected 1 Go tool, got %d", len(goTools))
	}

	gt := goTools[0]

	// Call Execute with JSON input.
	input := json.RawMessage(`{"name": "World"}`)
	result, err := gt.Execute(ctx, input, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", result.Content)
	}
}

func TestLuaToolWithArgs(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "repeat_str",
    description = "Repeats a string N times",
    input_schema = {
        type = "object",
        properties = {
            text = { type = "string" },
            count = { type = "number" }
        },
        required = {"text", "count"}
    },
    handler = function(args)
        local result = ""
        for i = 1, args.count do
            result = result .. args.text
        end
        return {content = result}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	if len(goTools) != 1 {
		t.Fatalf("expected 1 Go tool, got %d", len(goTools))
	}

	gt := goTools[0]

	input := json.RawMessage(`{"text": "ab", "count": 3}`)
	result, err := gt.Execute(ctx, input, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "ababab" {
		t.Errorf("expected 'ababab', got %q", result.Content)
	}
}

func TestLuaToolWithError(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "failing_tool",
    description = "Always fails",
    input_schema = {
        type = "object",
        properties = {}
    },
    handler = function(args)
        return {content = "something went wrong", is_error = true}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	gt := goTools[0]

	result, err := gt.Execute(ctx, json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	if result.Content != "something went wrong" {
		t.Errorf("unexpected content: %q", result.Content)
	}
}

func TestLuaToolTimeout(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 1})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "slow_tool",
    description = "Runs forever",
    input_schema = {
        type = "object",
        properties = {}
    },
    handler = function(args)
        while true do
            -- infinite loop
        end
        return {content = "never reached"}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	gt := goTools[0]

	start := time.Now()
	result, err := gt.Execute(ctx, json.RawMessage(`{}`), nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for timeout")
	}
	// Should be killed within a few seconds.
	if elapsed > 5*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestLuaToolVMReuse(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 1, Timeout: 5})
	if err := eng.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
local count = 0
tools.register({
    name = "counter",
    description = "Counts invocations",
    input_schema = {
        type = "object",
        properties = {}
    },
    handler = function(args)
        count = count + 1
        return {content = tostring(count)}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	gt := goTools[0]

	// Each execution gets a fresh VM from the pool (count resets to 0).
	for i := 1; i <= 3; i++ {
		result, err := gt.Execute(ctx, json.RawMessage(`{}`), nil)
		if err != nil {
			t.Fatalf("Execute %d: %v", i, err)
		}
		// Since each call gets a fresh VM, count should always be "1".
		if result.Content != "1" {
			t.Errorf("call %d: expected '1', got %q", i, result.Content)
		}
	}
}

func TestDuplicateToolName(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "dup",
    description = "First",
    input_schema = { type = "object", properties = {} },
    handler = function(args) return {content = "first"} end
})
tools.register({
    name = "dup",
    description = "Second",
    input_schema = { type = "object", properties = {} },
    handler = function(args) return {content = "second"} end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	// Duplicate should be silently ignored (only first registration kept).
	if tb.Count() != 1 {
		t.Errorf("expected 1 tool after duplicate registration, got %d", tb.Count())
	}

	goTools := tb.GetGoTools()
	if goTools[0].Description() != "First" {
		t.Errorf("expected 'First' description to be kept, got %q", goTools[0].Description())
	}
}

func TestRegisterThenExecuteFullFlow(t *testing.T) {
	// Full end-to-end: register from Lua, get Go tool, call Execute.
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "concat",
    description = "Concatenates strings",
    input_schema = {
        type = "object",
        properties = {
            a = { type = "string" },
            b = { type = "string" }
        },
        required = {"a", "b"}
    },
    handler = function(args)
        return {content = args.a .. args.b}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	// Register in a Go tool.Registry.
	reg := tool.NewRegistry()
	for _, lt := range tb.GetGoTools() {
		if err := reg.Register(lt); err != nil {
			t.Fatalf("Register in tool.Registry: %v", err)
		}
	}

	// Retrieve and execute.
	got, ok := reg.Get("concat")
	if !ok {
		t.Fatal("tool not found in registry")
	}

	result, err := got.Execute(ctx, json.RawMessage(`{"a": "foo", "b": "bar"}`), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "foobar" {
		t.Errorf("expected 'foobar', got %q", result.Content)
	}
}

func TestLuaToolNamedHandler(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
function my_handler(args)
    return {content = "named: " .. args.val}
end

tools.register({
    name = "named_tool",
    description = "Uses a named handler",
    input_schema = {
        type = "object",
        properties = { val = { type = "string" } },
        required = {"val"}
    },
    handler = "my_handler"
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	gt := goTools[0]

	result, err := gt.Execute(ctx, json.RawMessage(`{"val": "test"}`), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "named: test" {
		t.Errorf("expected 'named: test', got %q", result.Content)
	}
}

func TestLuaToolMultipleTools(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "upper",
    description = "Uppercases text",
    input_schema = {
        type = "object",
        properties = { text = { type = "string" } },
        required = {"text"}
    },
    handler = function(args)
        return {content = string.upper(args.text)}
    end
})

tools.register({
    name = "lower",
    description = "Lowercases text",
    input_schema = {
        type = "object",
        properties = { text = { type = "string" } },
        required = {"text"}
    },
    handler = function(args)
        return {content = string.lower(args.text)}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	if tb.Count() != 2 {
		t.Fatalf("expected 2 tools, got %d", tb.Count())
	}

	goTools := tb.GetGoTools()
	toolMap := make(map[string]tool.Tool)
	for _, gt := range goTools {
		toolMap[gt.Name()] = gt
	}

	upperTool := toolMap["upper"]
	result, err := upperTool.Execute(ctx, json.RawMessage(`{"text": "Hello"}`), nil)
	if err != nil {
		t.Fatalf("upper.Execute: %v", err)
	}
	if result.Content != "HELLO" {
		t.Errorf("expected 'HELLO', got %q", result.Content)
	}

	lowerTool := toolMap["lower"]
	result, err = lowerTool.Execute(ctx, json.RawMessage(`{"text": "Hello"}`), nil)
	if err != nil {
		t.Fatalf("lower.Execute: %v", err)
	}
	if result.Content != "hello" {
		t.Errorf("expected 'hello', got %q", result.Content)
	}
}

func TestLuaToolEmptyInput(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "no_args",
    description = "Takes no arguments",
    input_schema = { type = "object", properties = {} },
    handler = function(args)
        return {content = "done"}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	gt := goTools[0]

	result, err := gt.Execute(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "done" {
		t.Errorf("expected 'done', got %q", result.Content)
	}
}

func TestLuaToolRoundTrip(t *testing.T) {
	// JSON args → Lua → JSON result round trip.
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "json_echo",
    description = "Echoes args as JSON",
    input_schema = {
        type = "object",
        properties = {
            data = { type = "object" }
        },
        required = {"data"}
    },
    handler = function(args)
        local encoded = json.encode(args.data)
        return {content = encoded}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	gt := goTools[0]

	input := json.RawMessage(`{"data": {"nested": "value", "num": 42}}`)
	result, err := gt.Execute(ctx, input, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	// Verify the JSON round-tripped correctly.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result.Content), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if parsed["nested"] != "value" {
		t.Errorf("unexpected nested: %v", parsed["nested"])
	}
	// JSON numbers unmarshal as float64.
	if parsed["num"] != float64(42) {
		t.Errorf("unexpected num: %v", parsed["num"])
	}
}

func TestLuaToolStringReturn(t *testing.T) {
	// Handler that returns a plain string instead of a table.
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "plain_return",
    description = "Returns a plain string",
    input_schema = { type = "object", properties = {} },
    handler = function(args)
        return "just a string"
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	gt := goTools[0]

	result, err := gt.Execute(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if result.Content != "just a string" {
		t.Errorf("expected 'just a string', got %q", result.Content)
	}
}

func TestLuaToolMissingHandler(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "bad_tool",
    description = "Missing handler",
    input_schema = { type = "object", properties = {} }
    -- no handler field
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	err = eng.Execute(ctx, script)
	if err == nil {
		t.Fatal("expected error for missing handler")
	}
}

func TestLuaToolWithoutBridge(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()
	// Do NOT set toolBridge on engine.

	code := `
tools.register({
    name = "orphan",
    description = "No bridge",
    input_schema = { type = "object", properties = {} },
    handler = function(args) return {content = "ok"} end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	err = eng.Execute(ctx, script)
	if err == nil {
		t.Fatal("expected error when bridge is nil")
	}
}

func TestLuaToolConcurrent(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 4, Timeout: 10})
	defer eng.Stop()

	tb := NewToolBridge(eng)
	eng.SetToolBridge(tb)

	code := `
tools.register({
    name = "echo",
    description = "Echoes input",
    input_schema = {
        type = "object",
        properties = { msg = { type = "string" } },
        required = {"msg"}
    },
    handler = function(args)
        return {content = args.msg}
    end
})
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer script.Close()

	goTools := tb.GetGoTools()
	gt := goTools[0]

	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			input := json.RawMessage(
				fmt.Sprintf(`{"msg": "hello %d"}`, idx),
			)
			result, err := gt.Execute(ctx, input, nil)
			if err != nil {
				errs <- err
				return
			}
			if result.IsError {
				errs <- fmt.Errorf("unexpected error: %s", result.Content)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent execute error: %v", err)
	}
}
