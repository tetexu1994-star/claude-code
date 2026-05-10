package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tetexu/tlaude-code/internal/logging"
)

// JSON-RPC version and MCP-related constants.
const (
	JSONRPCVersion  = "2.0"
	MCPProtocol     = "2024-11-05"
	DefaultTimeout  = 30 * time.Second
	SSEDataPrefix   = "data: "
)

// JSONRPCRequest represents a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCNotification represents a JSON-RPC 2.0 notification (no id).
type JSONRPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCError represents a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// Transport is the interface for MCP transport layers.
type Transport interface {
	Start(ctx context.Context) error
	Send(req JSONRPCRequest) error
	Receive() (*JSONRPCResponse, error)
	Notifications() <-chan *JSONRPCNotification
	Close() error
}

// Client is an MCP JSON-RPC client that works over any Transport.
type Client struct {
	transport Transport
	nextID    atomic.Int64

	mu       sync.Mutex
	pending  map[int64]chan *JSONRPCResponse
	closed   bool
	cancelFn context.CancelFunc
}

// NewClient creates a new MCP client over the given transport.
func NewClient(transport Transport) *Client {
	return &Client{
		transport: transport,
		pending:   make(map[int64]chan *JSONRPCResponse),
	}
}

// Start initializes the transport and sends the initialize request.
func (c *Client) Start(ctx context.Context) error {
	ctx, c.cancelFn = context.WithCancel(ctx)

	if err := c.transport.Start(ctx); err != nil {
		return fmt.Errorf("transport start: %w", err)
	}

	// Drain notification channel in background.
	go c.drainNotifications(ctx)

	// Send MCP initialize request.
	initParams := map[string]interface{}{
		"protocolVersion": MCPProtocol,
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]string{
			"name":    "tlaude-code",
			"version": "0.3.0",
		},
	}

	resp, err := c.Call(ctx, "initialize", initParams)
	if err != nil {
		c.Close()
		return fmt.Errorf("initialize: %w", err)
	}

	logging.Debug("mcp initialized", "result", string(resp))
	return nil
}

// Call sends a request and waits for a response.
func (c *Client) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	req := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan *JSONRPCResponse, 1)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("client is closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.transport.Send(req); err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Notify sends a notification (no response expected).
func (c *Client) Notify(ctx context.Context, method string, params interface{}) error {
	notif := JSONRPCNotification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
		Params:  params,
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return err
	}

	// Bypass the typed Transport.Send since it expects a request.
	return c.sendRaw(data)
}

// Close shuts down the client and its transport.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	if c.cancelFn != nil {
		c.cancelFn()
	}
	// Reject all pending calls.
	for id, ch := range c.pending {
		select {
		case ch <- &JSONRPCResponse{Error: &JSONRPCError{Code: -32000, Message: "client closed"}}:
		default:
		}
		delete(c.pending, id)
	}
	c.mu.Unlock()

	return c.transport.Close()
}

func (c *Client) sendRaw(data []byte) error {
	type writer interface {
		io.Writer
	}
	if w, ok := c.transport.(interface{ Write([]byte) (int, error) }); ok {
		_, err := w.Write(append(data, '\n'))
		return err
	}
	return fmt.Errorf("transport does not support raw writes")
}

func (c *Client) drainNotifications(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case notif, ok := <-c.transport.Notifications():
			if !ok {
				return
			}
			logging.Debug("mcp notification", "method", notif.Method)
		}
	}
}

// --- Stdio Transport ---

// StdioTransport implements Transport over a child process stdin/stdout.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	notifs chan *JSONRPCNotification
	done   chan struct{}
}

// NewStdioTransport creates a transport that communicates with a child process.
func NewStdioTransport(cmd *exec.Cmd) *StdioTransport {
	return &StdioTransport{
		cmd:    cmd,
		notifs: make(chan *JSONRPCNotification, 64),
		done:   make(chan struct{}),
	}
}

// Start runs the child process and connects pipes.
func (t *StdioTransport) Start(ctx context.Context) error {
	stdin, err := t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	t.stdin = stdin

	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	t.stdout = bufio.NewReader(stdout)

	go t.readLoop(ctx)
	return nil
}

// Send writes a JSON-RPC request to stdin.
func (t *StdioTransport) Send(req JSONRPCRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = t.stdin.Write(data)
	return err
}

// Receive is not used for stdio — responses are matched by ID via readLoop.
func (t *StdioTransport) Receive() (*JSONRPCResponse, error) {
	return nil, fmt.Errorf("receive not supported on stdio transport; use Call")
}

// Notifications returns the notification channel.
func (t *StdioTransport) Notifications() <-chan *JSONRPCNotification {
	return t.notifs
}

// Close terminates the child process.
func (t *StdioTransport) Close() error {
	if t.stdin != nil {
		t.stdin.Close()
	}
	if t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}
	return nil
}

func (t *StdioTransport) Write(b []byte) (int, error) {
	return t.stdin.Write(b)
}

