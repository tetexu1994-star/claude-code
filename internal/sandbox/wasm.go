package sandbox

import (
	"context"
	"fmt"
)

// TODO: WASM sandbox requires wazero dependency.
//   go get github.com/bytecodealliance/wazero
//
// Once wazero is available, implement:
//   1. Create wazero runtime with NewRuntime(ctx)
//   2. Instantiate WASI preview1 (wasi_snapshot_preview1.MustInstantiate)
//   3. Compile and instantiate WASM modules
//   4. Execute commands and capture stdout/stderr
//   5. Enforce memory limits, timeout, and isolation

type wasmSandbox struct {
	cfg Config
}

func newWasmSandbox(cfg Config) (*wasmSandbox, error) {
	return nil, fmt.Errorf("WASM sandbox not available: wazero dependency not installed")
}

func (s *wasmSandbox) Name() string { return "WASM" }

func (s *wasmSandbox) Execute(ctx context.Context, command string, args []string) (*Result, error) {
	return nil, fmt.Errorf("WASM sandbox not available: wazero dependency not installed")
}

func (s *wasmSandbox) ExecuteScript(ctx context.Context, language string, code string) (*Result, error) {
	return nil, fmt.Errorf("WASM sandbox not available: wazero dependency not installed")
}
