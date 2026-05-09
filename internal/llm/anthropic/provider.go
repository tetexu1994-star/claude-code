package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tetexu/claude-code/internal/llm"
)

type Provider struct {
	config    llm.ProviderConfig
	client    *http.Client
}

func init() {
	llm.Register("anthropic", func(cfg llm.ProviderConfig) llm.LLMProvider {
		return NewProvider(cfg)
	})
}

func NewProvider(cfg llm.ProviderConfig) *Provider {
	return &Provider{
		config: cfg,
		client: &http.Client{
			Timeout: 120 * time.Second,
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

func (p *Provider) Chat(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	apiReq := map[string]interface{}{
		"model":       req.Model,
		"max_tokens":  req.MaxTokens,
		"messages":    req.Messages,
		"temperature": req.Temperature,
	}
	if req.System != "" {
		apiReq["system"] = req.System
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

	var apiResp struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("anthropic: parse response failed: %w", err)
	}

	content := ""
	if len(apiResp.Content) > 0 {
		content = apiResp.Content[0].Text
	}

	return &llm.ChatResponse{
		Content:      content,
		InputTokens:  apiResp.Usage.InputTokens,
		OutputTokens: apiResp.Usage.OutputTokens,
	}, nil
}
