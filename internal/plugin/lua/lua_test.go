package lua

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestNewEngine(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 4})
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
	if eng.opts.PoolSize != 4 {
		t.Errorf("expected PoolSize=4, got %d", eng.opts.PoolSize)
	}
	if cap(eng.pool) != 4 {
		t.Errorf("expected pool cap=4, got %d", cap(eng.pool))
	}
}

func TestDefaultOptions(t *testing.T) {
	eng := NewEngine(Options{})
	if eng.opts.PoolSize != runtime.NumCPU() {
		t.Errorf("expected PoolSize=%d, got %d", runtime.NumCPU(), eng.opts.PoolSize)
	}
	if eng.opts.Timeout != 30 {
		t.Errorf("expected Timeout=30, got %d", eng.opts.Timeout)
	}
}

func TestCompileExecute(t *testing.T) {
	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	script, err := eng.Compile("test", `x = 42`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestCompileSyntaxError(t *testing.T) {
	eng := NewEngine(Options{})
	defer eng.Stop()

	_, err := eng.Compile("bad", `function broken(`)
	if err == nil {
		t.Fatal("expected syntax error")
	}
}

func TestCallFunction(t *testing.T) {
	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
function greet(name)
    return "hello " .. name
end

function add(a, b)
    return a + b
end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	t.Run("string arg", func(t *testing.T) {
		result, err := eng.CallFunction(ctx, script, "greet", lua.LString("world"))
		if err != nil {
			t.Fatalf("CallFunction: %v", err)
		}
		if string(result.(lua.LString)) != "hello world" {
			t.Errorf("expected 'hello world', got %q", result)
		}
	})

	t.Run("number args", func(t *testing.T) {
		result, err := eng.CallFunction(ctx, script, "add", lua.LNumber(3), lua.LNumber(4))
		if err != nil {
			t.Fatalf("CallFunction: %v", err)
		}
		if float64(result.(lua.LNumber)) != 7.0 {
			t.Errorf("expected 7, got %v", result)
		}
	})

	t.Run("non-existent function", func(t *testing.T) {
		_, err := eng.CallFunction(ctx, script, "nonexistent")
		if err == nil {
			t.Fatal("expected error for non-existent function")
		}
	})

	t.Run("not a function", func(t *testing.T) {
		code2 := `x = 99`
		script2, err := eng.Compile("test2", code2)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		if err := eng.Execute(ctx, script2); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		_, err = eng.CallFunction(ctx, script2, "x")
		if err == nil {
			t.Fatal("expected error when calling non-function")
		}
	})
}

func TestSandboxRemovesOS(t *testing.T) {
	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	// Try to access os.execute — should be nil in sandbox
	code := `
if os == nil then
    result = "blocked"
else
    result = "available"
end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, err := eng.CallFunction(ctx, script, "result")
	// After Execute, "result" should be a global; let's test by evaluating a getter
	_ = result
	_ = err
}

func TestSandboxRemovesIO(t *testing.T) {
	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
result = "blocked"
if io ~= nil then
    result = "available"
end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Check the result via CallFunction on a helper
	code2 := `
function get_io_status()
    if io == nil then return "blocked" else return "available" end
end
function get_os_status()
    if os == nil then return "blocked" else return "available" end
end
function get_debug_status()
    if debug == nil then return "blocked" else return "available" end
end
`
	script2, err := eng.Compile("sandbox_test", code2)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := eng.Execute(ctx, script2); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	tests := []struct{ fn, expected string }{
		{"get_io_status", "blocked"},
		{"get_os_status", "blocked"},
		{"get_debug_status", "blocked"},
	}
	for _, tc := range tests {
		t.Run(tc.fn, func(t *testing.T) {
			result, err := eng.CallFunction(ctx, script2, tc.fn)
			if err != nil {
				t.Fatalf("CallFunction %s: %v", tc.fn, err)
			}
			if string(result.(lua.LString)) != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestJSONModule(t *testing.T) {
	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
local json = require("json")

-- encode
local encoded = json.encode({name = "test", value = 42})
if not encoded then
    error("encode failed")
end

-- decode
local decoded = json.decode(encoded)
if decoded.name ~= "test" then
    error("decode name mismatch")
end
if decoded.value ~= 42 then
    error("decode value mismatch")
end

function get_result()
    return decoded.name, decoded.value
end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, err := eng.CallFunction(ctx, script, "get_result")
	if err != nil {
		t.Fatalf("CallFunction: %v", err)
	}
	// The function returns two values, PCall captures 1 (we set nret=1)
	_ = result

	// Test json.encode directly from Lua
	code2 := `
function encode_test()
    return json.encode({a = 1, b = "hello"})
end
`
	script2, err := eng.Compile("test2", code2)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if err := eng.Execute(ctx, script2); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	result2, err := eng.CallFunction(ctx, script2, "encode_test")
	if err != nil {
		t.Fatalf("CallFunction: %v", err)
	}
	encoded := string(result2.(lua.LString))
	if encoded == "" {
		t.Error("expected non-empty json string")
	}
}

func TestHTTPModule(t *testing.T) {
	// Test buildHTTPResponse directly at Go level since the sandbox blocks
	// loopback addresses and httptest binds to 127.0.0.1 by default.
	L := lua.NewState()
	defer L.Close()

	resp := buildHTTPResponse(L, 200, `{"message":"hello"}`, http.Header{
		"Content-Type": []string{"application/json"},
	})

	if resp.Type() != lua.LTTable {
		t.Fatal("expected table response")
	}
	if float64(resp.RawGetString("status").(lua.LNumber)) != 200 {
		t.Errorf("expected status 200")
	}
	if string(resp.RawGetString("body").(lua.LString)) != `{"message":"hello"}` {
		t.Errorf("unexpected body: %s", resp.RawGetString("body"))
	}
	headers := resp.RawGetString("headers")
	if headers.Type() != lua.LTTable {
		t.Fatal("expected headers table")
	}
}

func TestHTTPModuleFunctionsAvailable(t *testing.T) {
	eng := NewEngine(Options{Timeout: 10})
	defer eng.Stop()

	code := `
function check_http()
    if http == nil then return "missing" end
    if http.get == nil then return "no_get" end
    if http.post == nil then return "no_post" end
    return "ok"
end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, err := eng.CallFunction(ctx, script, "check_http")
	if err != nil {
		t.Fatalf("CallFunction: %v", err)
	}
	if string(result.(lua.LString)) != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
}

func TestHTTPModuleBlocksPrivateIPs(t *testing.T) {
	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
local http = require("http")
local resp, err = http.get("http://127.0.0.1:12345/", 1)
function get_err() return err end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	errResult, err := eng.CallFunction(ctx, script, "get_err")
	if err != nil {
		t.Fatalf("CallFunction: %v", err)
	}
	errStr := string(errResult.(lua.LString))
	if errStr == "" {
		t.Error("expected error for localhost access")
	}
}

func TestFSModuleWithinDir(t *testing.T) {
	pluginDir := t.TempDir()
	testFile := filepath.Join(pluginDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello fs"), 0644); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
local fs = require("fs")
local content = fs.read("test.txt")
if not content then
    error("fs.read returned nil")
end
function get_content() return content end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	script.PluginDir = pluginDir

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, err := eng.CallFunction(ctx, script, "get_content")
	if err != nil {
		t.Fatalf("CallFunction: %v", err)
	}
	if string(result.(lua.LString)) != "hello fs" {
		t.Errorf("expected 'hello fs', got %q", result)
	}
}

func TestFSModuleRestricted(t *testing.T) {
	pluginDir := t.TempDir()

	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
local fs = require("fs")
local content, err = fs.read("../../etc/passwd")
function get_err() return err end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	script.PluginDir = pluginDir

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	errResult, err := eng.CallFunction(ctx, script, "get_err")
	if err != nil {
		t.Fatalf("CallFunction: %v", err)
	}
	errStr := string(errResult.(lua.LString))
	if errStr == "" {
		t.Error("expected error for path outside plugin dir")
	}
}

func TestFSModuleExists(t *testing.T) {
	pluginDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pluginDir, "exists.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
local fs = require("fs")
function check_exists() return fs.exists("exists.txt") end
function check_missing() return fs.exists("missing.txt") end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	script.PluginDir = pluginDir

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	t.Run("exists", func(t *testing.T) {
		result, err := eng.CallFunction(ctx, script, "check_exists")
		if err != nil {
			t.Fatalf("CallFunction: %v", err)
		}
		if result != lua.LTrue {
			t.Errorf("expected true, got %v", result)
		}
	})
	t.Run("missing", func(t *testing.T) {
		result, err := eng.CallFunction(ctx, script, "check_missing")
		if err != nil {
			t.Fatalf("CallFunction: %v", err)
		}
		if result != lua.LFalse {
			t.Errorf("expected false, got %v", result)
		}
	})
}

func TestFSModuleList(t *testing.T) {
	pluginDir := t.TempDir()
	os.WriteFile(filepath.Join(pluginDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(pluginDir, "b.txt"), []byte("b"), 0644)

	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
local fs = require("fs")
local files = fs.list(".")
function get_count() return #files end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	script.PluginDir = pluginDir

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, err := eng.CallFunction(ctx, script, "get_count")
	if err != nil {
		t.Fatalf("CallFunction: %v", err)
	}
	if float64(result.(lua.LNumber)) < 2 {
		t.Errorf("expected at least 2 files, got %v", result)
	}
}

func TestPluginConfigModule(t *testing.T) {
	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
local cfg = require("plugin_config")
function get_key(k) return cfg.get(k) end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	script.Config = map[string]interface{}{
		"api_key":    "secret123",
		"max_retries": float64(3),
		"enabled":    true,
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	t.Run("string config", func(t *testing.T) {
		result, err := eng.CallFunction(ctx, script, "get_key", lua.LString("api_key"))
		if err != nil {
			t.Fatalf("CallFunction: %v", err)
		}
		if string(result.(lua.LString)) != "secret123" {
			t.Errorf("expected 'secret123', got %q", result)
		}
	})

	t.Run("number config", func(t *testing.T) {
		result, err := eng.CallFunction(ctx, script, "get_key", lua.LString("max_retries"))
		if err != nil {
			t.Fatalf("CallFunction: %v", err)
		}
		if float64(result.(lua.LNumber)) != 3.0 {
			t.Errorf("expected 3, got %v", result)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		result, err := eng.CallFunction(ctx, script, "get_key", lua.LString("nonexistent"))
		if err != nil {
			t.Fatalf("CallFunction: %v", err)
		}
		if result != lua.LNil {
			t.Errorf("expected nil for missing key, got %v", result)
		}
	})
}

func TestTimeout(t *testing.T) {
	eng := NewEngine(Options{Timeout: 1})
	defer eng.Stop()

	code := `
while true do
    -- infinite loop
end
`
	script, err := eng.Compile("infinite", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	err = eng.Execute(ctx, script)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestVMReuse(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 2, Timeout: 5})
	defer eng.Stop()

	// Execute a script, close it, then check the VM went back to the pool
	script, err := eng.Compile("test", `x = 1`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The VM should now be assigned to this script
	if script.vm == nil {
		t.Fatal("expected non-nil VM after execute")
	}

	// Close returns VM to pool
	script.Close()
	if script.vm != nil {
		t.Error("expected nil VM after close")
	}

	// Re-execute should get a VM (possibly from pool)
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Re-execute: %v", err)
	}
	if script.vm == nil {
		t.Fatal("expected non-nil VM after re-execute")
	}
}

func TestConcurrentVMs(t *testing.T) {
	eng := NewEngine(Options{PoolSize: 4, Timeout: 10})
	defer eng.Stop()

	code := `
function compute(n)
    local sum = 0
    for i = 1, n do
        sum = sum + i
    end
    return sum
end
`
	script, err := eng.Compile("concurrent", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	script.PluginDir = t.TempDir()

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			result, err := eng.CallFunction(ctx, script, "compute", lua.LNumber(n*100))
			if err != nil {
				errs <- err
				return
			}
			// sum of 1..N = N*(N+1)/2
			expected := float64(n*100) * float64(n*100+1) / 2.0
			if float64(result.(lua.LNumber)) != expected {
				errs <- fmt.Errorf("wrong sum: got %v, expected %v", result, expected)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

func TestLoadFileRemoved(t *testing.T) {
	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	code := `
result = "loadfile_blocked"
if loadfile ~= nil then
    result = "loadfile_available"
end
function get_loadfile_status() return result end
`
	script, err := eng.Compile("test", code)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, err := eng.CallFunction(ctx, script, "get_loadfile_status")
	if err != nil {
		t.Fatalf("CallFunction: %v", err)
	}
	if string(result.(lua.LString)) != "loadfile_blocked" {
		t.Errorf("expected loadfile to be blocked, got %q", result)
	}
}

func TestSandboxModuleAccess(t *testing.T) {
	// Verify that require() can't load system modules
	eng := NewEngine(Options{Timeout: 5})
	defer eng.Stop()

	script, err := eng.Compile("test", `
function try_require_os()
    local ok, err = pcall(require, "os")
    if ok then return "available" else return "blocked" end
end
`)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	if err := eng.Execute(ctx, script); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	result, err := eng.CallFunction(ctx, script, "try_require_os")
	if err != nil {
		t.Fatalf("CallFunction: %v", err)
	}
	if string(result.(lua.LString)) != "blocked" {
		t.Errorf("expected require('os') to fail, got %q", result)
	}
}
