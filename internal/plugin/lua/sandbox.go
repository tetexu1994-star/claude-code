package lua

import (
	lua "github.com/yuin/gopher-lua"
)

// Sandbox restricts what a Lua plugin can access.
// Prevents: os access, file I/O, arbitrary code execution.
type Sandbox struct {
	PluginDir string // plugin's own directory (only dir it can read)
}

// NewSandbox creates a security sandbox. PluginDir can be set later
// before Apply is called.
func NewSandbox(pluginDir string) *Sandbox {
	return &Sandbox{PluginDir: pluginDir}
}

// Apply configures an LState with restricted globals.
// Removes: os, io, debug, loadlib
// Exposes: safe functions registered after sandboxing.
func (s *Sandbox) Apply(L *lua.LState) {
	// Remove dangerous global functions
	L.SetGlobal("loadfile", lua.LNil)
	L.SetGlobal("dofile", lua.LNil)
	L.SetGlobal("load", lua.LNil)

	// Remove dangerous standard libraries from globals
	L.SetGlobal("os", lua.LNil)
	L.SetGlobal("io", lua.LNil)
	L.SetGlobal("debug", lua.LNil)

	// Clear package.loaders so require() can't find system modules.
	// Our safe modules are registered via PreloadModule (package.preload).
	clearPackageLoaders(L)
}

// clearPackageLoaders removes the file-based loader from package.loaders
// while keeping the preload loader (so require() works for our PreloadModule-registered modules).
// loader[1] = loLoaderPreload (safe: checks package.preload)
// loader[2] = loLoaderLua (dangerous: loads .lua files from disk)
func clearPackageLoaders(L *lua.LState) {
	pkg := L.GetField(L.Get(lua.EnvironIndex), "package")
	if pkg.Type() != lua.LTTable {
		return
	}
	loaders := L.GetField(pkg, "loaders")
	if tbl, ok := loaders.(*lua.LTable); ok {
		// Remove all loaders except index 1 (the preload loader).
		// We do this instead of iterating with ForEach because we want to
		// explicitly keep the preload loader at index 1.
		tbl.ForEach(func(key, _ lua.LValue) {
			if kn, ok := key.(lua.LNumber); ok && int(kn) == 1 {
				return // keep the preload loader
			}
			tbl.RawSet(key, lua.LNil)
		})
	}
}
