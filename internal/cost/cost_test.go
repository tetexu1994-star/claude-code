package cost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetRate(t *testing.T) {
	t.Parallel()

	t.Run("known rate", func(t *testing.T) {
		rate := GetRate("anthropic", "claude-sonnet-4-20250514")
		if rate.Input != 3.00 {
			t.Errorf("expected Input 3.00, got %f", rate.Input)
		}
		if rate.Output != 15.00 {
			t.Errorf("expected Output 15.00, got %f", rate.Output)
		}
	})

	t.Run("openai gpt-4o", func(t *testing.T) {
		rate := GetRate("openai", "gpt-4o")
		if rate.Input != 2.50 {
			t.Errorf("expected Input 2.50, got %f", rate.Input)
		}
	})

	t.Run("deepseek chat", func(t *testing.T) {
		rate := GetRate("deepseek", "deepseek-chat")
		if rate.Input != 0.27 {
			t.Errorf("expected Input 0.27, got %f", rate.Input)
		}
	})

	t.Run("unknown provider returns default", func(t *testing.T) {
		rate := GetRate("unknown", "unknown-model")
		if rate.Input != 1.00 {
			t.Errorf("expected default Input 1.00, got %f", rate.Input)
		}
		if rate.Output != 3.00 {
			t.Errorf("expected default Output 3.00, got %f", rate.Output)
		}
	})
}

func TestEstimateCost(t *testing.T) {
	t.Parallel()

	t.Run("anthropic sonnet", func(t *testing.T) {
		cost := EstimateCost("anthropic", "claude-sonnet-4-20250514", 1000000, 1000000)
		expected := 3.00 + 15.00 // 1M input * $3/M + 1M output * $15/M
		if cost != expected {
			t.Errorf("expected %f, got %f", expected, cost)
		}
	})

	t.Run("zero tokens", func(t *testing.T) {
		cost := EstimateCost("openai", "gpt-4o", 0, 0)
		if cost != 0.0 {
			t.Errorf("expected 0.0, got %f", cost)
		}
	})

	t.Run("unknown provider uses defaults", func(t *testing.T) {
		cost := EstimateCost("unknown", "unknown", 1000000, 1000000)
		expected := 1.00 + 3.00
		if cost != expected {
			t.Errorf("expected %f, got %f", expected, cost)
		}
	})
}

func TestClassifyPrompt(t *testing.T) {
	t.Parallel()

	t.Run("simple", func(t *testing.T) {
		if ClassifyPrompt(100) != ComplexitySimple {
			t.Error("expected Simple for 100 tokens")
		}
		if ClassifyPrompt(499) != ComplexitySimple {
			t.Error("expected Simple for 499 tokens")
		}
	})

	t.Run("normal", func(t *testing.T) {
		if ClassifyPrompt(500) != ComplexityNormal {
			t.Error("expected Normal for 500 tokens")
		}
		if ClassifyPrompt(2999) != ComplexityNormal {
			t.Error("expected Normal for 2999 tokens")
		}
	})

	t.Run("complex", func(t *testing.T) {
		if ClassifyPrompt(3000) != ComplexityComplex {
			t.Error("expected Complex for 3000 tokens")
		}
	})
}

func TestClassifyPromptText(t *testing.T) {
	t.Parallel()

	t.Run("simple commands", func(t *testing.T) {
		for _, cmd := range []string{"ls -la", "cat file.txt", "echo hello", "pwd", "cd /tmp", "which go", "whoami"} {
			c := ClassifyPromptText(cmd)
			if c != ComplexitySimple {
				t.Errorf("command %q: expected Simple, got %v", cmd, c)
			}
		}
	})

	t.Run("complex keywords", func(t *testing.T) {
		longText := strings.Repeat("explain how this works in detail please give me a very long explanation ", 20)
		c := ClassifyPromptText(longText)
		if c != ComplexityComplex {
			t.Errorf("expected Complex, got %v", c)
		}
	})

	t.Run("complex with analyze", func(t *testing.T) {
		longText := strings.Repeat("analyze this code and refactor it to be better, review the design and architecture ", 10)
		c := ClassifyPromptText(longText)
		if c != ComplexityComplex {
			t.Errorf("expected Complex, got %v", c)
		}
	})

	t.Run("short explain not complex", func(t *testing.T) {
		c := ClassifyPromptText("explain hello")
		if c == ComplexityComplex {
			t.Error("short 'explain' should not be complex")
		}
	})

	t.Run("normal text", func(t *testing.T) {
		c := ClassifyPromptText("What is the capital of France?")
		if c != ComplexitySimple {
			t.Errorf("expected Simple for short question, got %v", c)
		}
	})
}

