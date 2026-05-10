package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type restrictedSandbox struct {
	cfg Config
}

func newRestrictedSandbox(cfg Config) (*restrictedSandbox, error) {
	if cfg.TempDir == "" {
		cfg.TempDir = "/tmp/tlaude-code-sandbox"
	}
	if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
		return nil, fmt.Errorf("create sandbox temp dir: %w", err)
	}
	return &restrictedSandbox{cfg: cfg}, nil
}

func (s *restrictedSandbox) Name() string { return "Restricted" }

func (s *restrictedSandbox) Execute(ctx context.Context, command string, args []string) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutSec)*time.Second)
	defer cancel()

	workDir := filepath.Join(s.cfg.TempDir, fmt.Sprintf("run_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = workDir

	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + workDir,
		"TEMP=" + s.cfg.TempDir,
	}
	if !s.cfg.AllowNetwork {
		cmd.Env = append(cmd.Env, "NO_NETWORK=1")
	}

	cmd.Stdin = nil
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("sandbox execute failed: %w", err)
		}
	}

	maxOutput := 100 * 1024
	output := stdout.String()
	if len(output) > maxOutput {
		output = output[:maxOutput] + "\n... (output truncated at 100KB)"
	}
	errOutput := stderr.String()
	if len(errOutput) > maxOutput {
		errOutput = errOutput[:maxOutput] + "\n... (stderr truncated at 100KB)"
	}

	return &Result{
		Stdout:   output,
		Stderr:   errOutput,
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

func (s *restrictedSandbox) ExecuteScript(ctx context.Context, language string, code string) (*Result, error) {
	switch strings.ToLower(language) {
	case "python", "py":
		return s.Execute(ctx, "python3", []string{"-c", code})
	case "bash", "sh", "shell":
		return s.Execute(ctx, "bash", []string{"-c", code})
	case "node", "js", "javascript":
		return s.Execute(ctx, "node", []string{"-e", code})
	case "go":
		// Write temp file and run.
		tmpFile := filepath.Join(s.cfg.TempDir, fmt.Sprintf("sandbox_%d.go", time.Now().UnixNano()))
		if err := os.WriteFile(tmpFile, []byte(code), 0644); err != nil {
			return nil, fmt.Errorf("write temp go file: %w", err)
		}
		defer os.Remove(tmpFile)
		return s.Execute(ctx, "go", []string{"run", tmpFile})
	default:
		return nil, fmt.Errorf("unsupported language for sandbox: %s", language)
	}
}
