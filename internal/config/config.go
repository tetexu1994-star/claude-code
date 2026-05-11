package config

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tetexu/tlaude-code/internal/logging"
	"github.com/tetexu/tlaude-code/internal/moa"
	"github.com/tetexu/tlaude-code/internal/sandbox"
)

// providerEnvVars maps provider names to their environment variable names.
var providerEnvVars = map[string]string{
	"anthropic":   "ANTHROPIC_API_KEY",
	"openai":      "OPENAI_API_KEY",
	"deepseek":    "DEEPSEEK_API_KEY",
	"openrouter":  "OPENROUTER_API_KEY",
	"siliconflow": "SILICONFLOW_API_KEY",
	"tongyi":      "DASHSCOPE_API_KEY",
	"zhipu":       "ZHIPU_API_KEY",
}

// providerDefaultModels maps each provider to its default model name.
var providerDefaultModels = map[string]string{
	"anthropic":   "claude-sonnet-4-20250514",
	"deepseek":    "deepseek-chat",
	"openai":      "gpt-4o",
	"openrouter":  "openrouter/auto",
	"siliconflow": "Qwen/Qwen2.5-72B-Instruct",
	"tongyi":      "qwen-plus",
	"zhipu":       "glm-4-plus",
}

// providerBaseURLs maps provider names to their API base URLs for connectivity probes.
var providerBaseURLs = map[string]string{
	"anthropic":   "https://api.anthropic.com",
	"openai":      "https://api.openai.com",
	"deepseek":    "https://api.deepseek.com",
	"openrouter":  "https://openrouter.ai",
	"siliconflow": "https://api.siliconflow.cn",
	"tongyi":      "https://dashscope.aliyuncs.com",
	"zhipu":       "https://open.bigmodel.cn",
}

// SafetyMode controls tool-execution approval behavior.
type SafetyMode string

const (
	SafetyModeAsk    SafetyMode = "ask"    // prompt for every tool call (default)
	SafetyModeAllow  SafetyMode = "allow"  // auto-approve all tool calls
	SafetyModeReject SafetyMode = "reject" // auto-reject all tool calls
)

// AgentMoAConfig holds MoA settings for the agent system.
type AgentMoAConfig struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	DefaultStrategy string `yaml:"default_strategy" json:"default_strategy"`
	AutoSelect      bool   `yaml:"auto_select" json:"auto_select"`
}

// AgentBackendConfig holds configuration for an external agent backend process.
type AgentBackendConfig struct {
	Command string   `yaml:"command" json:"command"` // CLI command or path
	Args    []string `yaml:"args" json:"args"`       // base arguments
	Timeout int      `yaml:"timeout" json:"timeout"` // seconds
}

// PluginConfig holds plugin system configuration.
type PluginConfig struct {
	Dir      string   `yaml:"dir" json:"dir"`
	Enabled  []string `yaml:"enabled" json:"enabled"`
	Disabled []string `yaml:"disabled" json:"disabled"`
}

// AgentConfig holds agent-related configuration.
type AgentConfig struct {
	DefaultAgent string                         `yaml:"default_agent" json:"default_agent"`
	MoA          AgentMoAConfig                 `yaml:"moa" json:"moa"`
	AgentsDir    string                         `yaml:"agents_dir" json:"agents_dir"`
	Backends     map[string]AgentBackendConfig  `yaml:"backends" json:"backends"`
}

// MCPServerConfig 单个 MCP 服务器配置
type MCPServerConfig struct {
	Name    string   `yaml:"name" json:"name"`
	Command string   `yaml:"command,omitempty" json:"command,omitempty"` // stdio 模式
	Args    []string `yaml:"args,omitempty" json:"args,omitempty"`
	URL     string   `yaml:"url,omitempty" json:"url,omitempty"` // SSE 模式
	Enabled bool     `yaml:"enabled" json:"enabled"`
}

// Config 是全局配置结构体
type Config struct {
	Provider             string            `yaml:"provider"`
	Model                string            `yaml:"model"`
	Temperature          float64           `yaml:"temperature"`
	MaxTokens            int               `yaml:"max_tokens"`
	WorkingDirectory     string            `yaml:"working_directory"`
	SessionDir           string            `yaml:"session_dir"`
	APIKeys              map[string]string `yaml:"api_keys"`
	SafetyMode           SafetyMode        `yaml:"safety_mode"`
	AlwaysAllowPatterns  []string          `yaml:"always_allow_patterns"`
	MoA                  moa.MoAConfig     `yaml:"moa" json:"moa"`
	Sandbox              sandbox.Config    `yaml:"sandbox" json:"sandbox"`

	// Cost tracking
	EnableCostTracking bool `yaml:"enable_cost_tracking" json:"enable_cost_tracking"`

	// Smart routing
	SmartRouting bool `yaml:"smart_routing" json:"smart_routing"`

	// Memory search
	EnableMemorySearch bool `yaml:"enable_memory_search" json:"enable_memory_search"`

	// MCP servers
	MCPServers []MCPServerConfig `yaml:"mcp_servers" json:"mcp_servers"`

	// Plugin system
	Plugins PluginConfig `yaml:"plugins" json:"plugins"`

	// Agent system (Phase 2).
	Agent AgentConfig `yaml:"agent" json:"agent"`

	// Context compaction.
	Compact CompactConfig `yaml:"compact" json:"compact"`

	// Memory system.
	Memory MemoryConfig `yaml:"memory" json:"memory"`
}

