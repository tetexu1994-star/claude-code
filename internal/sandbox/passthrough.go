package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type passthroughSandbox struct {
	cfg Config
}

func newPassthroughSandbox(cfg Config) (*passthroughSandbox, error) {
	return &passthroughSandbox{cfg: cfg}, nil
}

func (s *passthroughSandbox) Name() string { return "Direct" }

func (s *passthroughSandbox) Execute(ctx context.Context, command string, args []string) (*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.TimeoutSec)*time.Second)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, command, args...)
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
			return nil, fmt.Errorf("execute failed: %w", err)
		}
	}

	return &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

func (s *passthroughSandbox) ExecuteScript(ctx context.Context, language string, code string) (*Result, error) {
	switch strings.ToLower(language) {
	case "python", "py":
		return s.Execute(ctx, "python3", []string{"-c", code})
	case "bash", "sh", "shell":
		return s.Execute(ctx, "bash", []string{"-c", code})
	case "node", "js", "javascript":
		return s.Execute(ctx, "node", []string{"-e", code})
	default:
		return s.Execute(ctx, "bash", []string{"-c", code})
	}
}
