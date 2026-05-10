package mcp

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequestMarshaling(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params: map[string]interface{}{
			"protocolVersion": MCPProtocol,
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Method  string `json:"method"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if parsed.JSONRPC != JSONRPCVersion {
		t.Fatalf("jsonrpc mismatch: %s", parsed.JSONRPC)
	}
	if parsed.Method != "initialize" {
		t.Fatalf("method mismatch: %s", parsed.Method)
	}
}

func TestJSONRPCResponseMarshaling(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Result:  json.RawMessage(`{"ok":true}`),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed JSONRPCResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if string(parsed.Result) != `{"ok":true}` {
		t.Fatalf("result mismatch: %s", string(parsed.Result))
	}
	if parsed.Error != nil {
		t.Fatal("expected no error")
	}
}

func TestJSONRPCError(t *testing.T) {
	e := &JSONRPCError{Code: -32601, Message: "Method not found"}
	if e.Error() != "jsonrpc error -32601: Method not found" {
		t.Fatalf("unexpected error string: %s", e.Error())
	}
}

func TestJSONRPCNotificationMarshaling(t *testing.T) {
	notif := JSONRPCNotification{
		JSONRPC: JSONRPCVersion,
		Method:  "notifications/initialized",
		Params:  map[string]interface{}{},
	}

	data, err := json.Marshal(notif)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Notification should have no ID field.
	var raw struct {
		ID *int64 `json:"id"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if raw.ID != nil {
		t.Fatal("notification should not have an id field")
	}
}

func TestManagerAddRemove(t *testing.T) {
	mgr := NewManager()
	if len(mgr.List()) != 0 {
		t.Fatal("expected empty manager")
	}

	if _, ok := mgr.Get("test"); ok {
		t.Fatal("expected not found")
	}

	mgr.Remove("nonexistent") // should not panic
	mgr.Close()               // should not panic
}

func TestStripCRLF(t *testing.T) {
	tests := []struct {
		input    []byte
		expected []byte
	}{
		{[]byte("hello"), []byte("hello")},
		{[]byte("hello\n"), []byte("hello")},
		{[]byte("hello\r\n"), []byte("hello")},
		{[]byte("hello\r"), []byte("hello")},
		{[]byte("\n"), []byte("")},
	}

	for _, tt := range tests {
		got := stripCRLF(tt.input)
		if string(got) != string(tt.expected) {
			t.Fatalf("stripCRLF(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNewManager(t *testing.T) {
	mgr := NewManager()
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}
