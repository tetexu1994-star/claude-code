package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/tetexu/tlaude-code/internal/logging"
)

// CompletionOptions describes completion provider capabilities.
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters,omitempty"`
	ResolveProvider   bool     `json:"resolveProvider,omitempty"`
}

// ServerCapabilities describes what an LSP server can do.
type ServerCapabilities struct {
	TextDocumentSync   interface{}         `json:"textDocumentSync,omitempty"`
	HoverProvider      bool                `json:"hoverProvider,omitempty"`
	DefinitionProvider bool                `json:"definitionProvider,omitempty"`
	ReferencesProvider bool                `json:"referencesProvider,omitempty"`
	CompletionProvider *CompletionOptions  `json:"completionProvider,omitempty"`
	SignatureHelpProvider *struct {
		TriggerCharacters []string `json:"triggerCharacters,omitempty"`
	} `json:"signatureHelpProvider,omitempty"`
	CodeActionProvider   interface{} `json:"codeActionProvider,omitempty"`
	DocumentSymbolProvider bool      `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider bool    `json:"workspaceSymbolProvider,omitempty"`
}

// rpcRequest is the wire format for a JSON-RPC request.
type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// rpcResponse is the wire format for a JSON-RPC response.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcNotification is the wire format for a JSON-RPC notification.
type rpcNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcError represents a JSON-RPC error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// StartOptions configures the LSP server process environment.
type StartOptions struct {
	Env map[string]string
	Cwd string
}

// Client is an LSP JSON-RPC client that communicates with a language server
// process over stdio using Content-Length framed messages.
type Client struct {
	serverName string
	onCrash    func(error)

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	transport *Transport

	mu              sync.Mutex
	seq             int64
	capabilities    *ServerCapabilities
	initialized     bool
	crashed         bool
	stoppingIntent  bool // true when Stop() was called intentionally
	startFailed     bool
	startError      error

	handlers        map[string]func(json.RawMessage)
	requestHandlers map[string]func(json.RawMessage) (json.RawMessage, error)
	pending         map[int64]chan *rpcResponse
	done            chan struct{}

	// Queued handlers registered before Start() is called.
	pendingHandlers        []pendingHandler
	pendingRequestHandlers  []pendingRequestHandler
}

type pendingHandler struct {
	method  string
	handler func(json.RawMessage)
}

type pendingRequestHandler struct {
	method  string
	handler func(json.RawMessage) (json.RawMessage, error)
}

// NewClient creates a new LSP client for the named server.
func NewClient(serverName string, onCrash func(error)) *Client {
	return &Client{
		serverName:       serverName,
		onCrash:          onCrash,
		handlers:         make(map[string]func(json.RawMessage)),
		requestHandlers:  make(map[string]func(json.RawMessage) (json.RawMessage, error)),
		pending:          make(map[int64]chan *rpcResponse),
	}
}

// Capabilities returns the server capabilities (nil before initialize).
func (c *Client) Capabilities() *ServerCapabilities {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capabilities
}

// IsInitialized returns whether the server has been initialized.
func (c *Client) IsInitialized() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.initialized
}

// Crashed returns whether the server has crashed.
func (c *Client) Crashed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.crashed
}

// Start launches the LSP server process and establishes the JSON-RPC connection.
// It spawns the process, verifies it started successfully, and begins the read loop.
func (c *Client) Start(command string, args []string, opts *StartOptions) error {
	c.mu.Lock()
	if c.transport != nil {
		c.mu.Unlock()
		return fmt.Errorf("lsp start: client %q already started", c.serverName)
	}
	c.mu.Unlock()

	cmd := exec.Command(command, args...)
	cmd.Stderr = nil // stderr will be handled separately

	if opts != nil {
		if len(opts.Env) > 0 {
			cmd.Env = cmd.Environ()
			for k, v := range opts.Env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}
		if opts.Cwd != "" {
			cmd.Dir = opts.Cwd
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("lsp start: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("lsp start: stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("lsp start: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("lsp start: %w", err)
	}

	// Capture stderr for diagnostics.
	go func() {
		data := make([]byte, 4096)
		for {
			n, readErr := stderr.Read(data)
			if n > 0 {
				output := string(data[:n])
				logging.Debug("lsp stderr", "server", c.serverName, "output", output)
			}
			if readErr != nil {
				return
			}
		}
	}()

	c.cmd = cmd
	c.stdin = stdin
	c.transport = NewTransport(stdout, stdin)
	c.done = make(chan struct{})

	// Register process error/exit handlers.
	go c.monitorProcess()

	// Flush any queued handlers to the transport before starting read loop.
	c.mu.Lock()
	for _, ph := range c.pendingHandlers {
		c.handlers[ph.method] = ph.handler
	}
	c.pendingHandlers = nil
	for _, prh := range c.pendingRequestHandlers {
		c.requestHandlers[prh.method] = prh.handler
	}
	c.pendingRequestHandlers = nil
	c.mu.Unlock()

	// Start the read loop in a goroutine.
	go c.readLoop()

	logging.Debug("lsp client started", "server", c.serverName)
	return nil
}

// Initialize sends the LSP initialize request and tracks capabilities.
func (c *Client) Initialize(params interface{}) error {
	c.mu.Lock()
	if c.startFailed {
		err := c.startError
		c.mu.Unlock()
		return fmt.Errorf("lsp initialize: %w", err)
	}
	c.mu.Unlock()

	var result struct {
		Capabilities ServerCapabilities `json:"capabilities"`
	}
	if err := c.SendRequest("initialize", params, &result); err != nil {
		return fmt.Errorf("lsp initialize: %w", err)
	}

	c.mu.Lock()
	c.capabilities = &result.Capabilities
	c.mu.Unlock()

	// Send initialized notification.
	if err := c.SendNotification("initialized", struct{}{}); err != nil {
		return fmt.Errorf("lsp initialized notification: %w", err)
	}

	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()

	logging.Debug("lsp server initialized", "server", c.serverName)
	return nil
}

// SendRequest sends a JSON-RPC request and waits for the response.
// The result is unmarshaled into the provided value.
func (c *Client) SendRequest(method string, params interface{}, result interface{}) error {
	c.mu.Lock()
	if c.transport == nil {
		c.mu.Unlock()
		return fmt.Errorf("lsp sendRequest: client not started")
	}
	if c.startFailed {
		err := c.startError
		c.mu.Unlock()
		return fmt.Errorf("lsp sendRequest: %w", err)
	}
	if c.crashed {
		c.mu.Unlock()
		return fmt.Errorf("lsp sendRequest: server %q has crashed", c.serverName)
	}
	id := atomic.AddInt64(&c.seq, 1)
	ch := make(chan *rpcResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("lsp sendRequest marshal: %w", err)
	}

	c.mu.Lock()
	if err := c.transport.WriteMessage(data); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("lsp sendRequest write: %w", err)
	}
	c.mu.Unlock()

	resp := <-ch
	if resp.Error != nil {
		return resp.Error
	}

	if result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("lsp sendRequest unmarshal: %w", err)
		}
	}

	return nil
}

// SendNotification sends a JSON-RPC notification (no response expected).
func (c *Client) SendNotification(method string, params interface{}) error {
	c.mu.Lock()
	if c.transport == nil {
		c.mu.Unlock()
		return fmt.Errorf("lsp sendNotification: client not started")
	}
	if c.startFailed {
		err := c.startError
		c.mu.Unlock()
		return fmt.Errorf("lsp sendNotification: %w", err)
	}
	c.mu.Unlock()

	notif := rpcNotification{
		JSONRPC: "2.0",
		Method:  method,
	}

	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("lsp sendNotification marshal: %w", err)
		}
		notif.Params = data
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("lsp sendNotification marshal: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.transport.WriteMessage(data); err != nil {
		return fmt.Errorf("lsp sendNotification write: %w", err)
	}

	return nil
}

// OnNotification registers a handler for incoming notifications from the server.
// If called before Start(), the handler is queued and applied when the connection is ready.
func (c *Client) OnNotification(method string, handler func(json.RawMessage)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.transport == nil {
		c.pendingHandlers = append(c.pendingHandlers, pendingHandler{method, handler})
		return
	}

	if c.startFailed {
		return
	}

	c.handlers[method] = handler
}

// OnRequest registers a handler for incoming requests from the server.
// If called before Start(), the handler is queued and applied when the connection is ready.
func (c *Client) OnRequest(method string, handler func(json.RawMessage) (json.RawMessage, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.transport == nil {
		c.pendingRequestHandlers = append(c.pendingRequestHandlers, pendingRequestHandler{method, handler})
		return
	}

	if c.startFailed {
		return
	}

	c.requestHandlers[method] = handler
}

// Stop gracefully shuts down the LSP server (shutdown request → exit notification)
// and cleans up all resources.
func (c *Client) Stop() error {
	c.mu.Lock()
	c.stoppingIntent = true
	c.mu.Unlock()

	var shutdownErr error

	c.mu.Lock()
	transport := c.transport
	c.mu.Unlock()

	if transport != nil {
		// Try graceful shutdown. If the process has already exited, the
		// readLoop defer will reject the pending request and SendRequest
		// will return an error — that's fine.
		if err := c.SendRequest("shutdown", nil, nil); err != nil {
			logging.Debug("lsp shutdown request failed", "server", c.serverName, "error", err)
			shutdownErr = err
		}
		_ = c.SendNotification("exit", nil)
	}

	// Close read loop.
	c.mu.Lock()
	if c.done != nil {
		close(c.done)
	}
	c.mu.Unlock()

	// Kill the process if still running.
	c.mu.Lock()
	if c.stdin != nil {
		c.stdin.Close()
	}
	cmd := c.cmd
	c.mu.Unlock()

	if cmd != nil && cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}

	// Reject all pending requests.
	c.mu.Lock()
	for id, ch := range c.pending {
		select {
		case ch <- &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32000, Message: "server stopped"}}:
		default:
		}
		delete(c.pending, id)
	}

	c.transport = nil
	c.cmd = nil
	c.stdin = nil
	c.initialized = false
	c.capabilities = nil
	c.stoppingIntent = false

	if shutdownErr != nil {
		c.startFailed = true
		c.startError = shutdownErr
	}
	c.mu.Unlock()

	logging.Debug("lsp client stopped", "server", c.serverName)
	return shutdownErr
}

// monitorProcess watches the child process for errors and unexpected exits.
func (c *Client) monitorProcess() {
	// Wait() blocks until the process exits — called without the lock since
	// c.cmd is set once in Start() and then never reassigned (until Stop()).
	_ = c.cmd.Wait()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stoppingIntent {
		return
	}

	exitCode := c.cmd.ProcessState.ExitCode()
	if exitCode != 0 {
		c.initialized = false
		c.crashed = true
		crashErr := fmt.Errorf("lsp server %q crashed with exit code %d", c.serverName, exitCode)
		logging.Error("lsp server crashed", "server", c.serverName, "exitCode", exitCode)
		if c.onCrash != nil {
			// Call back without the lock to avoid potential deadlocks.
			c.mu.Unlock()
			c.onCrash(crashErr)
			c.mu.Lock()
		}
	}
}

// readLoop reads framed messages from the transport and dispatches them.
func (c *Client) readLoop() {
	defer func() {
		// When the read loop ends (process exited / error), reject all pending
		// requests so blocked SendRequest calls don't hang forever.
		c.mu.Lock()
		for id, ch := range c.pending {
			select {
			case ch <- &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: -32000, Message: "server disconnected"}}:
			default:
			}
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		data, err := c.transport.ReadMessage()
		if err != nil {
			c.mu.Lock()
			stopping := c.stoppingIntent
			c.mu.Unlock()
			if !stopping {
				logging.Debug("lsp read error", "server", c.serverName, "error", err)
			}
			return
		}

		c.dispatchMessage(data)
	}
}

// dispatchMessage routes an incoming message to the appropriate handler.
func (c *Client) dispatchMessage(data []byte) {
	// First check if this is a response (has an "id" field that is a number).
	var raw struct {
		ID     *json.RawMessage `json:"id"`
		Method string           `json:"method"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		logging.Warn("lsp parse message", "server", c.serverName, "error", err)
		return
	}

	if raw.Method != "" {
		if raw.ID != nil {
			// This is a request from the server.
			var req struct {
				ID     int64           `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params,omitempty"`
			}
			if err := json.Unmarshal(data, &req); err != nil {
				logging.Warn("lsp parse request", "server", c.serverName, "error", err)
				return
			}

			c.mu.Lock()
			handler, ok := c.requestHandlers[req.Method]
			c.mu.Unlock()

			if !ok {
				logging.Debug("lsp unhandled request", "server", c.serverName, "method", req.Method)
				return
			}

			result, handlerErr := handler(req.Params)
			if handlerErr != nil {
				logging.Warn("lsp request handler error", "server", c.serverName, "method", req.Method, "error", handlerErr)
				return
			}

			// Send the response back.
			resp := rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			}
			respData, marshalErr := json.Marshal(resp)
			if marshalErr != nil {
				logging.Warn("lsp marshal response", "server", c.serverName, "error", marshalErr)
				return
			}

			c.mu.Lock()
			if c.transport != nil {
				if wrErr := c.transport.WriteMessage(respData); wrErr != nil {
					logging.Warn("lsp write response", "server", c.serverName, "error", wrErr)
				}
			}
			c.mu.Unlock()
		} else {
			// This is a notification from the server.
			var notif rpcNotification
			if err := json.Unmarshal(data, &notif); err != nil {
				logging.Warn("lsp parse notification", "server", c.serverName, "error", err)
				return
			}

			c.mu.Lock()
			handler, ok := c.handlers[notif.Method]
			c.mu.Unlock()

			if !ok {
				logging.Debug("lsp unhandled notification", "server", c.serverName, "method", notif.Method)
				return
			}

			handler(notif.Params)
		}
	} else if raw.ID != nil {
		// This is a response to one of our requests.
		var resp rpcResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			logging.Warn("lsp parse response", "server", c.serverName, "error", err)
			return
		}

		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		c.mu.Unlock()
		if ok {
			select {
			case ch <- &resp:
			default:
			}
		}
	}
}
