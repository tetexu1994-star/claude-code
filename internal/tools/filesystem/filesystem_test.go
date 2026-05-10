package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewTool(t *testing.T) {
	t.Parallel()

	tool := NewTool()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name != "filesystem" {
		t.Errorf("expected name 'filesystem', got %q", tool.Name)
	}
	if !tool.Enabled {
		t.Error("expected enabled true")
	}
	if len(tool.AllowedPaths) == 0 {
		t.Error("expected at least one allowed path")
	}
}

func TestIsPathAllowed(t *testing.T) {
	t.Parallel()

	t.Run("no allowed paths allows everything", func(t *testing.T) {
		tool := &Tool{AllowedPaths: nil}
		if !tool.isPathAllowed("/anything") {
			t.Error("expected allowed when no paths configured")
		}
	})

	t.Run("empty allowed paths allows everything", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{}}
		if !tool.isPathAllowed("/anything") {
			t.Error("expected allowed when empty paths")
		}
	})

	t.Run("path within allowed", func(t *testing.T) {
		dir := t.TempDir()
		tool := &Tool{AllowedPaths: []string{dir}}
		if !tool.isPathAllowed(filepath.Join(dir, "test.txt")) {
			t.Error("expected path to be allowed")
		}
	})

	t.Run("path outside allowed", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{"/tmp/test-allowed"}}
		if tool.isPathAllowed("/etc/passwd") {
			t.Error("expected path to be denied")
		}
	})
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("read existing file", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		testFile := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
			t.Fatalf("write test file: %v", err)
		}

		ctx := context.Background()
		result, err := tool.ReadFile(ctx, testFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != "hello world" {
			t.Errorf("expected 'hello world', got %q", result.Content)
		}
		if result.Size != 11 {
			t.Errorf("expected size 11, got %d", result.Size)
		}
	})

	t.Run("read file outside allowed path", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		ctx := context.Background()
		_, err := tool.ReadFile(ctx, "/etc/passwd")
		if err == nil {
			t.Error("expected error for path outside allowed")
		}
		if !strings.Contains(err.Error(), "not allowed") {
			t.Errorf("expected 'not allowed', got %q", err.Error())
		}
	})

	t.Run("read nonexistent file", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		ctx := context.Background()
		_, err := tool.ReadFile(ctx, filepath.Join(dir, "nonexistent.txt"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("write new file", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		testFile := filepath.Join(dir, "output.txt")
		ctx := context.Background()
		result, err := tool.WriteFile(ctx, testFile, "file content")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Path != testFile {
			t.Errorf("expected path %q, got %q", testFile, result.Path)
		}
		if result.Size != 12 {
			t.Errorf("expected size 12, got %d", result.Size)
		}

		// Verify content
		data, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(data) != "file content" {
			t.Errorf("expected 'file content', got %q", string(data))
		}
	})

	t.Run("write with parent directory creation", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		testFile := filepath.Join(dir, "sub", "deep", "output.txt")
		ctx := context.Background()
		_, err := tool.WriteFile(ctx, testFile, "nested content")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(data) != "nested content" {
			t.Errorf("expected 'nested content', got %q", string(data))
		}
	})

	t.Run("write outside allowed path", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		ctx := context.Background()
		_, err := tool.WriteFile(ctx, "/etc/test.txt", "bad")
		if err == nil {
			t.Error("expected error for write outside allowed path")
		}
	})
}

func TestListDir(t *testing.T) {
	dir := t.TempDir()

	t.Run("list directory", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		// Create some files
		os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
		os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
		os.MkdirAll(filepath.Join(dir, "sub"), 0755)

		ctx := context.Background()
		result, err := tool.ListDir(ctx, dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Total < 2 {
			t.Errorf("expected at least 2 entries, got %d", result.Total)
		}

		foundFile, foundDir := false, false
		for _, e := range result.Entries {
			if e.Name == "a.txt" && !e.IsDir {
				foundFile = true
			}
			if e.Name == "sub" && e.IsDir {
				foundDir = true
			}
		}
		if !foundFile {
			t.Error("expected a.txt in listing")
		}
		if !foundDir {
			t.Error("expected sub dir in listing")
		}
	})

	t.Run("list outside allowed path", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		ctx := context.Background()
		_, err := tool.ListDir(ctx, "/etc")
		if err == nil {
			t.Error("expected error for list outside allowed path")
		}
	})
}

func TestDeleteFile(t *testing.T) {
	dir := t.TempDir()

	t.Run("delete file", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		testFile := filepath.Join(dir, "delete_me.txt")
		os.WriteFile(testFile, []byte("temp"), 0644)

		ctx := context.Background()
		err := tool.DeleteFile(ctx, testFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify file is gone
		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			t.Error("expected file to be deleted")
		}
	})

	t.Run("delete outside allowed path", func(t *testing.T) {
		tool := &Tool{AllowedPaths: []string{dir}}
		ctx := context.Background()
		err := tool.DeleteFile(ctx, "/etc/test.txt")
		if err == nil {
			t.Error("expected error for delete outside allowed path")
		}
	})
}

func TestStructs(t *testing.T) {
	t.Parallel()

	t.Run("ReadResult", func(t *testing.T) {
		r := ReadResult{Content: "test", Size: 4}
		if r.Content != "test" || r.Size != 4 {
			t.Error("ReadResult field mismatch")
		}
	})

	t.Run("WriteResult", func(t *testing.T) {
		r := WriteResult{Path: "/tmp/test", Size: 10}
		if r.Path != "/tmp/test" || r.Size != 10 {
			t.Error("WriteResult field mismatch")
		}
	})

	t.Run("ListResult", func(t *testing.T) {
		r := ListResult{Total: 3}
		if r.Total != 3 {
			t.Error("ListResult field mismatch")
		}
	})

	t.Run("Entry", func(t *testing.T) {
		e := Entry{Name: "file.txt", IsDir: false, Size: 100, ModTime: "2026-01-01 12:00:00"}
		if e.Name != "file.txt" || e.IsDir {
			t.Error("Entry field mismatch")
		}
	})
}