// CompactConfig holds context compaction settings.
type CompactConfig struct {
	Enabled     bool `yaml:"enabled" json:"enabled"`
	AutoCompact bool `yaml:"auto_compact" json:"auto_compact"`
	TokenBudget int  `yaml:"token_budget" json:"token_budget"`
}

// MemoryConfig holds memory system configuration.
type MemoryConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	BaseDir string `yaml:"base_dir" json:"base_dir"` // empty = default (~/.tlaude-code/memory/)
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	sessionDir := filepath.Join(homeDir, ".tlaude-code", "sessions")

	return &Config{
		Provider:            "anthropic",
		Model:               "claude-sonnet-4-20250514",
		Temperature:         0.7,
		MaxTokens:           4096,
		SessionDir:          sessionDir,
		APIKeys:             make(map[string]string),
		SafetyMode:          SafetyModeAsk,
		AlwaysAllowPatterns: []string{"bash:ls", "bash:cat", "bash:pwd", "bash:echo"},
		MoA: moa.MoAConfig{
			Enabled:       false,
			ProviderNames: []string{},
			Synthesizer:   "",
			TimeoutSec:    30,
			MaxParallel:   3,
			Mode:          "synthesize",
		},
		Sandbox: sandbox.Config{
			Mode:         sandbox.ModeRestricted,
			TimeoutSec:   30,
			MaxMemoryMB:  128,
			AllowNetwork: false,
			AllowWrite:   false,
			TempDir:      "/tmp/tlaude-code-sandbox",
		},
		EnableCostTracking: true,
		SmartRouting:       false,
		EnableMemorySearch: true,
		Plugins: PluginConfig{
			Dir:      filepath.Join(homeDir, ".tlaude-code", "plugins"),
			Enabled:  []string{},
			Disabled: []string{},
		},
		Compact: CompactConfig{
			Enabled:     true,
			AutoCompact: true,
			TokenBudget: 40000,
		},
		Memory: MemoryConfig{
			Enabled: true,
			BaseDir: "",
		},
		Agent: AgentConfig{
			DefaultAgent: "general",
			MoA: AgentMoAConfig{
				Enabled:         false,
				DefaultStrategy: "synthesize",
				AutoSelect:      true,
			},
			AgentsDir: filepath.Join(homeDir, ".tlaude-code", "agents"),
			Backends: map[string]AgentBackendConfig{
				"claude-code": {
					Command: "npx",
					Args:    []string{"@anthropic-ai/claude-code", "--print", "--dangerously-skip-permissions"},
					Timeout: 120,
				},
				"hermes": {
					Command: filepath.Join(homeDir, ".hermes", "hermes-agent", "venv", "bin", "hermes"),
					Args:    []string{"-z"},
					Timeout: 120,
				},
			},
		},
	}
}

// configDir 返回配置目录路径
func configDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".tlaude-code"), nil
}

// configFile 返回配置文件的完整路径
func configFile() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load 从 ~/.tlaude-code/config.yaml 加载配置
// 如果文件不存在，返回默认配置
func Load() (*Config, error) {
	cfg := DefaultConfig()
	cfg.DetectEnvAPIKeys()

	path, err := configFile()
	if err != nil {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return cfg, fmt.Errorf("parsing config file: %w", err)
	}

	// 确保 APIKeys map 已初始化
	if cfg.APIKeys == nil {
		cfg.APIKeys = make(map[string]string)
	}
	cfg.DetectEnvAPIKeys()
	return cfg, nil
}

// Save 保存配置到 ~/.tlaude-code/config.yaml
func (c *Config) Save() error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	path := filepath.Join(dir, "config.yaml")

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// GetAPIKey 获取指定 provider 的 API Key
func (c *Config) GetAPIKey(provider string) string {
	if c.APIKeys == nil {
		return ""
	}
	return c.APIKeys[provider]
}

// SetAPIKey 设置指定 provider 的 API Key
func (c *Config) SetAPIKey(provider, key string) {
	if c.APIKeys == nil {
		c.APIKeys = make(map[string]string)
	}
	c.APIKeys[provider] = key
}

