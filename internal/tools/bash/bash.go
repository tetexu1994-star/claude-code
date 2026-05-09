package bash

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Result struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration string `json:"duration"`
}

type Tool struct {
	Name        string
	Description string
	Enabled     bool
	Timeout     time.Duration
	AllowedCmds []string
}

func NewTool() *Tool {
	return &Tool{
		Name:        "bash",
		Description: "Execute shell commands and return results",
		Enabled:     true,
		Timeout:     120 * time.Second,
		AllowedCmds: nil,
	}
}

func (t *Tool) Execute(ctx context.Context, command string) (*Result, error) {
	if !t.Enabled {
		return nil, fmt.Errorf("bash tool is disabled")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(cmdCtx, "bash", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start).Round(time.Millisecond).String()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to execute command: %w", err)
		}
	}

	return &Result{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

func (t *Tool) SetAllowedCommands(cmds []string) {
	t.AllowedCmds = cmds
}