func TestRouterSelect(t *testing.T) {
	t.Parallel()

	t.Run("default complexity returns fixed route", func(t *testing.T) {
		r := NewRouter("anthropic", "claude-sonnet-4-20250514")
		result := r.Select(ComplexityNormal, nil)
		if result.Provider != "anthropic" {
			t.Errorf("expected 'anthropic', got %q", result.Provider)
		}
		if result.Model != "claude-sonnet-4-20250514" {
			t.Errorf("expected 'claude-sonnet-4-20250514', got %q", result.Model)
		}
		if result.Reason != "fixed (user preference)" {
			t.Errorf("expected fixed reason, got %q", result.Reason)
		}
	})

	t.Run("cheapest with available providers", func(t *testing.T) {
		r := NewRouter("openai", "gpt-4o")
		result := r.Select(ComplexitySimple, []string{"openai", "deepseek"})
		if result.Provider == "" {
			t.Error("expected non-empty provider")
		}
		if !strings.Contains(result.Reason, "cheapest") {
			t.Errorf("expected 'cheapest' reason, got %q", result.Reason)
		}
	})

	t.Run("strongest with available providers", func(t *testing.T) {
		r := NewRouter("openai", "gpt-4o")
		result := r.Select(ComplexityComplex, []string{"anthropic", "openai"})
		if result.Provider == "" {
			t.Error("expected non-empty provider")
		}
		if !strings.Contains(result.Reason, "strongest") {
			t.Errorf("expected 'strongest' reason, got %q", result.Reason)
		}
	})

	t.Run("strongest prefers anthropic sonnet over gpt-4o", func(t *testing.T) {
		r := NewRouter("openai", "gpt-4o")
		result := r.Select(ComplexityComplex, []string{"anthropic", "openai"})
		if result.Provider != "anthropic" {
			t.Errorf("expected anthropic (score 10 > gpt-4o score 9), got %q", result.Provider)
		}
	})

	t.Run("empty available falls back to default", func(t *testing.T) {
		r := NewRouter("anthropic", "claude-sonnet-4-20250514")
		result := r.Select(ComplexitySimple, nil)
		if result.Provider != "anthropic" {
			t.Errorf("expected fallback to 'anthropic', got %q", result.Provider)
		}
	})
}

func TestTrackerRecordAndStats(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tracker.Record("openai", "gpt-4o", 1000, 500)
	tracker.Record("openai", "gpt-4o", 2000, 300)

	stats := tracker.GetStats("openai", "gpt-4o")
	if stats.TotalCalls != 2 {
		t.Errorf("expected 2 calls, got %d", stats.TotalCalls)
	}
	if stats.TotalInputTokens != 3000 {
		t.Errorf("expected 3000 input tokens, got %d", stats.TotalInputTokens)
	}
	if stats.TotalOutputTokens != 800 {
		t.Errorf("expected 800 output tokens, got %d", stats.TotalOutputTokens)
	}
}

func TestTrackerTotalCost(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tracker.Record("openai", "gpt-4o", 1000000, 500000)
	total := tracker.TotalCost()
	if total <= 0 {
		t.Error("expected positive total cost")
	}

	// Cost should be ~ (1M/1M * $2.50) + (0.5M/1M * $10.00) = $2.50 + $5.00 = $7.50
	expected := EstimateCost("openai", "gpt-4o", 1000000, 500000)
	if total != expected {
		t.Errorf("expected total %f, got %f", expected, total)
	}
}

func TestTrackerEmptyStats(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats := tracker.GetStats("nonexistent", "nonexistent")
	if stats.TotalCalls != 0 {
		t.Errorf("expected 0 calls, got %d", stats.TotalCalls)
	}
	if stats.TotalCostUSD != 0.0 {
		t.Errorf("expected 0.0 cost, got %f", stats.TotalCostUSD)
	}

	allStats := tracker.GetAllStats()
	if len(allStats) != 0 {
		t.Errorf("expected empty stats, got %d entries", len(allStats))
	}

	if tracker.TotalCost() != 0.0 {
		t.Errorf("expected 0.0 total cost, got %f", tracker.TotalCost())
	}
}

