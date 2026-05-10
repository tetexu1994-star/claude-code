package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tetexu/tlaude-code/internal/tool"
)

type ReadResult struct {
	Content string `json:"content"`
	Size    int    `json:"size"`
}

type WriteResult struct {
	Path string `json:"path"`
	Size int    `json:"size"`
}

type ListResult struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
}

type Entry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

type Tool struct {
	Name         string
	Description  string
	Enabled      bool
	AllowedPaths []string
}

func NewTool() *Tool {
	return &Tool{
		Name:        "filesystem",
		Description: "Read, write, list files and directories",
		Enabled:     true,
		AllowedPaths: []string{"/Users/tetexu/tlaude-code"},
	}
}

func (t *Tool) isPathAllowed(target string) bool {
	if len(t.AllowedPaths) == 0 {
		return true
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	for _, allowed := range t.AllowedPaths {
		allowedAbs, err := filepath.Abs(allowed)
		if err != nil {
			continue
		}
		if strings.HasPrefix(abs, allowedAbs) {
			return true
		}
	}
	return false
}

func (t *Tool) ReadFile(ctx context.Context, path string) (*ReadResult, error) {
	if !t.isPathAllowed(path) {
		return nil, fmt.Errorf("path not allowed: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file failed: %w", err)
	}
	return &ReadResult{
		Content: string(data),
		Size:    len(data),
	}, nil
}

func (t *Tool) WriteFile(ctx context.Context, path string, content string) (*WriteResult, error) {
	if !t.isPathAllowed(path) {
		return nil, fmt.Errorf("path not allowed: %s", path)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create dir failed: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("write file failed: %w", err)
	}
	return &WriteResult{
		Path: path,
		Size: len(content),
	}, nil
}

func (t *Tool) ListDir(ctx context.Context, path string) (*ListResult, error) {
	if !t.isPathAllowed(path) {
		return nil, fmt.Errorf("path not allowed: %s", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("list dir failed: %w", err)
	}
	result := &ListResult{}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result.Entries = append(result.Entries, Entry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}
	result.Total = len(result.Entries)
	return result, nil
}

func (t *Tool) DeleteFile(ctx context.Context, path string) error {
	if !t.isPathAllowed(path) {
		return fmt.Errorf("path not allowed: %s", path)
	}
	return os.Remove(path)
}

// --- Tool interface adapters (split tools) ---

// FileReadTool implements tool.Tool for reading files.
type FileReadTool struct {
	inner *Tool
}

// NewFileReadTool creates a FileReadTool implementing tool.Tool.
func NewFileReadTool() *FileReadTool {
	return &FileReadTool{inner: NewTool()}
}

func (t *FileReadTool) Name() string        { return "read_file" }
func (t *FileReadTool) Description() string { return "Read the contents of a file." }
func (t *FileReadTool) IsEnabled() bool     { return t.inner.Enabled }
func (t *FileReadTool) IsConcurrencySafe() bool {
	return true
}

func (t *FileReadTool) ToolDefinition() tool.ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute path to the file"
    }
  },
  "required": ["path"]
}`)
	return tool.ToolDefinition{
		Name:        "read_file",
		Description: t.Description(),
		InputSchema: schema,
	}
}

func (t *FileReadTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &tool.ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	result, err := t.inner.ReadFile(ctx, params.Path)
	if err != nil {
		return &tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	return &tool.ToolResult{Content: result.Content}, nil
}

// FileWriteTool implements tool.Tool for writing files.
type FileWriteTool struct {
	inner *Tool
}

// NewFileWriteTool creates a FileWriteTool implementing tool.Tool.
func NewFileWriteTool() *FileWriteTool {
	return &FileWriteTool{inner: NewTool()}
}

func (t *FileWriteTool) Name() string        { return "write_file" }
func (t *FileWriteTool) Description() string { return "Write content to a file. Creates parent directories if needed." }
func (t *FileWriteTool) IsEnabled() bool     { return t.inner.Enabled }
func (t *FileWriteTool) IsConcurrencySafe() bool {
	return false
}

func (t *FileWriteTool) ToolDefinition() tool.ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute path to the file"
    },
    "content": {
      "type": "string",
      "description": "Content to write"
    }
  },
  "required": ["path", "content"]
}`)
	return tool.ToolDefinition{
		Name:        "write_file",
		Description: t.Description(),
		InputSchema: schema,
	}
}

func (t *FileWriteTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &tool.ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	result, err := t.inner.WriteFile(ctx, params.Path, params.Content)
	if err != nil {
		return &tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	return &tool.ToolResult{Content: fmt.Sprintf("wrote %d bytes to %s", result.Size, result.Path)}, nil
}

// FileEditTool implements tool.Tool for exact string replacements in files.
type FileEditTool struct {
	inner *Tool
}

// NewFileEditTool creates a FileEditTool implementing tool.Tool.
func NewFileEditTool() *FileEditTool {
	return &FileEditTool{inner: NewTool()}
}

func (t *FileEditTool) Name() string        { return "edit_file" }
func (t *FileEditTool) Description() string { return "Perform exact string replacements in an existing file." }
func (t *FileEditTool) IsEnabled() bool     { return t.inner.Enabled }
func (t *FileEditTool) IsConcurrencySafe() bool {
	return false
}

func (t *FileEditTool) ToolDefinition() tool.ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute path to the file to modify"
    },
    "old_string": {
      "type": "string",
      "description": "The text to replace"
    },
    "new_string": {
      "type": "string",
      "description": "The text to replace it with (must be different from old_string)"
    },
    "replace_all": {
      "type": "boolean",
      "description": "Replace all occurrences of old_string (default false)",
      "default": false
    }
  },
  "required": ["path", "old_string", "new_string"]
}`)
	return tool.ToolDefinition{
		Name:        "edit_file",
		Description: t.Description(),
		InputSchema: schema,
	}
}

func (t *FileEditTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *tool.ToolContext) (*tool.ToolResult, error) {
	var params struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &tool.ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if params.OldString == params.NewString {
		return &tool.ToolResult{IsError: true, Content: "old_string and new_string must be different"}, nil
	}

	result, err := t.inner.ReadFile(ctx, params.Path)
	if err != nil {
		return &tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}

	content := result.Content
	if params.ReplaceAll {
		content, err = replaceAllStr(content, params.OldString, params.NewString)
	} else {
		content, err = replaceOnceStr(content, params.OldString, params.NewString)
	}
	if err != nil {
		return &tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}

	_, err = t.inner.WriteFile(ctx, params.Path, content)
	if err != nil {
		return &tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	return &tool.ToolResult{Content: fmt.Sprintf("edited %s", params.Path)}, nil
}

func replaceOnceStr(s, old, newStr string) (string, error) {
	idx := strings.Index(s, old)
	if idx < 0 {
		return "", fmt.Errorf("old_string not found in file")
	}
	return s[:idx] + newStr + s[idx+len(old):], nil
}

func replaceAllStr(s, old, newStr string) (string, error) {
	if !strings.Contains(s, old) {
		return "", fmt.Errorf("old_string not found in file")
	}
	return strings.ReplaceAll(s, old, newStr), nil
}
