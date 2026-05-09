package llm

import (
	"context"
	"time"
)

// ChatRequest 表示一次聊天请求
type ChatRequest struct {
	Messages []Message `json:"messages"`
	Model    string    `json:"model"`
	Stream   bool      `json:"stream"`
	// 可选参数
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
}

// Message 表示一条聊天消息
type Message struct {
	Role    string `json:"role"` // "user", "assistant", "system"
	Content string `json:"content"`
}

// ChatResponse 表示一次聊天响应
type ChatResponse struct {
	Message Message `json:"message"`
	Model   string  `json:"model"`
	Usage   *Usage  `json:"usage,omitempty"`
}

// Usage 表示 token 用量统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Chunk 表示流式响应的一个片段
type Chunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
	Error   error  `json:"-"`
}

// Provider 是所有 LLM 提供者必须实现的接口
type Provider interface {
	// Name 返回提供者名称
	Name() string

	// IsAvailable 返回提供者当前是否可用
	IsAvailable() bool

	// Chat 发送一次聊天请求并获取完整响应
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// Stream 发送一次聊天请求并获取流式响应
	Stream(ctx context.Context, req ChatRequest) (<-chan Chunk, error)

	// Models 返回该提供者支持的模型列表
	Models() ([]string, error)
}

// ProviderConfig 提供者通用配置
type ProviderConfig struct {
	// APIKey API 密钥
	APIKey string `yaml:"api_key" json:"api_key"`
	// BaseURL API 基础地址
	BaseURL string `yaml:"base_url" json:"base_url"`
	// Timeout 请求超时时间
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
	// Model 默认模型
	Model string `yaml:"model" json:"model"`
	// Proxy 代理地址（可选）
	Proxy string `yaml:"proxy,omitempty" json:"proxy,omitempty"`
	// Priority 优先级（数字越小优先级越高）
	Priority int `yaml:"priority" json:"priority"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() ProviderConfig {
	return ProviderConfig{
		Timeout:  time.Second * 60,
		Priority: 100,
	}
}
