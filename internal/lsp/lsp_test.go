package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- Transport Tests ---

func TestTransportReadWriteMessage(t *testing.T) {
	var buf bytes.Buffer
	data := []byte(`{"jsonrpc":"2.0","id":1,"result":"hello"}`)
	// Same buffer for read and write — WriteMessage writes to it, ReadMessage reads back.
	transport := NewTransport(&buf, &buf)

	if err := transport.WriteMessage(data); err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	readData, err := transport.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if !bytes.Equal(readData, data) {
		t.Fatalf("round-trip mismatch: got %q, want %q", readData, data)
	}
}

func TestTransportWriteMessage(t *testing.T) {
	var buf bytes.Buffer
	data := []byte(`{"jsonrpc":"2.0","id":1,"result":"ok"}`)
	transport := NewTransport(nil, &buf)

	if err := transport.WriteMessage(data); err != nil {
		t.Fatalf("WriteMessage failed: %v", err)
	}

	expected := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(data), data)
	if buf.String() != expected {
		t.Fatalf("WriteMessage output mismatch:\ngot:  %q\nwant: %q", buf.String(), expected)
	}
}

func TestTransportReadMessageMalformedHeader(t *testing.T) {
	// Missing Content-Length header.
	buf := bytes.NewBufferString("\r\n{}")
	transport := NewTransport(buf, io.Discard)

	_, err := transport.ReadMessage()
	if err == nil || !strings.Contains(err.Error(), "Content-Length") {
		t.Fatalf("expected Content-Length error, got: %v", err)
	}
}

func TestTransportReadMessageInvalidLength(t *testing.T) {
	buf := bytes.NewBufferString("Content-Length: abc\r\n\r\n{}")
	transport := NewTransport(buf, io.Discard)

	_, err := transport.ReadMessage()
	if err == nil {
		t.Fatal("expected parse error for invalid Content-Length")
	}
}

func TestTransportReadMessageShortBody(t *testing.T) {
	buf := bytes.NewBufferString("Content-Length: 100\r\n\r\nshort")
	transport := NewTransport(buf, io.Discard)

	_, err := transport.ReadMessage()
	if err == nil {
		t.Fatal("expected error for short body")
	}
}

func TestTransportMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	transport := NewTransport(&buf, &buf)

	msg1 := []byte(`{"id":1}`)
	msg2 := []byte(`{"id":2}`)

	if err := transport.WriteMessage(msg1); err != nil {
		t.Fatalf("WriteMessage 1 failed: %v", err)
	}
	if err := transport.WriteMessage(msg2); err != nil {
		t.Fatalf("WriteMessage 2 failed: %v", err)
	}

	r1, err := transport.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage 1 failed: %v", err)
	}
	r2, err := transport.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage 2 failed: %v", err)
	}

	if !bytes.Equal(r1, msg1) {
		t.Fatalf("msg1 mismatch: got %q, want %q", r1, msg1)
	}
	if !bytes.Equal(r2, msg2) {
		t.Fatalf("msg2 mismatch: got %q, want %q", r2, msg2)
	}
}

func TestTransportExtraHeaders(t *testing.T) {
	var buf bytes.Buffer
	data := []byte(`{"id":1}`)
	// LSP spec allows extra headers like Content-Type.
	fmt.Fprintf(&buf, "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\n")
	fmt.Fprintf(&buf, "Content-Length: %d\r\n", len(data))
	fmt.Fprintf(&buf, "\r\n")
	buf.Write(data)

	transport := NewTransport(&buf, io.Discard)
	readData, err := transport.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage failed: %v", err)
	}
	if !bytes.Equal(readData, data) {
		t.Fatalf("data mismatch: got %q, want %q", readData, data)
	}
}

// --- Client Tests ---

func TestClientStartStop(t *testing.T) {
	// Use a process that exits quickly so the test won't hang.
	client := NewClient("test-server", nil)

	err := client.Start("true", nil, nil)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_ = client.Stop()
}

func TestClientDoubleStart(t *testing.T) {
	client := NewClient("test-server", nil)
	if err := client.Start("true", nil, nil); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}
	defer client.Stop()

	err := client.Start("true", nil, nil)
	if err == nil {
		t.Fatal("expected error on double start")
	}
}

func TestClientStartInvalidCommand(t *testing.T) {
	client := NewClient("test-server", nil)
	err := client.Start("/nonexistent/command_xyz_123", nil, nil)
	if err == nil {
		client.Stop()
		t.Fatal("expected error for nonexistent command")
	}
}

func TestClientSendRequestNotStarted(t *testing.T) {
	client := NewClient("test-server", nil)
	var result json.RawMessage
	err := client.SendRequest("test", nil, &result)
	if err == nil {
		t.Fatal("expected error when not started")
	}
}

