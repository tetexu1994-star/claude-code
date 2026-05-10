package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// WebSearchTool performs a web search and returns results.
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string        { return "WebSearch" }
func (t *WebSearchTool) Description() string { return "Search the web and return results." }
func (t *WebSearchTool) IsEnabled() bool     { return true }
func (t *WebSearchTool) IsConcurrencySafe() bool { return true }

func (t *WebSearchTool) ToolDefinition() ToolDefinition {
	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "The search query"
    },
    "allowed_domains": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Only include results from these domains"
    },
    "blocked_domains": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Never include results from these domains"
    }
  },
  "required": ["query"]
}`)
	return ToolDefinition{Name: "WebSearch", Description: t.Description(), InputSchema: schema}
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

var (
	ddgResultRe = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
	ddgDescRe   = regexp.MustCompile(`<td class="result-snippet"[^>]*>([^<]*)</td>`)
)

func (t *WebSearchTool) Execute(ctx context.Context, input json.RawMessage, toolCtx *ToolContext) (*ToolResult, error) {
	var params struct {
		Query          string   `json:"query"`
		AllowedDomains []string `json:"allowed_domains"`
		BlockedDomains []string `json:"blocked_domains"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("invalid input: %v", err)}, nil
	}
	if params.Query == "" {
		return &ToolResult{IsError: true, Content: "query is required"}, nil
	}

	results, err := searchDuckDuckGo(ctx, params.Query)
	if err != nil {
		return &ToolResult{IsError: true, Content: fmt.Sprintf("search failed: %v", err)}, nil
	}

	if len(results) == 0 {
		return &ToolResult{Content: "no results found"}, nil
	}

	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, r.Title, r.URL))
		sb.WriteString(fmt.Sprintf("   %s\n\n", r.Snippet))
	}

	return &ToolResult{Content: strings.TrimSpace(sb.String())}, nil
}

func searchDuckDuckGo(ctx context.Context, query string) ([]searchResult, error) {
	searchURL := "https://lite.duckduckgo.com/lite/"

	data := url.Values{}
	data.Set("q", query)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "TlaudeCode/1.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if err != nil {
		return nil, err
	}

	return parseDuckDuckGoLite(string(body)), nil
}

func parseDuckDuckGoLite(html string) []searchResult {
	var results []searchResult

	linkMatches := ddgResultRe.FindAllStringSubmatch(html, -1)
	descMatches := ddgDescRe.FindAllStringSubmatch(html, -1)

	for i, lm := range linkMatches {
		if len(lm) < 3 {
			continue
		}
		href := strings.TrimSpace(lm[1])
		title := strings.TrimSpace(htmlToText(lm[2]))

		if href == "" || strings.HasPrefix(href, "//duckduckgo.com") || strings.HasPrefix(href, "/") {
			continue
		}

		if !strings.HasPrefix(href, "http") {
			href = "https:" + href
		}

		snippet := ""
		if i < len(descMatches) && len(descMatches[i]) > 1 {
			snippet = strings.TrimSpace(htmlToText(descMatches[i][1]))
		}

		results = append(results, searchResult{
			Title:   title,
			URL:     href,
			Snippet: snippet,
		})

		if len(results) >= 10 {
			break
		}
	}

	return results
}
