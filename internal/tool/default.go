package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

)

// sharedTaskManager is the single TaskManager shared across Agent and Task tools.
var sharedTaskManager = NewTaskManager()

// SharedTaskManager returns the package-level TaskManager shared by Agent and Task tools.
func SharedTaskManager() *TaskManager {
	return sharedTaskManager
}

// DefaultRegistry returns a Registry pre-populated with the standard set
// of built-in tools.
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	for _, t := range DefaultTools() {
		_ = reg.Register(t)
	}
	return reg
}

// DefaultTools returns the default built-in tool instances.
func DefaultTools() []Tool {
	return []Tool{
		&defaultBashTool{timeout: 120 * time.Second},
		&defaultFileReadTool{},
		&defaultFileWriteTool{},
		&defaultFileEditTool{},
		&defaultGlobTool{},
		&defaultGrepTool{},
		&WebFetchTool{},
		&WebSearchTool{},
		NewTodoWriteTool(),
		NewAgentTool(sharedTaskManager),
		NewTaskCreateTool(sharedTaskManager),
		NewTaskGetTool(sharedTaskManager),
		NewTaskListTool(sharedTaskManager),
		NewTaskStopTool(sharedTaskManager),
		NewEnterPlanModeTool(),
		NewExitPlanModeTool(),
	}
}

// --- Bash ---

type defaultBashTool struct {
	timeout time.Duration
}

func (t *defaultBashTool) Name() string { return "bash" }
func (t *defaultBashTool) Description() string {
	return "Execute a shell command in a sandboxed environment. Returns stdout, stderr, and exit code."
}
func (t *defaultBashTool) IsEnabled() bool        { return true }
func (t *defaultBashTool) IsConcurrencySafe() bool { return false }
func (t *defaultBashTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {
      "type": "string",
      "description": "The shell command to execute"
    }
  },
  "required": ["command"]
}`)
	return ToolDefinition{
		Name:        "bash",
		Description: t.Description(),
		InputSchema: schema,
	}
}

func (t *defaultBashTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}

	cmdCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", params.Command)
	out, err := cmd.CombinedOutput()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return &ToolResult{
				Content: strings.TrimSpace(string(out)),
				IsError: exitErr.ExitCode() != 0,
			}, nil
		}
		return &ToolResult{IsError: true, Content: err.Error()}, nil
	}
	return &ToolResult{Content: strings.TrimSpace(string(out))}, nil
}

// --- FileRead ---

type defaultFileReadTool struct{}

func (t *defaultFileReadTool) Name() string        { return "read_file" }
func (t *defaultFileReadTool) Description() string { return "Read the contents of a file." }
func (t *defaultFileReadTool) IsEnabled() bool     { return true }
func (t *defaultFileReadTool) IsConcurrencySafe() bool { return true }
func (t *defaultFileReadTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute path to the file"
    },
    "offset": {
      "type": "integer",
      "description": "The line number to start reading from"
    },
    "limit": {
      "type": "integer",
      "description": "The number of lines to read"
    }
  },
  "required": ["path"]
}`)
	return ToolDefinition{Name: "read_file", Description: t.Description(), InputSchema: schema}
}

