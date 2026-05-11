package llm

import (
	"context"
	"time"
)

// ToolCall represents an LLM-requested tool invocation.
type ToolCall struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Args   map[string]interface{} `json:"args"`
	Result string                 `json:"result,omitempty"`
}

// Message 聊天消息
type Message struct {
	Role      string     `json:"role"` // "user", "assistant", "system", "tool"
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolID    string     `json:"tool_id,omitempty"` // for tool result messages
}

// ToolDefinition 描述一个可用的工具
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	System      string           `json:"system,omitempty"`
	Stream      bool             `json:"stream"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	TopP        float64          `json:"top_p,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Message      Message `json:"message"`
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
}

// Usage tracks token consumption for a single API call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Chunk 流式响应片段
type Chunk struct {
	Content   string     `json:"content"`
	Done      bool       `json:"done"`
	Error     error      `json:"-"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Provider LLM 提供者接口
type Provider interface {
	Name() string
	IsAvailable() bool
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	ChatStream(ctx context.Context, req ChatRequest) (<-chan Chunk, error)
	Models() ([]string, error)
}

// ProviderConfig 提供者配置
type ProviderConfig struct {
	APIKey   string        `yaml:"api_key" json:"api_key"`
	BaseURL  string        `yaml:"base_url" json:"base_url"`
	Timeout  time.Duration `yaml:"timeout" json:"timeout"`
	Model    string        `yaml:"model" json:"model"`
	Proxy    string        `yaml:"proxy,omitempty" json:"proxy,omitempty"`
	Priority int           `yaml:"priority" json:"priority"`
}

// DefaultConfig 返回默认 ProviderConfig
func DefaultConfig() ProviderConfig {
	return ProviderConfig{
		Timeout:  60 * time.Second,
		Priority: 100,
	}
}

// ProviderFactory 创建 Provider 的函数类型
type ProviderFactory func(cfg ProviderConfig) (Provider, error)

var factoryRegistry = make(map[string]ProviderFactory)

// RegisterFactory 注册 Provider 工厂
func RegisterFactory(name string, factory ProviderFactory) {
	factoryRegistry[name] = factory
}

// GetFactory 获取 Provider 工厂
func GetFactory(name string) (ProviderFactory, bool) {
	f, ok := factoryRegistry[name]
	return f, ok
}

