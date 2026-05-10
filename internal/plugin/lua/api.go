package lua

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
	"github.com/tetexu/tlaude-code/internal/tool"
)

// RegisterJSONModule exposes json.encode / json.decode to LState.
func RegisterJSONModule(L *lua.LState) {
	mod := L.NewTable()
	L.SetFuncs(mod, map[string]lua.LGFunction{
		"encode": jsonEncode,
		"decode": jsonDecode,
	})
	L.SetGlobal("json", mod)
	L.PreloadModule("json", func(L *lua.LState) int {
		t := L.NewTable()
		L.SetFuncs(t, map[string]lua.LGFunction{
			"encode": jsonEncode,
			"decode": jsonDecode,
		})
		L.Push(t)
		return 1
	})
}

func jsonEncode(L *lua.LState) int {
	val := L.CheckAny(1)
	goVal := luaToGo(val)
	data, err := json.Marshal(goVal)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(string(data)))
	return 1
}

func jsonDecode(L *lua.LState) int {
	str := L.CheckString(1)
	var result interface{}
	if err := json.Unmarshal([]byte(str), &result); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(goToLua(L, result))
	return 1
}

// RegisterHTTPModule exposes a limited http client.
// Blocks internal/private IP addresses. Default 10s timeout.
func RegisterHTTPModule(L *lua.LState, timeout time.Duration) {
	mod := L.NewTable()
	L.SetFuncs(mod, map[string]lua.LGFunction{
		"get":  makeHTTPGet(timeout),
		"post": makeHTTPPost(timeout),
	})
	L.SetGlobal("http", mod)
	L.PreloadModule("http", func(L *lua.LState) int {
		t := L.NewTable()
		L.SetFuncs(t, map[string]lua.LGFunction{
			"get":  makeHTTPGet(timeout),
			"post": makeHTTPPost(timeout),
		})
		L.Push(t)
		return 1
	})
}

func makeHTTPGet(timeout time.Duration) lua.LGFunction {
	return func(L *lua.LState) int {
		urlStr := L.CheckString(1)
		reqTimeout := timeout
		if L.GetTop() >= 2 {
			if secs, ok := L.Get(2).(lua.LNumber); ok {
				reqTimeout = time.Duration(secs) * time.Second
			}
		}

		client, err := newSafeHTTPClient(reqTimeout)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		resp, err := client.Get(urlStr)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
		L.Push(buildHTTPResponse(L, resp.StatusCode, string(body), resp.Header))
		return 1
	}
}

func makeHTTPPost(timeout time.Duration) lua.LGFunction {
	return func(L *lua.LState) int {
		urlStr := L.CheckString(1)
		postBody := L.CheckString(2)
		reqTimeout := timeout
		contentType := "application/json"

		if L.GetTop() >= 3 {
			if t, ok := L.Get(3).(*lua.LTable); ok {
				if ct := t.RawGetString("content-type"); ct.Type() == lua.LTString {
					contentType = string(ct.(lua.LString))
				}
			}
		}
		if L.GetTop() >= 4 {
			if secs, ok := L.Get(4).(lua.LNumber); ok {
				reqTimeout = time.Duration(secs) * time.Second
			}
		}

		client, err := newSafeHTTPClient(reqTimeout)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		resp, err := client.Post(urlStr, contentType, strings.NewReader(postBody))
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		L.Push(buildHTTPResponse(L, resp.StatusCode, string(body), resp.Header))
		return 1
	}
}

func buildHTTPResponse(L *lua.LState, status int, body string, headers http.Header) *lua.LTable {
	t := L.NewTable()
	t.RawSetString("status", lua.LNumber(status))
	t.RawSetString("body", lua.LString(body))

	headerTable := L.NewTable()
	for k, vals := range headers {
		headerTable.RawSetString(k, lua.LString(strings.Join(vals, ", ")))
	}
	t.RawSetString("headers", headerTable)

	return t
}

func newSafeHTTPClient(timeout time.Duration) (*http.Client, error) {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext:       safeDialContext(dialer),
		ForceAttemptHTTP2: true,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}, nil
}

func safeDialContext(dialer *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		if err := checkHostAllowed(host); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, addr)
	}
}

func checkHostAllowed(host string) error {
	// Parse as IP directly first
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("http: access to %s is blocked (internal/private IP)", host)
		}
		return nil
	}

	// Resolve hostname and check all IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("http: cannot resolve %s: %w", host, err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("http: access to %s resolves to blocked IP %s", host, ip)
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

