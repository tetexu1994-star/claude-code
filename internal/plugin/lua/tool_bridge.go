package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
	"github.com/tetexu/tlaude-code/internal/tool"
)

// ToolBridge connects Lua-registered tools to the Go tool system.
type ToolBridge struct {
	mu     sync.RWMutex
	engine *Engine
	tools  map[string]*luaTool
}

// luaTool implements tool.Tool for a Lua-defined tool.
type luaTool struct {
	compiled    *CompiledScript
	handlerName string
	def         tool.ToolDefinition
}

// NewToolBridge creates a tool bridge backed by the given Lua engine.
func NewToolBridge(engine *Engine) *ToolBridge {
	return &ToolBridge{
		engine: engine,
		tools:  make(map[string]*luaTool),
	}
}

// RegisterTool stores a tool definition from Lua. Returns an error on
// duplicate names. When called from a pool VM during handler execution,
// duplicates are silently ignored (the proto re-execution re-registers).
func (tb *ToolBridge) RegisterTool(compiled *CompiledScript, def tool.ToolDefinition, handlerName string) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if def.Name == "" {
		return fmt.Errorf("tool name must not be empty")
	}
	if handlerName == "" {
		return fmt.Errorf("handler name must not be empty")
	}

	if _, exists := tb.tools[def.Name]; exists {
		// Duplicate registration is harmless — happens when the proto
		// is re-executed on a pool VM during handler invocation.
		return nil
	}

	tb.tools[def.Name] = &luaTool{
		compiled:    compiled,
		handlerName: handlerName,
		def:         def,
	}
	return nil
}

// GetGoTools returns all registered Lua tools as tool.Tool instances.
func (tb *ToolBridge) GetGoTools() []tool.Tool {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	result := make([]tool.Tool, 0, len(tb.tools))
	for _, lt := range tb.tools {
		result = append(result, lt)
	}
	return result
}

// Count returns the number of registered Lua tools.
func (tb *ToolBridge) Count() int {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return len(tb.tools)
}

// tool.Tool interface implementation for luaTool.

func (lt *luaTool) Name() string                   { return lt.def.Name }
func (lt *luaTool) Description() string             { return lt.def.Description }
func (lt *luaTool) ToolDefinition() tool.ToolDefinition { return lt.def }
func (lt *luaTool) IsEnabled() bool                 { return true }
func (lt *luaTool) IsConcurrencySafe() bool         { return true }

func (lt *luaTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
	// Parse input JSON into args map.
	var args map[string]interface{}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return &tool.ToolResult{IsError: true, Content: fmt.Sprintf("invalid args: %v", err)}, nil
		}
	}

	// Borrow a VM from the pool.
	L := lt.compiled.engine.getVM()
	defer lt.compiled.engine.putVM(L)

	// Register plugin-specific modules.
	RegisterFSModule(L, lt.compiled.PluginDir)
	RegisterPluginConfigModule(L, lt.compiled.Config)
	if lt.compiled.engine.toolBridge != nil {
		RegisterToolAPI(L, lt.compiled, lt.compiled.engine.toolBridge)
	}

	// Apply timeout.
	tctx, cancel := context.WithTimeout(ctx, time.Duration(lt.compiled.engine.opts.Timeout)*time.Second)
	defer cancel()
	L.SetContext(tctx)

	// Run the proto to define all functions (including the handler).
	// This also re-calls tools.register(), which is a harmless no-op.
	fn := L.NewFunctionFromProto(lt.compiled.Proto)
	L.Push(fn)
	if err := L.PCall(0, lua.MultRet, nil); err != nil {
		return &tool.ToolResult{IsError: true, Content: fmt.Sprintf("script error: %v", err)}, nil
	}

	// Look up and call the handler function.
	handlerFn := L.GetGlobal(lt.handlerName)
	if handlerFn.Type() != lua.LTFunction {
		return &tool.ToolResult{IsError: true, Content: fmt.Sprintf("handler %q not found (got %s)", lt.handlerName, handlerFn.Type())}, nil
	}

	L.Push(handlerFn)
	L.Push(goToLua(L, args))
	if err := L.PCall(1, 1, nil); err != nil {
		return &tool.ToolResult{IsError: true, Content: fmt.Sprintf("handler error: %v", err)}, nil
	}

	result := L.Get(-1)
	L.Pop(1)

	return parseLuaToolResult(result)
}

// parseLuaToolResult converts a Lua handler return value into a ToolResult.
func parseLuaToolResult(v lua.LValue) (*tool.ToolResult, error) {
	tbl, ok := v.(*lua.LTable)
	if !ok {
		// Handler returned a non-table; treat as content string.
		return &tool.ToolResult{Content: v.String()}, nil
	}

	content := ""
	if cv := tbl.RawGetString("content"); cv.Type() != lua.LTNil {
		content = cv.String()
	}

	isError := false
	if ev := tbl.RawGetString("is_error"); ev.Type() != lua.LTNil {
		if b, ok := ev.(lua.LBool); ok {
			isError = bool(b)
		}
	}

	return &tool.ToolResult{
		Content: content,
		IsError: isError,
	}, nil
}
