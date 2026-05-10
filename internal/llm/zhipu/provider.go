package zhipu

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
	llm.RegisterFactory("zhipu", func(cfg llm.ProviderConfig) (llm.Provider, error) {
		if cfg.BaseURL == "" {
			cfg.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
		}
		return NewProvider(cfg), nil
	})
}

func NewProvider(cfg llm.ProviderConfig) *Provider {
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}
	return &Provider{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:    10,
				IdleConnTimeout: 30 * time.Second,
			},
		},
	}
}

func (p *Provider) Name() string { return "zhipu" }

func (p *Provider) IsAvailable() bool {
	return p.config.APIKey != ""
}

// openAIRequest OpenAI 兼容请求体
type openAIRequest struct {
	Model       string              `json:"model"`
	Messages    []llm.Message       `json:"messages"`
	Temperature float64             `json:"temperature,omitempty"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	TopP        float64             `json:"top_p,omitempty"`
	Stream      bool                `json:"stream"`
	Tools       []llm.ToolDefinition `json:"tools,omitempty"`
}

// openAIResponse OpenAI 兼容响应体
type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      llm.Message `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *Provider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	apiReq := openAIRequest{
		Model:       req.Model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      false,
		Tools:       req.Tools,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return nil, fmt.Errorf("zhipu: marshal request failed: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("zhipu: create request failed: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("zhipu: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("zhipu: read response failed: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("zhipu: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("zhipu: parse response failed: %w", err)
	}

	var msg llm.Message
	if len(apiResp.Choices) > 0 {
		msg = apiResp.Choices[0].Message
	}
	if msg.Role == "" {
		msg.Role = "assistant"
	}

	return &llm.ChatResponse{
		Message:      msg,
		Model:        apiResp.Model,
		InputTokens:  apiResp.Usage.PromptTokens,
		OutputTokens: apiResp.Usage.CompletionTokens,
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
		"glm-4-plus",
		"glm-4-flash",
	}, nil
}
