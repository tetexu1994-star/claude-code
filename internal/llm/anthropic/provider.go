package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
)

type Provider struct {
	config llm.ProviderConfig
	client *http.Client
}

func init() {
	llm.RegisterFactory("anthropic", func(cfg llm.ProviderConfig) (llm.Provider, error) {
		return NewProvider(cfg), nil
	})
}

func NewProvider(cfg llm.ProviderConfig) *Provider {
	return &Provider{
		config: cfg,
		client: &http.Client{
			Timeout: time.Duration(cfg.Timeout),
			Transport: &http.Transport{
				MaxIdleConns:    10,
				IdleConnTimeout: 30 * time.Second,
			},
		},
	}
}

func (p *Provider) Name() string { return "anthropic" }

func (p *Provider) IsAvailable() bool {
	return p.config.APIKey != "" && p.config.BaseURL != ""
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []llm.Message   `json:"messages"`
	System      string          `json:"system,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
	Tools       []anthropicTool `json:"tools,omitempty"`
}

type anthropicContentBlock struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text,omitempty"`
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

func (p *Provider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	apiReq := anthropicRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		Stream:      false,
	}
	if req.System != "" {
		apiReq.System = req.System
	}
	if len(req.Tools) > 0 {
		apiReq.Tools = make([]anthropicTool, len(req.Tools))
		for i, t := range req.Tools {
			apiReq.Tools[i] = anthropicTool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			}
		}
	}

	body, _ := json.Marshal(apiReq)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("anthropic: parse response failed: %w", err)
	}

	content := ""
	var toolCalls []llm.ToolCall
	for _, block := range apiResp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   block.ID,
				Name: block.Name,
				Args: block.Input,
			})
		}
	}

	return &llm.ChatResponse{
		Message: llm.Message{
			Role:      "assistant",
			Content:   content,
			ToolCalls: toolCalls,
		},
		Model:        apiResp.Model,
		InputTokens:  apiResp.Usage.InputTokens,
		OutputTokens: apiResp.Usage.OutputTokens,
	}, nil
}

func (p *Provider) ChatStream(ctx context.Context, req llm.ChatRequest) (<-chan llm.Chunk, error) {
	ch := make(chan llm.Chunk)
	go func() {
		defer close(ch)
		resp, err := p.Chat(ctx, req)
		if err != nil {
			ch <- llm.Chunk{Error: err}
			return
		}
		if len(resp.Message.ToolCalls) > 0 {
			ch <- llm.Chunk{Done: true, ToolCalls: resp.Message.ToolCalls}
		} else {
			ch <- llm.Chunk{Content: resp.Message.Content}
			ch <- llm.Chunk{Done: true}
		}
	}()
	return ch, nil
}

func (p *Provider) Models() ([]string, error) {
	return []string{
		"claude-sonnet-4-20250514",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}, nil
}
