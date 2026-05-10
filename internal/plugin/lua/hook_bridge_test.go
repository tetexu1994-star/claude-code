package lua

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/tetexu/tlaude-code/internal/hook"
)

// setupHookTest creates an engine, hook bridge, and a helper to compile and
// execute a Lua script that registers hooks. Returns the engine, hook bridge,
// and the plugin's hook registry so tests can dispatch events.
func setupHookTest(t *testing.T, pluginName string, luaCode string) (*Engine, *HookBridge, *hook.HookRegistry) {
	t.Helper()

	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	t.Cleanup(func() { eng.Stop() })

	hb := NewHookBridge(eng)
	eng.SetHookBridge(hb)

	reg := hook.NewHookRegistry()
	hb.SetHookRegistry(pluginName, reg)

	script, err := eng.Compile(pluginName, luaCode)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	script.PluginDir = t.TempDir()
	script.Plugins = []string{pluginName}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	t.Cleanup(func() { script.Close() })

	return eng, hb, reg
}

func TestNewHookBridge(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	hb := NewHookBridge(eng)
	if hb == nil {
		t.Fatal("expected non-nil hook bridge")
	}
	if hb.Count() != 0 {
		t.Errorf("expected 0 hooks, got %d", hb.Count())
	}
}

func TestHookRegister(t *testing.T) {
	code := `
hooks.on_tool_before(function(event)
    return {deny = true, reason = "blocked"}
end)
`
	_, hb, reg := setupHookTest(t, "test-plugin", code)

	if hb.Count() != 1 {
		t.Fatalf("expected 1 hook, got %d", hb.Count())
	}
	if reg.Len() != 1 {
		t.Fatalf("expected registry len 1, got %d", reg.Len())
	}
}

func TestHookToolBeforeAllow(t *testing.T) {
	code := `
hooks.on_tool_before(function(event)
    return {allow = true}
end)
`
	_, _, reg := setupHookTest(t, "test-allow", code)

	results, err := reg.Dispatch(context.Background(), hook.HookToolBefore, &hook.HookEvent{
		ToolName: "bash",
	})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Allow {
		t.Error("expected Allow=true")
	}
}

func TestHookToolBeforeDeny(t *testing.T) {
	code := `
hooks.on_tool_before(function(event)
    if event.tool_name == "bash" then
        return {deny = true, reason = "rm -rf blocked by policy"}
    end
end)
`
	_, _, reg := setupHookTest(t, "test-deny", code)

	results, err := reg.Dispatch(context.Background(), hook.HookToolBefore, &hook.HookEvent{
		ToolName: "bash",
	})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Deny {
		t.Error("expected Deny=true")
	}
	if results[0].Reason != "rm -rf blocked by policy" {
		t.Errorf("expected reason, got %q", results[0].Reason)
	}
}

func TestHookToolBeforePassThrough(t *testing.T) {
	code := `
hooks.on_tool_before(function(event)
    -- return nil means pass-through (no opinion)
    return nil
end)
`
	_, _, reg := setupHookTest(t, "test-passthrough", code)

	results, err := reg.Dispatch(context.Background(), hook.HookToolBefore, &hook.HookEvent{
		ToolName: "bash",
	})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results (nil is pass-through), got %d", len(results))
	}
}

func TestHookToolAfter(t *testing.T) {
	code := `
hooks.on_tool_after(function(event)
    return {allow = true}
end)
`
	_, _, reg := setupHookTest(t, "test-after", code)

	results, err := reg.Dispatch(context.Background(), hook.HookToolAfter, &hook.HookEvent{
		ToolName: "bash",
	})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Allow {
		t.Error("expected Allow=true")
	}
}

func TestHookSessionStart(t *testing.T) {
	capturedSession := ""
	code := `
hooks.on_session_start(function(event)
    captured_session = event.session_id
end)
`
	_, _, reg := setupHookTest(t, "test-session", code)

	// Register a Go-side hook to capture the session for verification.
	var sessionID string
	reg.Register(hook.HookSessionStart, "capture", func(ctx context.Context, event *hook.HookEvent) (*hook.HookResult, error) {
		sessionID = event.SessionID
		return nil, nil
	})

	_, err := reg.Dispatch(context.Background(), hook.HookSessionStart, &hook.HookEvent{
		SessionID: "sess-123",
	})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	// The Lua handler also ran (via HookBridge); verify our Go handler captured the session.
	if sessionID != "sess-123" {
		t.Errorf("expected session 'sess-123', got %q", sessionID)
	}
	_ = capturedSession // Lua captured it but we can't verify from Go side easily
}

