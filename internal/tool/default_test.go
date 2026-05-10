package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultBashToolExecute(t *testing.T) {
	tool := &defaultBashTool{timeout: 30 * 1e9} // 30 seconds

	input := json.RawMessage(`{"command": "echo hello_default_bash"}`)
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "hello_default_bash") {
		t.Errorf("expected 'hello_default_bash' in output, got %q", result.Content)
	}
	if result.IsError {
		t.Error("expected no error for successful command")
	}
}

func TestDefaultBashToolFailedCommand(t *testing.T) {
	tool := &defaultBashTool{timeout: 30 * 1e9}

	input := json.RawMessage(`{"command": "exit 1"}`)
	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected is_error for failed command")
	}
}

func TestDefaultFileReadTool(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	tool := &defaultFileReadTool{}
	input, _ := json.Marshal(map[string]string{"path": testFile})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Content)
	}
	if result.IsError {
		t.Error("expected no error")
	}
}

func TestDefaultFileReadToolBadInput(t *testing.T) {
	tool := &defaultFileReadTool{}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{invalid}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected is_error for bad JSON input")
	}
}

func TestDefaultFileWriteTool(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "output.txt")

	tool := &defaultFileWriteTool{}
	input, _ := json.Marshal(map[string]string{
		"path":    testFile,
		"content": "written content",
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Error("expected no error")
	}

	// Verify file contents
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "written content" {
		t.Errorf("expected 'written content', got %q", string(data))
	}
}

func TestDefaultFileEditTool(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "edit_test.txt")
	os.WriteFile(testFile, []byte("Hello World"), 0644)

	tool := &defaultFileEditTool{}

	t.Run("replace once", func(t *testing.T) {
		os.WriteFile(testFile, []byte("Hello World"), 0644)
		input, _ := json.Marshal(map[string]interface{}{
			"path":        testFile,
			"old_string":  "World",
			"new_string":  "Gopher",
			"replace_all": false,
		})
		result, err := tool.Execute(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Errorf("expected no error, got: %s", result.Content)
		}
		data, _ := os.ReadFile(testFile)
		if string(data) != "Hello Gopher" {
			t.Errorf("expected 'Hello Gopher', got %q", string(data))
		}
	})

	t.Run("replace all", func(t *testing.T) {
		os.WriteFile(testFile, []byte("foo bar foo"), 0644)
		input, _ := json.Marshal(map[string]interface{}{
			"path":        testFile,
			"old_string":  "foo",
			"new_string":  "baz",
			"replace_all": true,
		})
		result, err := tool.Execute(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Errorf("expected no error, got: %s", result.Content)
		}
		data, _ := os.ReadFile(testFile)
		if string(data) != "baz bar baz" {
			t.Errorf("expected 'baz bar baz', got %q", string(data))
		}
	})

	t.Run("old not found", func(t *testing.T) {
		os.WriteFile(testFile, []byte("Hello World"), 0644)
		input, _ := json.Marshal(map[string]interface{}{
			"path":        testFile,
			"old_string":  "NotFound",
			"new_string":  "X",
			"replace_all": false,
		})
		result, err := tool.Execute(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error for old_string not found")
		}
	})

	t.Run("same old and new", func(t *testing.T) {
		input, _ := json.Marshal(map[string]interface{}{
			"path":        testFile,
			"old_string":  "same",
			"new_string":  "same",
			"replace_all": false,
		})
		result, err := tool.Execute(context.Background(), input, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error for same old and new strings")
		}
	})
}

func TestDefaultGlobTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.WriteFile(filepath.Join(dir, "c.log"), []byte("c"), 0644)

	tool := &defaultGlobTool{}
	input, _ := json.Marshal(map[string]string{
		"pattern": "*.txt",
		"path":    dir,
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "a.txt") {
		t.Errorf("expected 'a.txt' in glob output, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "b.txt") {
		t.Errorf("expected 'b.txt' in glob output, got %q", result.Content)
	}
	if strings.Contains(result.Content, "c.log") {
		t.Error("c.log should not match *.txt pattern")
	}
}

func TestDefaultGrepTool(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "match.txt"), []byte("contains needle"), 0644)
	os.WriteFile(filepath.Join(dir, "nomatch.txt"), []byte("no haystack"), 0644)

	tool := &defaultGrepTool{}
	input, _ := json.Marshal(map[string]string{
		"pattern": "needle",
		"path":    dir,
	})

	result, err := tool.Execute(context.Background(), input, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected no error, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "match.txt") {
		t.Errorf("expected 'match.txt' in grep output, got %q", result.Content)
	}
	if strings.Contains(result.Content, "nomatch.txt") {
		t.Error("nomatch.txt should not appear in grep results")
	}
}

func TestDefaultToolsIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	// Read-only tools should be concurrency safe
	safeTools := map[string]bool{
		"read_file":  true,
		"Glob":       true,
		"Grep":       true,
		"WebFetch":   true,
		"WebSearch":  true,
		"TaskGet":    true,
		"TaskList":   true,
	}

	tools := DefaultTools()
	for _, tl := range tools {
		isSafe := safeTools[tl.Name()]
		if isSafe && !tl.IsConcurrencySafe() {
			t.Errorf("tool %q should be concurrency safe", tl.Name())
		}
	}
}