func (t *defaultFileReadTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	data, err := os.ReadFile(params.Path)
	if err != nil {
		return &ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// Apply offset/limit if specified.
	if params.Offset > 0 || params.Limit > 0 {
		lines := strings.Split(string(data), "\n")
		if params.Offset > 0 {
			if params.Offset >= len(lines) {
				return &ToolResult{Content: ""}, nil
			}
			lines = lines[params.Offset:]
		}
		if params.Limit > 0 && params.Limit < len(lines) {
			lines = lines[:params.Limit]
		}
		return &ToolResult{Content: strings.Join(lines, "\n")}, nil
	}
	return &ToolResult{Content: string(data)}, nil
}

// --- FileWrite ---

type defaultFileWriteTool struct{}

func (t *defaultFileWriteTool) Name() string      { return "write_file" }
func (t *defaultFileWriteTool) Description() string {
	return "Write content to a file. Creates parent directories if needed."
}
func (t *defaultFileWriteTool) IsEnabled() bool         { return true }
func (t *defaultFileWriteTool) IsConcurrencySafe() bool { return false }
func (t *defaultFileWriteTool) ToolDefinition() ToolDefinition {
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
	return ToolDefinition{Name: "write_file", Description: t.Description(), InputSchema: schema}
}

func (t *defaultFileWriteTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	dir := filepath.Dir(params.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &ToolResult{IsError: true, Content: err.Error()}, nil
	}
	if err := os.WriteFile(params.Path, []byte(params.Content), 0644); err != nil {
		return &ToolResult{IsError: true, Content: err.Error()}, nil
	}
	return &ToolResult{Content: fmt.Sprintf("wrote %d bytes to %s", len(params.Content), params.Path)}, nil
}

// --- FileEdit ---

type defaultFileEditTool struct{}

func (t *defaultFileEditTool) Name() string      { return "edit_file" }
func (t *defaultFileEditTool) Description() string {
	return "Perform exact string replacements in an existing file."
}
func (t *defaultFileEditTool) IsEnabled() bool         { return true }
func (t *defaultFileEditTool) IsConcurrencySafe() bool { return false }
func (t *defaultFileEditTool) ToolDefinition() ToolDefinition {
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
	return ToolDefinition{Name: "edit_file", Description: t.Description(), InputSchema: schema}
}

func (t *defaultFileEditTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if params.OldString == params.NewString {
		return &ToolResult{IsError: true, Content: "old_string and new_string must be different"}, nil
	}

	data, err := os.ReadFile(params.Path)
	if err != nil {
		return &ToolResult{IsError: true, Content: err.Error()}, nil
	}
	content := string(data)

	if params.ReplaceAll {
		if !strings.Contains(content, params.OldString) {
			return &ToolResult{IsError: true, Content: "old_string not found in file"}, nil
		}
		content = strings.ReplaceAll(content, params.OldString, params.NewString)
	} else {
		idx := strings.Index(content, params.OldString)
		if idx < 0 {
			return &ToolResult{IsError: true, Content: "old_string not found in file"}, nil
		}
		content = content[:idx] + params.NewString + content[idx+len(params.OldString):]
	}

	if err := os.WriteFile(params.Path, []byte(content), 0644); err != nil {
		return &ToolResult{IsError: true, Content: err.Error()}, nil
	}
	return &ToolResult{Content: fmt.Sprintf("edited %s", params.Path)}, nil
}

// --- Glob ---

type defaultGlobTool struct{}

func (t *defaultGlobTool) Name() string        { return "Glob" }
func (t *defaultGlobTool) Description() string { return "Fast file pattern matching tool that works with any codebase size." }
func (t *defaultGlobTool) IsEnabled() bool     { return true }
func (t *defaultGlobTool) IsConcurrencySafe() bool { return true }
func (t *defaultGlobTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "The glob pattern to match files against"
    },
    "path": {
      "type": "string",
      "description": "The directory to search in"
    }
  },
  "required": ["pattern"]
}`)
	return ToolDefinition{Name: "Glob", Description: t.Description(), InputSchema: schema}
}

func (t *defaultGlobTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	searchPath := params.Path
	if searchPath == "" {
		searchPath = "."
	}

	// Support ** patterns via recursive walk.
	if strings.Contains(params.Pattern, "**") {
		matches, err := globRecursive(searchPath, params.Pattern)
		if err != nil {
			return &ToolResult{IsError: true, Content: err.Error()}, nil
		}
		if len(matches) == 0 {
			return &ToolResult{Content: "no matches"}, nil
		}
		return &ToolResult{Content: strings.Join(matches, "\n")}, nil
	}

	matches, err := filepath.Glob(filepath.Join(searchPath, params.Pattern))
	if err != nil {
		return &ToolResult{IsError: true, Content: err.Error()}, nil
	}
	if len(matches) == 0 {
		return &ToolResult{Content: "no matches"}, nil
	}
	return &ToolResult{Content: strings.Join(matches, "\n")}, nil
}

// globRecursive handles ** patterns by walking the directory tree.
func globRecursive(root, pattern string) ([]string, error) {
	var matches []string
	// Convert glob pattern to a matching function.
	// Split on ** and match prefix/suffix.
	parts := strings.SplitN(pattern, "**", 2)
	prefix := parts[0]
	var suffix string
	if len(parts) > 1 {
		suffix = parts[1]
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			// Skip hidden directories and common non-source dirs.
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != root {
				return fs.SkipDir
			}
			if base == "node_modules" || base == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		// Check prefix match before the **.
		if prefix != "" && !strings.HasPrefix(rel, prefix) {
			return nil
		}
		// Check suffix match after the **.
		if suffix != "" {
			trimmed := rel
			if prefix != "" {
				trimmed = strings.TrimPrefix(rel, prefix)
			}
			matched, _ := filepath.Match(suffix, trimmed)
			if !matched {
				return nil
			}
		}
		matches = append(matches, rel)
		return nil
	})
	return matches, err
}

// --- Grep ---

type defaultGrepTool struct{}

func (t *defaultGrepTool) Name() string        { return "Grep" }
func (t *defaultGrepTool) Description() string { return "A powerful search tool for searching file contents with regex support." }
func (t *defaultGrepTool) IsEnabled() bool     { return true }
func (t *defaultGrepTool) IsConcurrencySafe() bool { return true }
func (t *defaultGrepTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {
      "type": "string",
      "description": "The regular expression pattern to search for in file contents"
    },
    "path": {
      "type": "string",
      "description": "File or directory to search in (default: .)"
    },
    "include": {
      "type": "string",
      "description": "Glob pattern to filter files (e.g. *.go)"
    }
  },
  "required": ["pattern"]
}`)
	return ToolDefinition{Name: "Grep", Description: t.Description(), InputSchema: schema}
}