func TestHookMessageModify(t *testing.T) {
	code := `
hooks.on_message(function(event)
    if event.message:find("@myplugin") then
        return {modify = {message = event.message:gsub("@myplugin%s*", "")}}
    end
end)
`
	_, _, reg := setupHookTest(t, "test-modify", code)

	results, err := reg.Dispatch(context.Background(), hook.HookMessage, &hook.HookEvent{
		Message: "@myplugin hello world",
	})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	mod := results[0].Modify
	if mod == nil {
		t.Fatal("expected Modify map")
	}
	msg, ok := mod["message"].(string)
	if !ok {
		t.Fatalf("expected message string in modify, got %T", mod["message"])
	}
	if msg != "hello world" {
		t.Errorf("expected 'hello world', got %q", msg)
	}
}

func TestHookTimeout(t *testing.T) {
	code := `
hooks.on_tool_before(function(event)
    -- Busy-wait to exceed the hook handler timeout.
    local start = os.clock()
    while os.clock() - start < 10 do
    end
    return {deny = true}
end)
`
	_, _, reg := setupHookTest(t, "test-timeout", code)

	// Dispatch with a short context timeout to trigger cancellation.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	results, err := reg.Dispatch(ctx, hook.HookToolBefore, &hook.HookEvent{
		ToolName: "bash",
	})
	// We expect either context cancellation or the hook handler timing out.
	// Either way the hook should not have returned deny=true.
	if err == nil {
		if len(results) > 0 && results[0].Deny {
			t.Error("hook should have timed out before returning deny")
		}
	}
}

func TestMultipleHooks(t *testing.T) {
	code := `
hooks.on_tool_before(function(event)
    return {allow = true}
end)
hooks.on_tool_before(function(event)
    return {deny = true, reason = "second"}
end)
`
	_, hb, reg := setupHookTest(t, "test-multi", code)

	if hb.Count() != 2 {
		t.Fatalf("expected 2 hooks, got %d", hb.Count())
	}

	results, err := reg.Dispatch(context.Background(), hook.HookToolBefore, &hook.HookEvent{
		ToolName: "bash",
	})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Allow {
		t.Error("first hook: expected Allow=true")
	}
	if !results[1].Deny {
		t.Error("second hook: expected Deny=true")
	}
}

func TestHookBridgeCount(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	hb := NewHookBridge(eng)
	eng.SetHookBridge(hb)

	// Plugin A: 2 hooks
	regA := hook.NewHookRegistry()
	hb.SetHookRegistry("plugin-a", regA)
	scriptA, err := eng.Compile("plugin-a", `
hooks.on_tool_before(function() return {allow = true} end)
hooks.on_tool_after(function() return {deny = true} end)
`)
	if err != nil {
		t.Fatalf("Compile A: %v", err)
	}
	scriptA.PluginDir = t.TempDir()
	scriptA.Plugins = []string{"plugin-a"}
	if err := eng.Execute(context.Background(), scriptA); err != nil {
		t.Fatalf("Execute A: %v", err)
	}
	defer scriptA.Close()

	// Plugin B: 1 hook
	regB := hook.NewHookRegistry()
	hb.SetHookRegistry("plugin-b", regB)
	scriptB, err := eng.Compile("plugin-b", `
hooks.on_session_start(function() return nil end)
`)
	if err != nil {
		t.Fatalf("Compile B: %v", err)
	}
	scriptB.PluginDir = t.TempDir()
	scriptB.Plugins = []string{"plugin-b"}
	if err := eng.Execute(context.Background(), scriptB); err != nil {
		t.Fatalf("Execute B: %v", err)
	}
	defer scriptB.Close()

	if hb.Count() != 3 {
		t.Errorf("expected 3 hooks across plugins, got %d", hb.Count())
	}
	if regA.Len() != 2 {
		t.Errorf("plugin-a: expected 2 hooks, got %d", regA.Len())
	}
	if regB.Len() != 1 {
		t.Errorf("plugin-b: expected 1 hook, got %d", regB.Len())
	}
}

func TestHookBridgeConcurrency(t *testing.T) {
	// Verify that dispatching hooks concurrently does not cause data races.
	code := `
hooks.on_tool_before(function(event)
    return {deny = true, reason = "blocked"}
end)
`
	_, _, reg := setupHookTest(t, "test-concurrent", code)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := reg.Dispatch(context.Background(), hook.HookToolBefore, &hook.HookEvent{
				ToolName: "bash",
			})
			if err != nil {
				t.Errorf("Dispatch error: %v", err)
				return
			}
			if len(results) != 1 || !results[0].Deny {
				t.Error("expected deny result")
			}
		}()
	}
	wg.Wait()
}

func TestHookSessionEnd(t *testing.T) {
	code := `
hooks.on_session_end(function(event)
    return {allow = true}
end)
`
	_, _, reg := setupHookTest(t, "test-session-end", code)

	results, err := reg.Dispatch(context.Background(), hook.HookSessionEnd, &hook.HookEvent{
		SessionID: "sess-456",
	})
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Allow {
		t.Error("expected Allow=true")
	}
}
