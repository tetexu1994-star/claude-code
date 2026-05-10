package bash

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewTool(t *testing.T) {
	t.Parallel()

	tool := NewTool()
	if tool == nil {
		t.Fatal("expected tool, got nil")
	}
	if tool.Name != "bash" {
		t.Errorf("expected name 'bash', got %q", tool.Name)
	}
	if !tool.Enabled {
		t.Error("expected enabled true")
	}
	if tool.Timeout != 120*time.Second {
		t.Errorf("expected timeout 120s, got %v", tool.Timeout)
	}
	if tool.AllowedCmds != nil {
		t.Error("expected nil AllowedCmds")
	}
}

func TestToolExecute(t *testing.T) {
	t.Parallel()

	t.Run("successful command", func(t *testing.T) {
		tool := NewTool()
		ctx := context.Background()
		result, err := tool.Execute(ctx, "echo hello_bash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if !strings.Contains(result.Stdout, "hello_bash") {
			t.Errorf("expected 'hello_bash', got %q", result.Stdout)
		}
		if result.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.ExitCode)
		}
		if result.Duration == "" {
			t.Error("expected non-empty duration")
		}
	})

	t.Run("failed command", func(t *testing.T) {
		tool := NewTool()
		ctx := context.Background()
		result, err := tool.Execute(ctx, "exit 42")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode != 42 {
			t.Errorf("expected exit code 42, got %d", result.ExitCode)
		}
	})

	t.Run("disable tool", func(t *testing.T) {
		tool := NewTool()
		tool.Enabled = false
		ctx := context.Background()
		_, err := tool.Execute(ctx, "echo test")
		if err == nil {
			t.Error("expected error for disabled tool")
		}
		if !strings.Contains(err.Error(), "disabled") {
			t.Errorf("expected disabled error, got %q", err.Error())
		}
	})

	t.Run("multi-line output", func(t *testing.T) {
		tool := NewTool()
		ctx := context.Background()
		result, err := tool.Execute(ctx, "echo line1 && echo line2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Stdout, "line1") {
			t.Error("expected line1 in output")
		}
		if !strings.Contains(result.Stdout, "line2") {
			t.Error("expected line2 in output")
		}
	})

	t.Run("stderr capture", func(t *testing.T) {
		tool := NewTool()
		ctx := context.Background()
		result, err := tool.Execute(ctx, "echo error_msg >&2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Stderr, "error_msg") {
			t.Errorf("expected 'error_msg' in stderr, got %q", result.Stderr)
		}
	})
}

func TestSetAllowedCommands(t *testing.T) {
	t.Parallel()

	tool := NewTool()
	cmds := []string{"ls", "cat", "echo"}
	tool.SetAllowedCommands(cmds)

	if len(tool.AllowedCmds) != 3 {
		t.Errorf("expected 3 commands, got %d", len(tool.AllowedCmds))
	}
	if tool.AllowedCmds[0] != "ls" {
		t.Errorf("expected first command 'ls', got %q", tool.AllowedCmds[0])
	}
}

func TestResultStruct(t *testing.T) {
	t.Parallel()

	r := Result{
		Stdout:   "output",
		Stderr:   "error",
		ExitCode: 0,
		Duration: "100ms",
	}

	if r.Stdout != "output" {
		t.Error("expected Stdout")
	}
	if r.Stderr != "error" {
		t.Error("expected Stderr")
	}
	if r.ExitCode != 0 {
		t.Error("expected ExitCode 0")
	}
	if r.Duration != "100ms" {
		t.Error("expected Duration")
	}
}