func (t *StdioTransport) readLoop(ctx context.Context) {
	defer close(t.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logging.Warn("mcp stdio read error", "error", err)
			}
			return
		}

		line = stripCRLF(line)
		if len(line) == 0 {
			continue
		}

		// Check for notification (no ID field).
		var raw struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      *int64          `json:"id"`
			Method  string          `json:"method"`
			Result  json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			logging.Warn("mcp parse error", "error", err, "line", string(line))
			continue
		}

		if raw.ID == nil && raw.Method != "" {
			var notif JSONRPCNotification
			if err := json.Unmarshal(line, &notif); err == nil {
				select {
				case t.notifs <- &notif:
				default:
				}
			}
		}
	}
}

// --- SSE Transport ---

// SSETransport implements Transport over HTTP Server-Sent Events.
type SSETransport struct {
	serverURL string
	client    *http.Client
	notifs    chan *JSONRPCNotification
	done      chan struct{}
	req       *http.Request
}

// NewSSETransport creates an SSE-based transport.
func NewSSETransport(serverURL string) *SSETransport {
	return &SSETransport{
		serverURL: serverURL,
		client:    &http.Client{Timeout: 0},
		notifs:    make(chan *JSONRPCNotification, 64),
		done:      make(chan struct{}),
	}
}

// Start opens the SSE connection.
func (t *SSETransport) Start(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", t.serverURL, nil)
	if err != nil {
		return fmt.Errorf("creating SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	t.req = req

	go t.readLoop(ctx)
	return nil
}

// Send posts a JSON-RPC request to the MCP server via HTTP POST.
func (t *SSETransport) Send(req JSONRPCRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := t.client.Post(t.serverURL, "application/json", strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("sse post: %w", err)
	}
	resp.Body.Close()
	return nil
}

// Receive is not used for SSE — responses are matched by ID in the SSE stream.
func (t *SSETransport) Receive() (*JSONRPCResponse, error) {
	return nil, fmt.Errorf("receive not supported on SSE transport; use Call")
}

// Notifications returns the notification channel.
func (t *SSETransport) Notifications() <-chan *JSONRPCNotification {
	return t.notifs
}

// Close terminates the SSE connection.
func (t *SSETransport) Close() error {
	close(t.done)
	if t.req != nil {
		if tr, ok := t.client.Transport.(*http.Transport); ok && tr != nil {
			tr.CloseIdleConnections()
		}
	}
	return nil
}

func (t *SSETransport) readLoop(ctx context.Context) {
	defer close(t.notifs)

	resp, err := t.client.Do(t.req)
	if err != nil {
		logging.Warn("sse connection failed", "error", err)
		return
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logging.Warn("sse read error", "error", err)
			}
			return
		}

		line = stripCRLF(line)
		lineStr := string(line)

		if !strings.HasPrefix(lineStr, SSEDataPrefix) {
			continue
		}

		data := strings.TrimPrefix(lineStr, SSEDataPrefix)

		// Check for notification.
		var raw struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      *int64          `json:"id"`
			Method  string          `json:"method"`
			Result  json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal([]byte(data), &raw); err != nil {
			continue
		}

		if raw.ID == nil && raw.Method != "" {
			var notif JSONRPCNotification
			if err := json.Unmarshal([]byte(data), &notif); err == nil {
				select {
				case t.notifs <- &notif:
				default:
				}
			}
		}
	}
}

// --- Manager ---

// ServerStatus 单个 MCP 服务器状态
type ServerStatus struct {
	Name   string
	Status string // "connected", "error", "disconnected"
}

// Manager manages multiple MCP server connections.
type Manager struct {
	mu      sync.Mutex
	clients map[string]*Client
}

// NewManager creates an empty MCP connection manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

// Add starts a new MCP client under the given name.
func (m *Manager) Add(ctx context.Context, name string, client *Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.clients[name]; ok {
		return fmt.Errorf("mcp server %q already registered", name)
	}

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("starting mcp server %q: %w", name, err)
	}

	m.clients[name] = client
	logging.Info("mcp server connected", "name", name)
	return nil
}

// Remove shuts down and removes an MCP client by name.
func (m *Manager) Remove(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if client, ok := m.clients[name]; ok {
		client.Close()
		delete(m.clients, name)
		logging.Info("mcp server removed", "name", name)
	}
}

// Get returns an MCP client by name.
func (m *Manager) Get(name string) (*Client, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, ok := m.clients[name]
	return client, ok
}

// List returns all registered MCP server names.
func (m *Manager) List() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}

// Status returns the status of all MCP servers.
func (m *Manager) Status() []ServerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	var statuses []ServerStatus
	for name := range m.clients {
		statuses = append(statuses, ServerStatus{
			Name:   name,
			Status: "connected",
		})
	}
	return statuses
}

// Close shuts down all MCP clients.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		client.Close()
		delete(m.clients, name)
	}
	logging.Info("all mcp servers disconnected")
}

// stripCRLF removes trailing \r and \n from b.
func stripCRLF(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