// RegisterFSModule exposes file ops restricted to pluginDir.
func RegisterFSModule(L *lua.LState, pluginDir string) {
	mod := L.NewTable()
	L.SetFuncs(mod, map[string]lua.LGFunction{
		"read":   makeFSRead(pluginDir),
		"exists": makeFSExists(pluginDir),
		"list":   makeFSList(pluginDir),
	})
	L.SetGlobal("fs", mod)
	L.PreloadModule("fs", func(L *lua.LState) int {
		t := L.NewTable()
		L.SetFuncs(t, map[string]lua.LGFunction{
			"read":   makeFSRead(pluginDir),
			"exists": makeFSExists(pluginDir),
			"list":   makeFSList(pluginDir),
		})
		L.Push(t)
		return 1
	})
}

func safePath(pluginDir, path string) (string, error) {
	clean := filepath.Clean(filepath.Join(pluginDir, path))
	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("fs: invalid path: %w", err)
	}
	pluginAbs, err := filepath.Abs(pluginDir)
	if err != nil {
		return "", fmt.Errorf("fs: invalid plugin dir: %w", err)
	}
	if !strings.HasPrefix(abs, pluginAbs+string(filepath.Separator)) && abs != pluginAbs {
		return "", fmt.Errorf("fs: path %q is outside plugin directory", path)
	}
	return abs, nil
}

func makeFSRead(pluginDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		safe, err := safePath(pluginDir, path)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		data, err := os.ReadFile(safe)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(string(data)))
		return 1
	}
}

func makeFSExists(pluginDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		path := L.CheckString(1)
		safe, err := safePath(pluginDir, path)
		if err != nil {
			L.Push(lua.LBool(false))
			return 1
		}
		_, err = os.Stat(safe)
		L.Push(lua.LBool(err == nil))
		return 1
	}
}

func makeFSList(pluginDir string) lua.LGFunction {
	return func(L *lua.LState) int {
		path := "."
		if L.GetTop() >= 1 {
			path = L.CheckString(1)
		}
		safe, err := safePath(pluginDir, path)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		entries, err := os.ReadDir(safe)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		t := L.NewTable()
		for _, entry := range entries {
			t.Append(lua.LString(entry.Name()))
		}
		L.Push(t)
		return 1
	}
}

// RegisterPluginConfigModule exposes the plugin's config map to Lua.
func RegisterPluginConfigModule(L *lua.LState, config map[string]interface{}) {
	mod := L.NewTable()
	L.SetFuncs(mod, map[string]lua.LGFunction{
		"get": makeConfigGet(config),
	})
	L.SetGlobal("plugin_config", mod)
	L.PreloadModule("plugin_config", func(L *lua.LState) int {
		t := L.NewTable()
		L.SetFuncs(t, map[string]lua.LGFunction{
			"get": makeConfigGet(config),
		})
		L.Push(t)
		return 1
	})
}

func makeConfigGet(config map[string]interface{}) lua.LGFunction {
	return func(L *lua.LState) int {
		key := L.CheckString(1)
		if config == nil {
			L.Push(lua.LNil)
			return 1
		}
		val, ok := config[key]
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLua(L, val))
		return 1
	}
}

// luaToGo converts a Lua value to a Go value for JSON encoding.
func luaToGo(v lua.LValue) interface{} {
	switch val := v.(type) {
	case *lua.LTable:
		maxN := val.Len()
		if maxN > 0 {
			// Check if array-like
			arr := make([]interface{}, 0, maxN)
			isArray := true
			val.ForEach(func(k, v lua.LValue) {
				if kn, ok := k.(lua.LNumber); ok {
					idx := int(kn)
					if idx >= 1 && idx <= maxN {
						for len(arr) < idx {
							arr = append(arr, nil)
						}
						arr[idx-1] = luaToGo(v)
						return
					}
				}
				isArray = false
			})
			if isArray {
				return arr
			}
		}
		obj := make(map[string]interface{})
		val.ForEach(func(k, v lua.LValue) {
			obj[k.String()] = luaToGo(v)
		})
		return obj
	case lua.LBool:
		return bool(val)
	case lua.LNumber:
		return float64(val)
	case lua.LString:
		return string(val)
	default:
		return nil
	}
}

// goToLua converts a Go value to a Lua value.
func goToLua(L *lua.LState, v interface{}) lua.LValue {
	switch val := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(val)
	case float64:
		return lua.LNumber(val)
	case string:
		return lua.LString(val)
	case []interface{}:
		t := L.NewTable()
		for _, item := range val {
			t.Append(goToLua(L, item))
		}
		return t
	case map[string]interface{}:
		t := L.NewTable()
		for k, item := range val {
			t.RawSetString(k, goToLua(L, item))
		}
		return t
	default:
		// Fallback: JSON round-trip
		data, _ := json.Marshal(val)
		var generic interface{}
		json.Unmarshal(data, &generic)
		return goToLua(L, generic)
	}
}