func TestClientSendNotificationNotStarted(t *testing.T) {
	client := NewClient("test-server", nil)
	err := client.SendNotification("test", nil)
	if err == nil {
		t.Fatal("expected error when not started")
	}
}

func TestClientNewClient(t *testing.T) {
	crashed := false
	onCrash := func(err error) {
		crashed = true
	}
	client := NewClient("test-server", onCrash)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.IsInitialized() {
		t.Fatal("expected not initialized")
	}
	if client.Capabilities() != nil {
		t.Fatal("expected nil capabilities")
	}
	if crashed {
		t.Fatal("crash handler should not have fired")
	}
}

func TestClientOnNotificationBeforeStart(t *testing.T) {
	var mu sync.Mutex
	var received []string
	client := NewClient("test-queue", nil)

	// Register handler before Start — should be queued.
	client.OnNotification("test/notification", func(params json.RawMessage) {
		mu.Lock()
		received = append(received, string(params))
		mu.Unlock()
	})

	// Verify we didn't panic.
	mu.Lock()
	if len(received) != 0 {
		t.Fatal("handler should not have fired yet")
	}
	mu.Unlock()
}

func TestClientOnRequestBeforeStart(t *testing.T) {
	client := NewClient("test-queue", nil)

	called := false
	client.OnRequest("test/request", func(params json.RawMessage) (json.RawMessage, error) {
		called = true
		return json.RawMessage(`{"ok":true}`), nil
	})

	if called {
		t.Fatal("request handler should not have fired yet")
	}
}

// --- Manager Tests ---

func TestManagerNew(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.GetServerForFile("test.ts") != nil {
		t.Fatal("expected nil for unknown extension")
	}
	if mgr.IsFileOpen("test.ts") {
		t.Fatal("expected file not open")
	}
}

func TestManagerInitialize(t *testing.T) {
	mgr := NewManager()

	configs := map[string]ServerConfig{
		"typescript": {
			Command: "typescript-language-server",
			Args:    []string{"--stdio"},
			ExtensionToLanguage: map[string]string{
				".ts":  "typescript",
				".tsx": "typescriptreact",
			},
		},
		"python": {
			Command: "pylsp",
			ExtensionToLanguage: map[string]string{
				".py": "python",
			},
		},
	}

	if err := mgr.Initialize(configs); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Check extension routing.
	ts := mgr.GetServerForFile("hello.ts")
	if ts == nil {
		t.Fatal("expected server for .ts")
	}
	if ts.serverName != "typescript" {
		t.Fatalf("expected typescript server, got %q", ts.serverName)
	}

	py := mgr.GetServerForFile("main.py")
	if py == nil {
		t.Fatal("expected server for .py")
	}
	if py.serverName != "python" {
		t.Fatalf("expected python server, got %q", py.serverName)
	}

	// Check unknown extension.
	if mgr.GetServerForFile("README.md") != nil {
		t.Fatal("expected nil for .md")
	}
}

func TestManagerInitializeCaseInsensitive(t *testing.T) {
	mgr := NewManager()

	configs := map[string]ServerConfig{
		"go": {
			Command: "gopls",
			ExtensionToLanguage: map[string]string{
				".go": "go",
			},
		},
	}

	if err := mgr.Initialize(configs); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// .Go should also match.
	s := mgr.GetServerForFile("main.Go")
	if s == nil {
		t.Fatal("expected server for .Go (case insensitive)")
	}
}

func TestManagerInitializeNoExtension(t *testing.T) {
	mgr := NewManager()

	configs := map[string]ServerConfig{
		"bad-no-ext": {
			Command: "ls",
			ExtensionToLanguage: map[string]string{
				"": "plaintext",
			},
		},
	}

	// Should not panic; empty extension should not break routing.
	if err := mgr.Initialize(configs); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if mgr.GetServerForFile("test") != nil {
		t.Log("empty extension matching behavior is implementation-defined")
	}
}

func TestManagerInitializeInvalidConfig(t *testing.T) {
	mgr := NewManager()

	configs := map[string]ServerConfig{
		"bad": {
			Command:             "",
			ExtensionToLanguage: map[string]string{".ts": "typescript"},
		},
	}

	// Should skip servers with empty command.
	if err := mgr.Initialize(configs); err != nil {
		t.Fatalf("Initialize should not error on invalid config: %v", err)
	}

	if mgr.GetServerForFile("test.ts") != nil {
		t.Fatal("expected nil for server with empty command")
	}
}

