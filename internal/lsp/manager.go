package lsp

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tetexu/tlaude-code/internal/logging"
)

// ServerConfig describes how to launch an LSP server and which file types it handles.
type ServerConfig struct {
	Command             string            `json:"command"`
	Args                []string          `json:"args,omitempty"`
	ExtensionToLanguage map[string]string `json:"extensionToLanguage"`
}

// Manager manages multiple LSP server instances and routes requests based on file extensions.
type Manager struct {
	mu      sync.RWMutex
	servers map[string]*Client
	extMap  map[string][]string // .ts -> [typescript-language-server]
	configs map[string]ServerConfig

	// Track which files have been opened on which servers (fileURI -> server name).
	openedFiles map[string]string
}

// NewManager creates an empty LSP server manager.
func NewManager() *Manager {
	return &Manager{
		servers:     make(map[string]*Client),
		extMap:      make(map[string][]string),
		configs:     make(map[string]ServerConfig),
		openedFiles: make(map[string]string),
	}
}

// Initialize loads the server configurations and builds the extension-to-server mapping.
// Servers are not started until a file matching their extension is accessed.
func (m *Manager) Initialize(configs map[string]ServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for serverName, cfg := range configs {
		if cfg.Command == "" {
			logging.Warn("lsp manager: server missing command field, skipping", "server", serverName)
			continue
		}
		if len(cfg.ExtensionToLanguage) == 0 {
			logging.Warn("lsp manager: server missing extensionToLanguage, skipping", "server", serverName)
			continue
		}

		// Map file extensions to this server.
		for ext := range cfg.ExtensionToLanguage {
			normalized := strings.ToLower(ext)
			if !strings.HasPrefix(normalized, ".") {
				normalized = "." + normalized
			}
			m.extMap[normalized] = append(m.extMap[normalized], serverName)
		}

		// Create a crash handler that resets state so the server can be restarted.
		serverNameCopy := serverName
		onCrash := func(err error) {
			m.mu.Lock()
			if s, ok := m.servers[serverNameCopy]; ok {
				s.mu.Lock()
				s.crashed = true
				s.initialized = false
				s.mu.Unlock()
			}
			m.mu.Unlock()
			logging.Error("lsp server crashed", "server", serverNameCopy, "error", err)
		}

		client := NewClient(serverName, onCrash)
		m.servers[serverName] = client
		m.configs[serverName] = cfg
	}

	logging.Info("lsp manager initialized", "servers", len(m.servers))
	return nil
}

// Shutdown stops all running servers and clears state.
func (m *Manager) Shutdown() error {
	m.mu.Lock()
	servers := m.servers
	m.servers = make(map[string]*Client)
	m.extMap = make(map[string][]string)
	m.configs = make(map[string]ServerConfig)
	m.openedFiles = make(map[string]string)
	m.mu.Unlock()

	var errs []error
	for name, server := range servers {
		if err := server.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("lsp shutdown %q: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("lsp shutdown errors: %v", errs)
	}

	logging.Info("lsp manager shut down")
	return nil
}

// GetServerForFile returns the LSP client for the given file path, or nil if no server matches.
// If multiple servers handle the same extension, returns the first registered server.
func (m *Manager) GetServerForFile(filePath string) *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ext := strings.ToLower(path.Ext(filePath))
	serverNames := m.extMap[ext]
	if len(serverNames) == 0 {
		return nil
	}

	return m.servers[serverNames[0]]
}

// EnsureStarted ensures the appropriate LSP server is started for the given file path.
// Returns nil if no server handles this file type.
func (m *Manager) EnsureStarted(filePath string) (*Client, error) {
	m.mu.RLock()
	ext := strings.ToLower(path.Ext(filePath))
	serverNames := m.extMap[ext]
	if len(serverNames) == 0 {
		m.mu.RUnlock()
		return nil, nil
	}
	serverName := serverNames[0]
	server := m.servers[serverName]
	m.mu.RUnlock()

	if server == nil {
		return nil, nil
	}

	server.mu.Lock()
	isRunning := server.transport != nil
	isCrashed := server.crashed
	isStartFailed := server.startFailed
	server.mu.Unlock()

	if isRunning && !isCrashed && !isStartFailed {
		return server, nil
	}

	// Start or restart the server.
	m.mu.RLock()
	cfg := m.configs[serverName]
	m.mu.RUnlock()

	if err := server.Start(cfg.Command, cfg.Args, nil); err != nil {
		return nil, fmt.Errorf("lsp ensureStarted %q: %w", serverName, err)
	}

	return server, nil
}

