// Package lua provides the gopher-lua embedded scripting engine for plugin execution.
package lua

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"
)

// Options configures the Lua engine.
type Options struct {
	PoolSize      int // default: runtime.NumCPU()
	Timeout       int // seconds, default: 30
	RegistrySize  int // default: lua.RegistrySize
	CallStackSize int // default: lua.CallStackSize
}

func (o *Options) defaults() {
	if o.PoolSize <= 0 {
		o.PoolSize = runtime.NumCPU()
	}
	if o.Timeout <= 0 {
		o.Timeout = 30
	}
	if o.RegistrySize <= 0 {
		o.RegistrySize = lua.RegistrySize
	}
	if o.CallStackSize <= 0 {
		o.CallStackSize = lua.CallStackSize
	}
}

// CompiledScript holds a pre-parsed Lua function proto (goroutine-safe).
type CompiledScript struct {
	Name      string
	Proto     *lua.FunctionProto
	Plugins   []string // which plugins use this script
	PluginDir string   // plugin's own directory (FS sandbox root)
	Config    map[string]interface{}

	engine *Engine
	vm     *lua.LState
	vmMu   sync.Mutex
}

// Engine manages a pool of sandboxed Lua VMs for plugin execution.
// Each VM is independent (LState is NOT goroutine-safe).
type Engine struct {
	pool       chan *lua.LState
	opts       Options
	mu         sync.RWMutex
	scripts    map[string]*CompiledScript // name → compiled proto
	toolBridge *ToolBridge
	hookBridge *HookBridge
}

// NewEngine creates a Lua engine with the given options.
func NewEngine(opts Options) *Engine {
	opts.defaults()
	return &Engine{
		pool:    make(chan *lua.LState, opts.PoolSize),
		opts:    opts,
		scripts: make(map[string]*CompiledScript),
	}
}

// Start pre-warms the VM pool by creating sandboxed VMs.
func (e *Engine) Start() error {
	for i := 0; i < e.opts.PoolSize; i++ {
		e.pool <- e.newSandboxedVM()
	}
	return nil
}

// Stop closes all VMs in the pool and in active scripts.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Close all script VMs
	for _, cs := range e.scripts {
		cs.vmMu.Lock()
		if cs.vm != nil {
			cs.vm.Close()
			cs.vm = nil
		}
		cs.vmMu.Unlock()
	}

	// Close pooled VMs
	close(e.pool)
	for L := range e.pool {
		L.Close()
	}
}

// Compile parses Lua source code into a compiled function proto.
// This is goroutine-safe (no LState needed for compilation).
func (e *Engine) Compile(name string, code string) (*CompiledScript, error) {
	chunk, err := parse.Parse(strings.NewReader(code), name)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", name, err)
	}
	proto, err := lua.Compile(chunk, name)
	if err != nil {
		return nil, fmt.Errorf("compiling %s: %w", name, err)
	}

	cs := &CompiledScript{
		Name:    name,
		Proto:   proto,
		engine:  e,
	}

	e.mu.Lock()
	e.scripts[name] = cs
	e.mu.Unlock()

	return cs, nil
}

// Execute runs the compiled script in a sandboxed VM.
// The VM is retained for subsequent CallFunction calls.
func (e *Engine) Execute(ctx context.Context, script *CompiledScript) error {
	return script.execute(ctx)
}

// CallFunction calls a named function defined by the previously executed script.
func (e *Engine) CallFunction(ctx context.Context, script *CompiledScript, fn string, args ...lua.LValue) (lua.LValue, error) {
	return script.callFunction(ctx, fn, args...)
}

func (cs *CompiledScript) execute(ctx context.Context) error {
	cs.vmMu.Lock()
	defer cs.vmMu.Unlock()

	if cs.vm == nil {
		cs.vm = cs.engine.getVM()
		// Register plugin-specific modules
		RegisterFSModule(cs.vm, cs.PluginDir)
		RegisterPluginConfigModule(cs.vm, cs.Config)
		if cs.engine.toolBridge != nil {
			RegisterToolAPI(cs.vm, cs, cs.engine.toolBridge)
		}
		if cs.engine.hookBridge != nil {
			RegisterHookAPI(cs.vm, cs, cs.engine.hookBridge)
		}
	}

	L := cs.vm

	ctx, cancel := context.WithTimeout(ctx, time.Duration(cs.engine.opts.Timeout)*time.Second)
	defer cancel()
	L.SetContext(ctx)

	fn := L.NewFunctionFromProto(cs.Proto)
	L.Push(fn)
	return L.PCall(0, lua.MultRet, nil)
}

func (cs *CompiledScript) callFunction(ctx context.Context, fnName string, args ...lua.LValue) (lua.LValue, error) {
	cs.vmMu.Lock()
	defer cs.vmMu.Unlock()

	if cs.vm == nil {
		return lua.LNil, fmt.Errorf("script %q not executed", cs.Name)
	}

	L := cs.vm

	ctx, cancel := context.WithTimeout(ctx, time.Duration(cs.engine.opts.Timeout)*time.Second)
	defer cancel()
	L.SetContext(ctx)

	fnVal := L.GetGlobal(fnName)
	if fnVal.Type() != lua.LTFunction {
		return lua.LNil, fmt.Errorf("%q is not a function (got %s)", fnName, fnVal.Type())
	}

	L.Push(fnVal)
	for _, arg := range args {
		L.Push(arg)
	}
	if err := L.PCall(len(args), 1, nil); err != nil {
		return lua.LNil, fmt.Errorf("calling %s: %w", fnName, err)
	}

	return L.Get(-1), nil
}

// Close releases the script's VM back to the engine pool.
func (cs *CompiledScript) Close() {
	cs.vmMu.Lock()
	defer cs.vmMu.Unlock()

	if cs.vm != nil {
		cs.engine.putVM(cs.vm)
		cs.vm = nil
	}
}

// PoolSize returns the configured VM pool size.
func (e *Engine) PoolSize() int {
	return e.opts.PoolSize
}

// ActiveVMs returns the number of scripts with active (busy) VMs.
func (e *Engine) ActiveVMs() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	count := 0
	for _, cs := range e.scripts {
		cs.vmMu.Lock()
		if cs.vm != nil {
			count++
		}
		cs.vmMu.Unlock()
	}
	return count
}

// SetToolBridge attaches a tool bridge to the engine so that plugins can
// register tools via tools.register().
func (e *Engine) SetToolBridge(tb *ToolBridge) {
	e.mu.Lock()
	e.toolBridge = tb
	e.mu.Unlock()
}

// SetHookBridge attaches a hook bridge to the engine so that plugins can
// register lifecycle hooks via hooks.on_*().
func (e *Engine) SetHookBridge(hb *HookBridge) {
	e.mu.Lock()
	e.hookBridge = hb
	e.mu.Unlock()
}
