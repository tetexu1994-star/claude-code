package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Provider != "anthropic" {
		t.Errorf("Provider = %s, want anthropic", cfg.Provider)
	}
	if cfg.Model != "claude-sonnet-4-20250514" {
		t.Errorf("Model = %s, want claude-sonnet-4-20250514", cfg.Model)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("Temperature = %f, want 0.7", cfg.Temperature)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", cfg.MaxTokens)
	}
	if cfg.APIKeys == nil {
		t.Error("APIKeys map should be initialized")
	}
	if cfg.SessionDir == "" {
		t.Error("SessionDir should not be empty")
	}
}

func TestGetAPIKey(t *testing.T) {
	cfg := DefaultConfig()
	if key := cfg.GetAPIKey("anthropic"); key != "" {
		t.Errorf("GetAPIKey() = %s, want empty", key)
	}

	cfg.SetAPIKey("anthropic", "sk-test-key")
	if key := cfg.GetAPIKey("anthropic"); key != "sk-test-key" {
		t.Errorf("GetAPIKey() = %s, want sk-test-key", key)
	}
}

func TestSetAPIKey_NilMap(t *testing.T) {
	cfg := &Config{}
	cfg.SetAPIKey("anthropic", "sk-test")
	if cfg.APIKeys == nil {
		t.Fatal("SetAPIKey should initialize nil APIKeys map")
	}
	if cfg.APIKeys["anthropic"] != "sk-test" {
		t.Error("SetAPIKey did not set the key correctly")
	}
}

func TestGetAPIKey_NilMap(t *testing.T) {
	cfg := &Config{}
	if key := cfg.GetAPIKey("anthropic"); key != "" {
		t.Errorf("GetAPIKey on nil map = %s, want empty", key)
	}
}

func TestDetectEnvAPIKeys(t *testing.T) {
	// Save and clear any pre-existing env vars to isolate the test
	saved := make(map[string]string)
	for _, envVar := range providerEnvVars {
		saved[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}
	defer func() {
		for envVar, val := range saved {
			if val != "" {
				os.Setenv(envVar, val)
			} else {
				os.Unsetenv(envVar)
			}
		}
	}()

	// Set up test env vars
	os.Setenv("ANTHROPIC_API_KEY", "env-anthropic-key")
	os.Setenv("OPENAI_API_KEY", "env-openai-key")

	cfg := DefaultConfig()
	cfg.DetectEnvAPIKeys()

	if key := cfg.GetAPIKey("anthropic"); key != "env-anthropic-key" {
		t.Errorf("anthropic key = %s, want env-anthropic-key", key)
	}
	if key := cfg.GetAPIKey("openai"); key != "env-openai-key" {
		t.Errorf("openai key = %s, want env-openai-key", key)
	}
	// Providers without env var set should remain empty
	if key := cfg.GetAPIKey("deepseek"); key != "" {
		t.Errorf("deepseek key = %s, want empty", key)
	}
}

func TestDetectEnvAPIKeys_DoesNotOverride(t *testing.T) {
	// Save and clear any pre-existing env vars to isolate the test
	saved := make(map[string]string)
	for _, envVar := range providerEnvVars {
		saved[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}
	defer func() {
		for envVar, val := range saved {
			if val != "" {
				os.Setenv(envVar, val)
			} else {
				os.Unsetenv(envVar)
			}
		}
	}()

	os.Setenv("ANTHROPIC_API_KEY", "env-anthropic-key")

	cfg := DefaultConfig()
	cfg.SetAPIKey("anthropic", "existing-key")
	cfg.DetectEnvAPIKeys()

	if key := cfg.GetAPIKey("anthropic"); key != "existing-key" {
		t.Errorf("anthropic key = %s, want existing-key (should not be overridden by env)", key)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Use a temp home to isolate from real config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := DefaultConfig()
	cfg.Provider = "openai"
	cfg.Model = "gpt-4"
	cfg.Temperature = 0.5
	cfg.MaxTokens = 2048
	cfg.SetAPIKey("openai", "sk-test")
	cfg.SetAPIKey("anthropic", "sk-ant-test")

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tmpDir, ".tlaude-code", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config.yaml was not created")
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.Provider != "openai" {
		t.Errorf("loaded Provider = %s, want openai", loaded.Provider)
	}
	if loaded.Model != "gpt-4" {
		t.Errorf("loaded Model = %s, want gpt-4", loaded.Model)
	}
	if loaded.Temperature != 0.5 {
		t.Errorf("loaded Temperature = %f, want 0.5", loaded.Temperature)
	}
	if loaded.MaxTokens != 2048 {
		t.Errorf("loaded MaxTokens = %d, want 2048", loaded.MaxTokens)
	}
	if key := loaded.GetAPIKey("openai"); key != "sk-test" {
		t.Errorf("loaded openai key = %s, want sk-test", key)
	}
	if key := loaded.GetAPIKey("anthropic"); key != "sk-ant-test" {
		t.Errorf("loaded anthropic key = %s, want sk-ant-test", key)
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should not error when config file does not exist: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load() should return default config when file does not exist")
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("default Provider = %s, want anthropic", cfg.Provider)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	// Write invalid YAML
	configDir := filepath.Join(tmpDir, ".tlaude-code")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("{{{invalid: yaml: ["), 0644)

	cfg, err := Load()
	if err == nil {
		t.Error("Load() should return error for invalid YAML")
	}
	if cfg == nil {
		t.Error("Load() should still return a config even on parse error")
	}
}

func TestProbeConnectivity(t *testing.T) {
	cfg := DefaultConfig()
	// No keys configured, should return empty results
	results := cfg.ProbeConnectivity()
	if len(results) != 0 {
		t.Errorf("ProbeConnectivity() with no keys = %d results, want 0", len(results))
	}

	// With a key set, the function should attempt a probe and return a map
	// that at minimum contains the provider name as a key (result can be nil
	// if the host is reachable, or an error if it isn't).
	cfg.SetAPIKey("deepseek", "sk-fake")
	results = cfg.ProbeConnectivity()
	if _, ok := results["deepseek"]; !ok {
		t.Error("ProbeConnectivity() should include probed provider in results map")
	}
}

func TestDefaultMemoryConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.Memory.Enabled {
		t.Error("Memory.Enabled should default to true")
	}
	if cfg.Memory.BaseDir != "" {
		t.Errorf("Memory.BaseDir should default to empty, got %q", cfg.Memory.BaseDir)
	}
}

func TestProviderEnvVars(t *testing.T) {
	// Verify all expected providers are mapped
	expected := []string{"anthropic", "openai", "deepseek", "openrouter", "siliconflow", "tongyi", "zhipu"}
	for _, name := range expected {
		if _, ok := providerEnvVars[name]; !ok {
			t.Errorf("providerEnvVars missing entry for %s", name)
		}
		if _, ok := providerBaseURLs[name]; !ok {
			t.Errorf("providerBaseURLs missing entry for %s", name)
		}
	}
}