// DetectEnvAPIKeys populates APIKeys from environment variables for any
// provider that does not already have a key set.
func (c *Config) DetectEnvAPIKeys() {
	for name, envVar := range providerEnvVars {
		if c.APIKeys[name] == "" {
			if val := os.Getenv(envVar); val != "" {
				c.APIKeys[name] = val
				logging.Debug("detected API key from env", "provider", name, "env", envVar)
			}
		}
	}
}

// ProbeConnectivity probes the base URL of each provider that has an API key
// configured. Returns a map of provider name to any error encountered.
// Probe failures are warnings only — they do not block operation.
func (c *Config) ProbeConnectivity() map[string]error {
	results := make(map[string]error)
	client := &http.Client{Timeout: 5 * time.Second}

	for name, key := range c.APIKeys {
		if key == "" {
			continue
		}
		baseURL, ok := providerBaseURLs[name]
		if !ok {
			continue
		}

		req, err := http.NewRequest("GET", baseURL, nil)
		if err != nil {
			results[name] = err
			logging.Warn("connectivity probe failed", "provider", name, "error", err)
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			results[name] = err
			logging.Warn("connectivity probe failed", "provider", name, "error", err)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 500 {
			results[name] = fmt.Errorf("server error: %d", resp.StatusCode)
			logging.Warn("connectivity probe server error", "provider", name, "status", resp.StatusCode)
		} else {
			results[name] = nil
			logging.Debug("connectivity probe ok", "provider", name, "status", resp.StatusCode)
		}
	}
	return results
}

// FirstRunWizard 首次运行向导
// 如果没有任何 API Key 配置，提示用户输入
func (c *Config) FirstRunWizard() error {
	// 先探测已通过环境变量配置的 provider 的连通性
	probeResults := c.ProbeConnectivity()
	for name, err := range probeResults {
		if err != nil {
			fmt.Printf("Warning: connectivity probe failed for %s: %v\n", name, err)
		}
	}

	hasKey := false
	for _, key := range c.APIKeys {
		if key != "" {
			hasKey = true
			break
		}
	}
	if hasKey {
		return nil
	}

	// If stdin is not a terminal, skip interactive prompts.
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		return fmt.Errorf("no API keys configured and stdin is not a terminal; set environment variables (e.g. DEEPSEEK_API_KEY) or create ~/.tlaude-code/config.yaml")
	}

	fmt.Println("=== First Run Setup ===")
	fmt.Println("Welcome to Tlaude Code!")
	fmt.Println()
	fmt.Println("You need at least one API key to get started.")
	fmt.Println("Supported providers: anthropic, openai, deepseek, openrouter, siliconflow, tongyi, zhipu")
	fmt.Println()

	providers := []string{"anthropic", "openai", "deepseek", "openrouter", "siliconflow", "tongyi", "zhipu"}
	for _, provider := range providers {
		fmt.Printf("Enter API key for %s (or press Enter to skip): ", provider)
		var key string
		fmt.Scanln(&key)
		if key != "" {
			c.SetAPIKey(provider, key)
			// Use provider-specific defaults for the first key entered.
			if c.Provider == "anthropic" && c.Model == "claude-sonnet-4-20250514" {
				c.Provider = provider
				if m, ok := providerDefaultModels[provider]; ok {
					c.Model = m
				}
			}
		}
	}

	if len(c.APIKeys) == 0 {
		return fmt.Errorf("no API keys configured")
	}

	if err := c.Save(); err != nil {
		return fmt.Errorf("saving config after setup: %w", err)
	}

	fmt.Println()
	fmt.Println("Setup complete! Configuration saved to ~/.tlaude-code/config.yaml")
	return nil
}

// ShouldAutoApprove checks if a tool call should be auto-approved without prompting.
// Returns true if SafetyMode is Allow, or if the command/path matches AlwaysAllowPatterns.
func (c *Config) ShouldAutoApprove(toolName string, args map[string]interface{}) bool {
	if c.SafetyMode == SafetyModeAllow {
		return true
	}
	if c.SafetyMode == SafetyModeReject {
		return false
	}
	// SafetyModeAsk: check AlwaysAllowPatterns
	for _, pattern := range c.AlwaysAllowPatterns {
		// Pattern format: "tool:value" e.g. "bash:ls", "write:/tmp/test"
		parts := strings.SplitN(pattern, ":", 2)
		if len(parts) < 2 {
			continue
		}
		if parts[0] != toolName {
			continue
		}
		// Check if the command/path starts with the allowed pattern
		switch toolName {
		case "bash":
			if cmd, ok := args["command"].(string); ok && strings.HasPrefix(cmd, parts[1]) {
				return true
			}
		case "write_file", "read_file", "delete_file":
			if path, ok := args["path"].(string); ok && strings.HasPrefix(path, parts[1]) {
				return true
			}
		}
	}
	return false
}