func TestTrackerSaveLoad(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tracker.Record("deepseek", "deepseek-chat", 5000, 2000)
	tracker.Record("deepseek", "deepseek-chat", 3000, 1000)

	if err := tracker.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Create a new tracker from the same dir and verify data was loaded.
	tracker2, err := NewTracker(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats := tracker2.GetStats("deepseek", "deepseek-chat")
	if stats.TotalCalls != 2 {
		t.Errorf("after load: expected 2 calls, got %d", stats.TotalCalls)
	}
	if stats.TotalInputTokens != 8000 {
		t.Errorf("after load: expected 8000 input tokens, got %d", stats.TotalInputTokens)
	}
}

func TestTrackerNewNonExistentDir(t *testing.T) {
	base := t.TempDir()
	dataDir := filepath.Join(base, "new", "cost", "data")
	tracker, err := NewTracker(dataDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracker == nil {
		t.Fatal("expected tracker, got nil")
	}

	// Verify dir was created
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("expected data dir to be created")
	}
}

func TestRouterNewRouter(t *testing.T) {
	t.Parallel()

	r := NewRouter("openai", "gpt-4o-mini")
	result := r.Select(ComplexityNormal, nil)
	if result.Provider != "openai" {
		t.Errorf("expected 'openai', got %q", result.Provider)
	}
	if result.Model != "gpt-4o-mini" {
		t.Errorf("expected 'gpt-4o-mini', got %q", result.Model)
	}
}

func TestRateStruct(t *testing.T) {
	t.Parallel()

	r := Rate{Input: 1.50, Output: 5.00}
	if r.Input != 1.50 {
		t.Error("expected Input 1.50")
	}
	if r.Output != 5.00 {
		t.Error("expected Output 5.00")
	}
}

func TestUsageRecordStruct(t *testing.T) {
	t.Parallel()

	r := UsageRecord{
		Provider:     "openai",
		Model:        "gpt-4o",
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      0.0075,
	}

	if r.Provider != "openai" {
		t.Error("expected Provider 'openai'")
	}
	if r.InputTokens != 1000 {
		t.Error("expected InputTokens 1000")
	}
	if r.CostUSD != 0.0075 {
		t.Error("expected CostUSD 0.0075")
	}
}

func TestProviderStatsStruct(t *testing.T) {
	t.Parallel()

	s := ProviderStats{
		TotalInputTokens:  10000,
		TotalOutputTokens: 5000,
		TotalCostUSD:      0.05,
		TotalCalls:        5,
	}

	if s.TotalInputTokens != 10000 {
		t.Error("expected TotalInputTokens 10000")
	}
	if s.TotalCalls != 5 {
		t.Error("expected TotalCalls 5")
	}
}

func TestTrackedMultipleProviders(t *testing.T) {
	dir := t.TempDir()
	tracker, err := NewTracker(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tracker.Record("openai", "gpt-4o", 1000, 500)
	tracker.Record("anthropic", "claude-sonnet-4-20250514", 2000, 1000)

	allStats := tracker.GetAllStats()
	if len(allStats) != 2 {
		t.Errorf("expected 2 provider entries, got %d", len(allStats))
	}

	total := tracker.TotalCost()
	if total <= 0 {
		t.Error("expected positive total cost")
	}
}

func TestGetRateMultiple(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider, model string
		expectedInput   float64
		expectedOutput  float64
	}{
		{"deepseek", "deepseek-chat", 0.27, 1.10},
		{"deepseek", "deepseek-reasoner", 0.55, 2.19},
		{"anthropic", "claude-haiku-3-5", 0.80, 4.00},
		{"openai", "gpt-4o-mini", 0.15, 0.60},
		{"siliconflow", "deepseek-chat", 0.27, 1.10},
		{"tongyi", "qwen-max", 1.60, 6.40},
		{"tongyi", "qwen-plus", 0.80, 2.00},
		{"zhipu", "glm-4-plus", 1.00, 1.00},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.model, func(t *testing.T) {
			rate := GetRate(tt.provider, tt.model)
			if rate.Input != tt.expectedInput {
				t.Errorf("Input: expected %f, got %f", tt.expectedInput, rate.Input)
			}
			if rate.Output != tt.expectedOutput {
				t.Errorf("Output: expected %f, got %f", tt.expectedOutput, rate.Output)
			}
		})
	}
}

func TestComplexityConstants(t *testing.T) {
	t.Parallel()

	if ComplexitySimple != 0 {
		t.Errorf("expected 0, got %d", ComplexitySimple)
	}
	if ComplexityNormal != 1 {
		t.Errorf("expected 1, got %d", ComplexityNormal)
	}
	if ComplexityComplex != 2 {
		t.Errorf("expected 2, got %d", ComplexityComplex)
	}
}