// RegisterToolAPI exposes the tools.register() function to Lua VMs.
// Tools registered via this API are stored in the bridge.
func RegisterToolAPI(L *lua.LState, compiled *CompiledScript, bridge *ToolBridge) {
	mod := L.NewTable()
	L.SetFuncs(mod, map[string]lua.LGFunction{
		"register": makeToolRegister(compiled, bridge),
	})
	L.SetGlobal("tools", mod)
	L.PreloadModule("tools", func(L *lua.LState) int {
		t := L.NewTable()
		L.SetFuncs(t, map[string]lua.LGFunction{
			"register": makeToolRegister(compiled, bridge),
		})
		L.Push(t)
		return 1
	})
}

func makeToolRegister(compiled *CompiledScript, bridge *ToolBridge) lua.LGFunction {
	return func(L *lua.LState) int {
		if bridge == nil {
			L.RaiseError("tool bridge not available")
			return 0
		}

		tbl := L.CheckTable(1)

		name := string(tbl.RawGetString("name").(lua.LString))
		if name == "" {
			L.RaiseError("tool name is required")
			return 0
		}

		description := ""
		if dv := tbl.RawGetString("description"); dv.Type() == lua.LTString {
			description = string(dv.(lua.LString))
		}

		// Parse input_schema into json.RawMessage.
		var schemaJSON json.RawMessage
		schemaVal := tbl.RawGetString("input_schema")
		if schemaVal.Type() != lua.LTNil {
			goSchema := luaToGo(schemaVal)
			schemaBytes, err := json.Marshal(goSchema)
			if err != nil {
				L.RaiseError("invalid input_schema: %v", err)
				return 0
			}
			schemaJSON = json.RawMessage(schemaBytes)
		}

		// The handler can be a function reference or a string naming a global function.
		handlerVal := tbl.RawGetString("handler")
		var handlerName string
		switch {
		case handlerVal.Type() == lua.LTFunction:
			// Inline function — store it as a global with a canonical name.
			handlerName = "__tool_handler_" + name
			L.SetGlobal(handlerName, handlerVal)
		case handlerVal.Type() == lua.LTString:
			handlerName = string(handlerVal.(lua.LString))
		default:
			L.RaiseError("handler must be a function or a string naming a global function")
			return 0
		}

		def := tool.ToolDefinition{
			Name:        name,
			Description: description,
			InputSchema: schemaJSON,
		}

		if err := bridge.RegisterTool(compiled, def, handlerName); err != nil {
			L.RaiseError("%s", err.Error())
			return 0
		}

		L.Push(lua.LBool(true))
		return 1
	}
}

// RegisterHookAPI exposes the hooks.on_*() functions to Lua VMs.
// When bridge is nil (pool VM re-execution), functions are stored as globals
// but not registered with the bridge, so no duplicate registrations occur.
func RegisterHookAPI(L *lua.LState, compiled *CompiledScript, bridge *HookBridge) {
	counter := uint64(0)

	mod := L.NewTable()
	for _, hp := range []struct {
		luaName string
		goPoint string
	}{
		{"on_tool_before", "on_tool_before"},
		{"on_tool_after", "on_tool_after"},
		{"on_session_start", "on_session_start"},
		{"on_session_end", "on_session_end"},
		{"on_message", "on_message"},
	} {
		point := hp.goPoint
		luaName := hp.luaName
		L.SetField(mod, luaName, L.NewFunction(func(L *lua.LState) int {
			handlerFn := L.CheckFunction(1)
			counter++
			handlerName := fmt.Sprintf("__hook_%s_%d", luaName, counter)
			L.SetGlobal(handlerName, handlerFn)
			if bridge != nil {
				if err := bridge.RegisterHook(compiled, point, handlerName); err != nil {
					L.RaiseError("%s", err.Error())
					return 0
				}
			}
			return 0
		}))
	}
	L.SetGlobal("hooks", mod)
	L.PreloadModule("hooks", func(L *lua.LState) int {
		_ = L.NewTable()
		// Preload creates a fresh module; re-create minimal hook stubs.
		// Since preload is per-require, each require("hooks") gets its own table.
		// For simplicity, we expose the same global on require.
		L.Push(L.GetGlobal("hooks"))
		return 1
	})
}

// Ensure url package is used (imported for documentation/API alignment).
var _ = url.Parse
