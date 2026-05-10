package sandbox

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Run("off mode", func(t *testing.T) {
		sb, err := New(Config{Mode: ModeOff})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sb == nil {
			t.Fatal("expected sandboxer, got nil")
		}
		if sb.Name() != "Direct" {
			t.Errorf("expected 'Direct', got %q", sb.Name())
		}
	})

	t.Run("restricted mode", func(t *testing.T) {
		sb, err := New(Config{Mode: ModeRestricted})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sb == nil {
			t.Fatal("expected sandboxer, got nil")
		}
		if sb.Name() != "Restricted" {
			t.Errorf("expected 'Restricted', got %q", sb.Name())
		}
	})

	t.Run("wasm mode returns error", func(t *testing.T) {
		sb, err := New(Config{Mode: ModeWASM})
		if err == nil {
			t.Fatal("expected error for WASM mode")
		}
		if !strings.Contains(err.Error(), "WASM") {
			t.Errorf("expected WASM error, got %q", err.Error())
		}
		_ = sb
	})

	t.Run("unknown mode returns error", func(t *testing.T) {
		sb, err := New(Config{Mode: "unknown"})
		if err == nil {
			t.Fatal("expected error for unknown mode")
		}
		if !strings.Contains(err.Error(), "unknown sandbox mode") {
			t.Errorf("expected 'unknown sandbox mode', got %q", err.Error())
		}
		_ = sb
	})
}

func TestPassthroughExecute(t *testing.T) {
	t.Parallel()

	sb, err := New(Config{Mode: ModeOff, TimeoutSec: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("successful command", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.Execute(ctx, "echo", []string{"hello"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if !strings.Contains(result.Stdout, "hello") {
			t.Errorf("expected stdout to contain 'hello', got %q", result.Stdout)
		}
		if result.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.ExitCode)
		}
		if result.Duration <= 0 {
			t.Error("expected positive duration")
		}
	})

	t.Run("failed command", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.Execute(ctx, "false", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode == 0 {
			t.Error("expected non-zero exit code")
		}
	})

	t.Run("command not found", func(t *testing.T) {
		ctx := context.Background()
		_, err := sb.Execute(ctx, "nonexistent_command_xyz", nil)
		if err == nil {
			t.Error("expected error for nonexistent command")
		}
	})
}

func TestPassthroughExecuteScript(t *testing.T) {
	t.Parallel()

	sb, err := New(Config{Mode: ModeOff, TimeoutSec: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("bash script", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.ExecuteScript(ctx, "bash", "echo hello_from_bash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Stdout, "hello_from_bash") {
			t.Errorf("expected 'hello_from_bash', got %q", result.Stdout)
		}
	})

	t.Run("python script", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.ExecuteScript(ctx, "python", "print('hello_py')")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Stdout, "hello_py") {
			t.Errorf("expected 'hello_py', got %q", result.Stdout)
		}
	})

	t.Run("shell alias", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.ExecuteScript(ctx, "shell", "echo shell_test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Stdout, "shell_test") {
			t.Errorf("expected 'shell_test', got %q", result.Stdout)
		}
	})

	t.Run("default language falls back to bash", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.ExecuteScript(ctx, "unknown_lang", "echo fallback")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Stdout, "fallback") {
			t.Errorf("expected 'fallback', got %q", result.Stdout)
		}
	})
}

func TestRestrictedExecute(t *testing.T) {
	sb, err := New(Config{
		Mode:       ModeRestricted,
		TimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("simple echo", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.Execute(ctx, "echo", []string{"hello_restricted"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Stdout, "hello_restricted") {
			t.Errorf("expected 'hello_restricted', got %q", result.Stdout)
		}
	})

	t.Run("exit code propagation", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.Execute(ctx, "bash", []string{"-c", "exit 42"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.ExitCode != 42 {
			t.Errorf("expected exit code 42, got %d", result.ExitCode)
		}
	})
}

func TestRestrictedExecuteScript(t *testing.T) {
	sb, err := New(Config{
		Mode:       ModeRestricted,
		TimeoutSec: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("bash via restricted", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.ExecuteScript(ctx, "bash", "echo restricted_bash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Stdout, "restricted_bash") {
			t.Errorf("expected 'restricted_bash', got %q", result.Stdout)
		}
	})

	t.Run("unsupported language", func(t *testing.T) {
		ctx := context.Background()
		_, err := sb.ExecuteScript(ctx, "rust", "fn main() {}")
		if err == nil {
			t.Error("expected error for unsupported language")
		}
		if !strings.Contains(err.Error(), "unsupported language") {
			t.Errorf("expected 'unsupported language', got %q", err.Error())
		}
	})
}

func TestPassthroughTimeout(t *testing.T) {
	t.Parallel()

	sb, err := New(Config{Mode: ModeOff, TimeoutSec: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Run("command killed by timeout", func(t *testing.T) {
		ctx := context.Background()
		result, err := sb.Execute(ctx, "sleep", []string{"3"})
		// On macOS, killed processes produce *exec.ExitError, so err may be nil.
		if err != nil {
			// Error path: expected for some systems.
			return
		}
		// Result path: exit code should be non-zero (signal killed).
		if result.ExitCode == 0 {
			t.Error("expected non-zero exit code for killed process")
		}
	})
}

func TestResult(t *testing.T) {
	t.Parallel()

	r := Result{
		Stdout:   "output",
		Stderr:   "error output",
		ExitCode: 1,
		Duration: 100 * time.Millisecond,
	}

	if r.Stdout != "output" {
		t.Error("expected Stdout 'output'")
	}
	if r.Stderr != "error output" {
		t.Error("expected Stderr 'error output'")
	}
	if r.ExitCode != 1 {
		t.Error("expected ExitCode 1")
	}
	if r.Duration != 100*time.Millisecond {
		t.Error("expected Duration 100ms")
	}
}

func TestConfig(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Mode:         ModeRestricted,
		TimeoutSec:   30,
		MaxMemoryMB:  512,
		AllowNetwork: false,
		AllowWrite:   true,
		TempDir:      "/tmp/test",
	}

	if cfg.Mode != ModeRestricted {
		t.Error("expected Mode restricted")
	}
	if cfg.TimeoutSec != 30 {
		t.Error("expected TimeoutSec 30")
	}
	if cfg.MaxMemoryMB != 512 {
		t.Error("expected MaxMemoryMB 512")
	}
	if cfg.AllowNetwork {
		t.Error("expected AllowNetwork false")
	}
	if !cfg.AllowWrite {
		t.Error("expected AllowWrite true")
	}
	if cfg.TempDir != "/tmp/test" {
		t.Error("expected TempDir '/tmp/test'")
	}
}

func TestModeConstants(t *testing.T) {
	t.Parallel()

	if ModeWASM != "wasm" {
		t.Errorf("expected 'wasm', got %q", ModeWASM)
	}
	if ModeRestricted != "restricted" {
		t.Errorf("expected 'restricted', got %q", ModeRestricted)
	}
	if ModeOff != "off" {
		t.Errorf("expected 'off', got %q", ModeOff)
	}
}