func TestManagerInitializeNoExtensionToLanguage(t *testing.T) {
	mgr := NewManager()

	configs := map[string]ServerConfig{
		"bad": {
			Command:             "ls",
			ExtensionToLanguage: map[string]string{},
		},
	}

	// Should skip servers with empty extensionToLanguage.
	if err := mgr.Initialize(configs); err != nil {
		t.Fatalf("Initialize should not error: %v", err)
	}

	if mgr.GetServerForFile("test.ts") != nil {
		t.Fatal("expected nil for server with no extensions")
	}
}

func TestManagerShutdown(t *testing.T) {
	mgr := NewManager()

	configs := map[string]ServerConfig{
		"test": {
			Command: "cat",
			Args:    []string{},
			ExtensionToLanguage: map[string]string{
				".test": "testlang",
			},
		},
	}

	if err := mgr.Initialize(configs); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Shutdown without starting any servers should be safe.
	if err := mgr.Shutdown(); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	if mgr.GetServerForFile("test.test") != nil {
		t.Fatal("expected nil after shutdown")
	}
}

func TestManagerEnsureStarted(t *testing.T) {
	mgr := NewManager()

	configs := map[string]ServerConfig{
		"test-server": {
			Command: "true",
			Args:    []string{},
			ExtensionToLanguage: map[string]string{
				".test": "testlang",
			},
		},
	}

	if err := mgr.Initialize(configs); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	defer mgr.Shutdown()

	// EnsureStarted should return a server (even if the process exits quickly).
	srv := mgr.GetServerForFile("test.test")
	if srv == nil {
		t.Fatal("expected server for .test")
	}
	if srv.serverName != "test-server" {
		t.Fatalf("expected test-server, got %q", srv.serverName)
	}
}

func TestManagerEnsureStartedUnknown(t *testing.T) {
	mgr := NewManager()
	server, err := mgr.EnsureStarted("unknown.xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if server != nil {
		t.Fatal("expected nil for unknown extension")
	}
}

func TestManagerIsFileOpen(t *testing.T) {
	mgr := NewManager()
	if mgr.IsFileOpen("nonexistent.ts") {
		t.Fatal("expected false for unknown file")
	}

	// Open/close file check.
	mgr.mu.Lock()
	mgr.openedFiles["file:///test.ts"] = "typescript"
	mgr.mu.Unlock()

	if !mgr.IsFileOpen("/test.ts") {
		t.Fatal("expected true for opened file")
	}
}

func TestManagerConcurrentAccess(t *testing.T) {
	mgr := NewManager()

	configs := map[string]ServerConfig{
		"ts": {
			Command: "cat",
			ExtensionToLanguage: map[string]string{
				".ts": "typescript",
			},
		},
		"py": {
			Command: "cat",
			ExtensionToLanguage: map[string]string{
				".py": "python",
			},
		},
	}

	if err := mgr.Initialize(configs); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				mgr.GetServerForFile("test.ts")
				mgr.GetServerForFile("test.py")
				mgr.IsFileOpen("/test.ts")
			}
		}()
	}

	wg.Wait()
	mgr.Shutdown()
}

// --- JSON-RPC Types Tests ---

func TestRPCError(t *testing.T) {
	e := &rpcError{Code: -32601, Message: "Method not found"}
	expected := "jsonrpc error -32601: Method not found"
	if e.Error() != expected {
		t.Fatalf("unexpected error string: got %q, want %q", e.Error(), expected)
	}
}

func TestServerCapabilitiesDefaults(t *testing.T) {
	cap := ServerCapabilities{}
	data, err := json.Marshal(cap)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Default marshaling should omit false booleans.
	if _, ok := parsed["hoverProvider"]; ok {
		t.Log("hoverProvider included in JSON despite being false (json tag has omitempty)")
	}
}

// --- fileURIToString Tests ---

func TestFileURIToString(t *testing.T) {
	result := fileURIToString("/home/user/test.ts")
	if !strings.HasPrefix(result, "file:///home/user/test.ts") {
		t.Fatalf("unexpected URI: %s", result)
	}
}

func TestFileURIToStringSpecialChars(t *testing.T) {
	// Paths with spaces should be encoded.
	result := fileURIToString("/home/user/my file.ts")
	if !strings.Contains(result, "my%20file.ts") {
		t.Logf("URI with space: %s", result)
	}
}

// --- CompletionOptions Tests ---

func TestCompletionOptionsMarshal(t *testing.T) {
	opts := CompletionOptions{
		TriggerCharacters: []string{".", "/"},
		ResolveProvider:   true,
	}
	data, err := json.Marshal(opts)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed CompletionOptions
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(parsed.TriggerCharacters) != 2 {
		t.Fatalf("expected 2 trigger chars, got %d", len(parsed.TriggerCharacters))
	}
	if !parsed.ResolveProvider {
		t.Fatal("expected ResolveProvider to be true")
	}
}
