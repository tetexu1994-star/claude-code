package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// WebFetchTool fetches content from a URL and returns it as text.
type WebFetchTool struct{}

func (t *WebFetchTool) Name() string        { return "WebFetch" }
func (t *WebFetchTool) Description() string { return "Fetch content from a URL and process into markdown." }
func (t *WebFetchTool) IsEnabled() bool     { return true }
func (t *WebFetchTool) IsConcurrencySafe() bool { return true }

func (t *WebFetchTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "url": {
      "type": "string",
      "description": "The URL to fetch content from"
    },
    "prompt": {
      "type": "string",
      "description": "The prompt to run on the fetched content"
    }
  },
  "required": ["url", "prompt"]
}`)
	return ToolDefinition{Name: "WebFetch", Description: t.Description(), InputSchema: schema}
}

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	htmlEntityRe = regexp.MustCompile(`&[a-zA-Z]+;`)
	multiSpaceRe = regexp.MustCompile(`\n\s*\n`)
)

func (t *WebFetchTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		URL    string `json:"url"`
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if params.URL == "" {
		return &ToolResult{IsError: true, Content: "url is required"}, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.URL, nil)
	if err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("failed to create request: %v", err)}, nil
	}
	req.Header.Set("User-Agent", "TlaudeCode/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("fetch failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, resp.Status)}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024)) // 5MB limit
	if err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("read failed: %v", err)}, nil
	}

	content := htmlToText(string(body))
	return &ToolResult{Content: content}, nil
}

func htmlToText(html string) string {
	text := htmlTagRe.ReplaceAllString(html, "")
	text = htmlEntityRe.ReplaceAllString(text, "")
	text = multiSpaceRe.ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)
	return text
}
