package lua

import (
	"context"
	"fmt"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
	"github.com/tetexu/tlaude-code/internal/hook"
)

// HookBridge connects Lua-registered hooks to the Go hook.HookRegistry.
type HookBridge struct {
	mu         sync.RWMutex
	engine     *Engine
	registries map[string]*hook.HookRegistry // plugin name → its hook registry
}

// NewHookBridge creates a hook bridge backed by the given Lua engine.
func NewHookBridge(engine *Engine) *HookBridge {
	return &HookBridge{
		engine:     engine,
		registries: make(map[string]*hook.HookRegistry),
	}
}

// SetHookRegistry assigns a HookRegistry for the named plugin. Subsequent
// calls to RegisterHook will install handlers into this registry.
func (hb *HookBridge) SetHookRegistry(pluginName string, registry *hook.HookRegistry) {
	hb.mu.Lock()
	defer hb.mu.Unlock()
	hb.registries[pluginName] = registry
}

// HookRegistry returns the HookRegistry for the named plugin, or nil.
func (hb *HookBridge) HookRegistry(pluginName string) *hook.HookRegistry {
	hb.mu.RLock()
	defer hb.mu.RUnlock()
	return hb.registries[pluginName]
}

// RegisterHook registers a Lua function as a hook handler. It creates a Go
// HookHandler wrapper and installs it into the HookRegistry for each plugin
// that owns the compiled script.
func (hb *HookBridge) RegisterHook(compiled *CompiledScript, point string, handlerName string) error {
	hb.mu.RLock()
	defer hb.mu.RUnlock()

	handler := hb.makeHandler(compiled, handlerName)

	for _, pluginName := range compiled.Plugins {
		if registry, ok := hb.registries[pluginName]; ok {
			registry.Register(hook.HookPoint(point), handlerName, handler)
		}
	}
	return nil
}

// Count returns total registered hooks across all plugins.
func (hb *HookBridge) Count() int {
	hb.mu.RLock()
	defer hb.mu.RUnlock()

	total := 0
	for _, registry := range hb.registries {
		total += registry.Len()
	}
	return total
}

// makeHandler creates a Go HookHandler that calls the named Lua function.
func (hb *HookBridge) makeHandler(compiled *CompiledScript, handlerName string) hook.HookHandler {
	return func(ctx context.Context, event *hook.HookEvent) (*hook.HookResult, error) {
		L := hb.engine.getVM()
		defer hb.engine.putVM(L)

		// Register plugin-specific modules so the handler has access.
		RegisterFSModule(L, compiled.PluginDir)
		RegisterPluginConfigModule(L, compiled.Config)
		if hb.engine.toolBridge != nil {
			RegisterToolAPI(L, compiled, hb.engine.toolBridge)
		}
		// Register hook API with nil bridge so hooks.on_*() calls during
		// proto re-execution store handler functions as globals without
		// re-registering into the hook registry.
		RegisterHookAPI(L, compiled, nil)

		// Apply timeout.
		tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		L.SetContext(tctx)

		// Re-run proto to define all functions.
		fn := L.NewFunctionFromProto(compiled.Proto)
		L.Push(fn)
		if err := L.PCall(0, lua.MultRet, nil); err != nil {
			return nil, fmt.Errorf("hook script error: %w", err)
		}

		// Look up handler.
		handlerFn := L.GetGlobal(handlerName)
		if handlerFn.Type() != lua.LTFunction {
			return nil, nil // handler not found, pass through
		}

		// Build event table and call handler.
		eventTable := buildEventTable(L, event)

		L.Push(handlerFn)
		L.Push(eventTable)
		if err := L.PCall(1, 1, nil); err != nil {
			return nil, nil // handler error, pass through
		}

		result := L.Get(-1)
		L.Pop(1)
		return parseHookResult(result), nil
	}
}

// buildEventTable converts a HookEvent into a Lua table.
func buildEventTable(L *lua.LState, event *hook.HookEvent) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("tool_name", lua.LString(event.ToolName))
	t.RawSetString("session_id", lua.LString(event.SessionID))
	t.RawSetString("message", lua.LString(event.Message))
	if event.Args != nil {
		t.RawSetString("args", goToLua(L, event.Args))
	}
	return t
}

// parseHookResult converts a Lua handler return value into a HookResult.
func parseHookResult(v lua.LValue) *hook.HookResult {
	if v.Type() == lua.LTNil {
		return nil // pass through
	}

	tbl, ok := v.(*lua.LTable)
	if !ok {
		return nil // pass through
	}

	result := &hook.HookResult{}

	if deny := tbl.RawGetString("deny"); deny.Type() == lua.LTBool {
		result.Deny = bool(deny.(lua.LBool))
	}
	if allow := tbl.RawGetString("allow"); allow.Type() == lua.LTBool {
		result.Allow = bool(allow.(lua.LBool))
	}
	if reason := tbl.RawGetString("reason"); reason.Type() == lua.LTString {
		result.Reason = string(reason.(lua.LString))
	}
	if modify := tbl.RawGetString("modify"); modify.Type() == lua.LTTable {
		if m, ok := luaToGo(modify).(map[string]interface{}); ok && len(m) > 0 {
			result.Modify = m
		}
	}

	return result
}
