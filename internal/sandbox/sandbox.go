package sandbox

import (
	"context"
	"fmt"
	"time"
)

// Mode is the sandbox execution mode.
type Mode string

const (
	ModeWASM       Mode = "wasm"
	ModeRestricted Mode = "restricted"
	ModeOff        Mode = "off"
)

// Config configures the sandbox.
type Config struct {
	Mode         Mode   `yaml:"mode" json:"mode"`
	TimeoutSec   int    `yaml:"timeout_sec" json:"timeout_sec"`
	MaxMemoryMB  int    `yaml:"max_memory_mb" json:"max_memory_mb"`
	AllowNetwork bool   `yaml:"allow_network" json:"allow_network"`
	AllowWrite   bool   `yaml:"allow_write" json:"allow_write"`
	TempDir      string `yaml:"temp_dir" json:"temp_dir"`
}

// Result captures the output of a sandbox execution.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// Sandboxer is the interface for executing commands and scripts in an isolated environment.
type Sandboxer interface {
	Execute(ctx context.Context, command string, args []string) (*Result, error)
	ExecuteScript(ctx context.Context, language string, code string) (*Result, error)
	Name() string
}

// New creates a Sandboxer based on the provided config.
func New(cfg Config) (Sandboxer, error) {
	switch cfg.Mode {
	case ModeWASM:
		return newWasmSandbox(cfg)
	case ModeRestricted:
		return newRestrictedSandbox(cfg)
	case ModeOff:
		return newPassthroughSandbox(cfg)
	default:
		return nil, fmt.Errorf("unknown sandbox mode: %s", cfg.Mode)
	}
}
