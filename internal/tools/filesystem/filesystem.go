package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		AllowedPaths: []string{"/Users/tetexu/class-claude-code"},
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