func (t *defaultGrepTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
		Include string `json:"include"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	searchPath := params.Path
	if searchPath == "" {
		searchPath = "."
	}

	re, err := regexp.Compile(params.Pattern)
	if err != nil {
		// Fall back to literal substring search if regex is invalid.
		re = nil
	}

	var includeRe *regexp.Regexp
	if params.Include != "" {
		// Convert glob to regex.
		globRe := globToRegex(params.Include)
		includeRe = regexp.MustCompile(globRe)
	}

	var results []string
	maxResults := 250
	maxFileSize := int64(1024 * 1024) // 1MB per file

	err = filepath.WalkDir(searchPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if strings.HasPrefix(base, ".") && path != searchPath {
				return fs.SkipDir
			}
			if base == "node_modules" || base == ".git" {
				return fs.SkipDir
			}
			return nil
		}

		// Check include filter.
		if includeRe != nil && !includeRe.MatchString(filepath.Base(path)) {
			return nil
		}

		info, err := d.Info()
		if err != nil || info.Size() > maxFileSize {
			return nil
		}

		// Skip binary files.
		if isBinaryPath(path) {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		lineNum := 0
		relPath, _ := filepath.Rel(searchPath, path)

		for scanner.Scan() && len(results) < maxResults {
			lineNum++
			line := scanner.Text()

			matched := false
			if re != nil {
				matched = re.MatchString(line)
			} else {
				matched = strings.Contains(line, params.Pattern)
			}
			if !matched {
				continue
			}

			if len(results) >= maxResults {
				break
			}
			results = append(results, fmt.Sprintf("%s:%d: %s", relPath, lineNum, line))
		}
		return nil
	})

	if err != nil {
		return &ToolResult{IsError: true, Content: err.Error()}, nil
	}

	if len(results) == 0 {
		return &ToolResult{Content: "no matches"}, nil
	}
	return &ToolResult{Content: strings.Join(results, "\n")}, nil
}

// globToRegex converts a simple glob pattern to a regex.
func globToRegex(pattern string) string {
	s := regexp.QuoteMeta(pattern)
	s = strings.ReplaceAll(s, `\*`, ".*")
	s = strings.ReplaceAll(s, `\?`, ".")
	return "^" + s + "$"
}

// isBinaryPath checks if a file path looks like a binary/non-text file.
func isBinaryPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".o": true, ".a": true, ".obj": true,
		".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
		".pyc": true, ".class": true, ".wasm": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
	}
	return binaryExts[ext]
}