// OpenFile sends a textDocument/didOpen notification to the appropriate LSP server.
func (m *Manager) OpenFile(filePath, content string) error {
	server, err := m.EnsureStarted(filePath)
	if err != nil {
		return err
	}
	if server == nil {
		return nil
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("lsp openFile abs: %w", err)
	}
	fileURI := fileURIToString(absPath)

	// Skip if already opened on this server.
	m.mu.RLock()
	openedServer := m.openedFiles[fileURI]
	m.mu.RUnlock()
	if openedServer == server.serverName {
		return nil
	}

	m.mu.RLock()
	ext := strings.ToLower(path.Ext(filePath))
	cfg := m.configs[server.serverName]
	m.mu.RUnlock()

	languageID := cfg.ExtensionToLanguage[ext]
	if languageID == "" {
		languageID = "plaintext"
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        fileURI,
			"languageId": languageID,
			"version":    1,
			"text":       content,
		},
	}

	if err := server.SendNotification("textDocument/didOpen", params); err != nil {
		return fmt.Errorf("lsp openFile: %w", err)
	}

	m.mu.Lock()
	m.openedFiles[fileURI] = server.serverName
	m.mu.Unlock()

	logging.Debug("lsp didOpen sent", "server", server.serverName, "file", filePath)
	return nil
}

// ChangeFile sends a textDocument/didChange notification to the appropriate LSP server.
// If the file hasn't been opened yet on this server, it opens it first.
func (m *Manager) ChangeFile(filePath, content string) error {
	server := m.GetServerForFile(filePath)
	if server == nil {
		return nil
	}

	server.mu.Lock()
	isRunning := server.transport != nil && !server.crashed && !server.startFailed
	server.mu.Unlock()

	if !isRunning {
		return m.OpenFile(filePath, content)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("lsp changeFile abs: %w", err)
	}
	fileURI := fileURIToString(absPath)

	m.mu.RLock()
	openedServer := m.openedFiles[fileURI]
	m.mu.RUnlock()

	// If file hasn't been opened on this server yet, open it first.
	if openedServer != server.serverName {
		return m.OpenFile(filePath, content)
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":     fileURI,
			"version": 1,
		},
		"contentChanges": []map[string]interface{}{
			{"text": content},
		},
	}

	if err := server.SendNotification("textDocument/didChange", params); err != nil {
		return fmt.Errorf("lsp changeFile: %w", err)
	}

	logging.Debug("lsp didChange sent", "server", server.serverName, "file", filePath)
	return nil
}

// SaveFile sends a textDocument/didSave notification to the appropriate LSP server.
func (m *Manager) SaveFile(filePath string) error {
	server := m.GetServerForFile(filePath)
	if server == nil {
		return nil
	}

	server.mu.Lock()
	isRunning := server.transport != nil && !server.crashed && !server.startFailed
	server.mu.Unlock()

	if !isRunning {
		return nil
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("lsp saveFile abs: %w", err)
	}
	fileURI := fileURIToString(absPath)

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": fileURI,
		},
	}

	if err := server.SendNotification("textDocument/didSave", params); err != nil {
		return fmt.Errorf("lsp saveFile: %w", err)
	}

	logging.Debug("lsp didSave sent", "server", server.serverName, "file", filePath)
	return nil
}

// CloseFile sends a textDocument/didClose notification and removes the file from tracking.
func (m *Manager) CloseFile(filePath string) error {
	server := m.GetServerForFile(filePath)
	if server == nil {
		return nil
	}

	server.mu.Lock()
	isRunning := server.transport != nil && !server.crashed && !server.startFailed
	server.mu.Unlock()

	if !isRunning {
		return nil
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("lsp closeFile abs: %w", err)
	}
	fileURI := fileURIToString(absPath)

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": fileURI,
		},
	}

	if err := server.SendNotification("textDocument/didClose", params); err != nil {
		return fmt.Errorf("lsp closeFile: %w", err)
	}

	m.mu.Lock()
	delete(m.openedFiles, fileURI)
	m.mu.Unlock()

	logging.Debug("lsp didClose sent", "server", server.serverName, "file", filePath)
	return nil
}

// IsFileOpen returns whether the file has been opened on an LSP server.
func (m *Manager) IsFileOpen(filePath string) bool {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	fileURI := fileURIToString(absPath)

	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.openedFiles[fileURI]
	return ok
}

// SendRequest sends a request to the LSP server for the given file and unmarshals the result.
func (m *Manager) SendRequest(filePath, method string, params, result interface{}) error {
	server, err := m.EnsureStarted(filePath)
	if err != nil {
		return err
	}
	if server == nil {
		return fmt.Errorf("lsp sendRequest: no server for file %q", filePath)
	}

	if err := server.SendRequest(method, params, result); err != nil {
		return fmt.Errorf("lsp sendRequest %s: %w", method, err)
	}

	return nil
}

// fileURIToString converts a local filesystem path to a file:// URI string.
func fileURIToString(p string) string {
	u := &url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(p),
	}
	return u.String()
}
