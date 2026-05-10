package lua

import (
	"time"

	lua "github.com/yuin/gopher-lua"
)

// getVM borrows a VM from the pool or creates a new sandboxed one.
func (e *Engine) getVM() *lua.LState {
	select {
	case L := <-e.pool:
		return L
	default:
		return e.newSandboxedVM()
	}
}

// putVM returns a VM to the pool. If the pool is full, the VM is closed.
func (e *Engine) putVM(L *lua.LState) {
	if L == nil {
		return
	}
	select {
	case e.pool <- L:
	default:
		L.Close()
	}
}

// newSandboxedVM creates a fresh sandboxed LState with safe modules.
func (e *Engine) newSandboxedVM() *lua.LState {
	L := lua.NewState(lua.Options{
		SkipOpenLibs:  true,
		RegistrySize:  e.opts.RegistrySize,
		CallStackSize: e.opts.CallStackSize,
	})

	// Open only safe standard libraries
	lua.OpenBase(L)
	lua.OpenPackage(L)
	lua.OpenTable(L)
	lua.OpenString(L)
	lua.OpenMath(L)

	// Apply generic sandbox restrictions
	NewSandbox("").Apply(L)

	// Register generic safe modules (available to all plugins)
	RegisterJSONModule(L)
	RegisterHTTPModule(L, time.Duration(e.opts.Timeout)*time.Second)

	return L
}
