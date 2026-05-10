package moa

import (
	"strings"
	"testing"
	"time"

	"github.com/tetexu/tlaude-code/internal/llm"
)

func TestMajorityVote(t *testing.T) {
	t.Parallel()

	t.Run("clear majority", func(t *testing.T) {
		results := []ParallelResult{
			{ProviderName: "A", Content: "yes"},
			{ProviderName: "B", Content: "yes"},
			{ProviderName: "C", Content: "no"},
		}
		content, winner := majorityVote(results)
		if content != "yes" {
			t.Errorf("expected 'yes', got %q", content)
		}
		if winner != "A" {
			t.Errorf("expected 'A', got %q", winner)
		}
	})

	t.Run("all unique", func(t *testing.T) {
		results := []ParallelResult{
			{ProviderName: "A", Content: "a"},
			{ProviderName: "B", Content: "b"},
			{ProviderName: "C", Content: "c"},
		}
		content, winner := majorityVote(results)
		// Map iteration is non-deterministic; any content is valid.
		if content != "a" && content != "b" && content != "c" {
			t.Errorf("expected one of a/b/c, got %q", content)
		}
		if winner == "" {
			t.Error("expected non-empty winner")
		}
	})

	t.Run("empty", func(t *testing.T) {
		content, winner := majorityVote(nil)
		if content != "" {
			t.Errorf("expected empty, got %q", content)
		}
		if winner != "" {
			t.Errorf("expected empty, got %q", winner)
		}
	})

	t.Run("trim whitespace", func(t *testing.T) {
		results := []ParallelResult{
			{ProviderName: "A", Content: "  hello  "},
			{ProviderName: "B", Content: "hello"},
		}
		content, _ := majorityVote(results)
		if content != "hello" {
			t.Errorf("expected trimmed 'hello', got %q", content)
		}
	})
}

func TestTruncateContent(t *testing.T) {
	t.Parallel()

	t.Run("short string", func(t *testing.T) {
		result := truncateContent("hello", 80)
		if result != "hello" {
			t.Errorf("expected 'hello', got %q", result)
		}
	})

	t.Run("long string", func(t *testing.T) {
		long := strings.Repeat("a", 100)
		result := truncateContent(long, 80)
		if len(result) > 83 { // 80 chars + "..." = 83
			t.Errorf("result too long: %d chars", len(result))
		}
		if !strings.HasSuffix(result, "...") {
			t.Error("expected to end with '...'")
		}
	})

	t.Run("first line boundary", func(t *testing.T) {
		result := truncateContent("short line\nmore content here that is very long", 80)
		if !strings.HasSuffix(result, "...") {
			t.Error("expected to end with '...'")
		}
		if strings.Contains(result, "\n") {
			t.Error("expected no newline in truncated result")
		}
	})

	t.Run("whitespace trimmed", func(t *testing.T) {
		result := truncateContent("  padded  ", 80)
		if result != "padded" {
			t.Errorf("expected 'padded', got %q", result)
		}
	})
}

func TestBuildSynthesisPrompt(t *testing.T) {
	t.Parallel()

	results := []ParallelResult{
		{ProviderName: "deepseek", Content: "Answer A", Latency: 100 * time.Millisecond},
		{ProviderName: "anthropic", Content: "Answer B", Latency: 200 * time.Millisecond},
	}

	t.Run("with user prompt", func(t *testing.T) {
		prompt := BuildSynthesisPrompt(results, "What is Go?")
		if !strings.Contains(prompt, "The user asked: What is Go?") {
			t.Error("expected user prompt prefix")
		}
		if !strings.Contains(prompt, "[Provider: deepseek") {
			t.Error("expected deepseek provider")
		}
		if !strings.Contains(prompt, "[Provider: anthropic") {
			t.Error("expected anthropic provider")
		}
		if !strings.Contains(prompt, "Answer A") {
			t.Error("expected Answer A")
		}
		if !strings.Contains(prompt, "Answer B") {
			t.Error("expected Answer B")
		}
		if !strings.Contains(prompt, "Synthesized response:") {
			t.Error("expected synthesis trailer")
		}
	})

	t.Run("with empty user prompt", func(t *testing.T) {
		prompt := BuildSynthesisPrompt(results, "")
		if strings.Contains(prompt, "The user asked:") {
			t.Error("should not contain user prompt prefix when empty")
		}
		if !strings.Contains(prompt, "Synthesized response:") {
			t.Error("expected synthesis trailer")
		}
	})

	t.Run("empty results", func(t *testing.T) {
		prompt := BuildSynthesisPrompt(nil, "test")
		if !strings.Contains(prompt, "Multiple AI responses were generated") {
			t.Error("expected the synthesis header")
		}
		if !strings.Contains(prompt, "Synthesized response:") {
			t.Error("expected synthesis trailer")
		}
	})
}

func TestBuildConsensusReport(t *testing.T) {
	t.Parallel()

	t.Run("multiple results", func(t *testing.T) {
		results := []ParallelResult{
			{ProviderName: "A", Content: "answer A", Latency: 100 * time.Millisecond},
			{ProviderName: "B", Content: "answer B", Latency: 200 * time.Millisecond},
		}
		report := BuildConsensusReport(results)
		if !strings.Contains(report, "No consensus") {
			t.Error("expected 'no consensus' header")
		}
		if !strings.Contains(report, "### A") {
			t.Error("expected provider A section")
		}
		if !strings.Contains(report, "### B") {
			t.Error("expected provider B section")
		}
	})

	t.Run("single result", func(t *testing.T) {
		results := []ParallelResult{
			{ProviderName: "A", Content: "only answer", Latency: 50 * time.Millisecond},
		}
		report := BuildConsensusReport(results)
		if !strings.Contains(report, "only answer") {
			t.Error("expected the content")
		}
		if strings.Contains(report, "---") {
			t.Error("should not have separator for single result")
		}
	})
}

func TestBuildMoASummary(t *testing.T) {
	t.Parallel()

	t.Run("fastest mode", func(t *testing.T) {
		result := &MoAResult{
			Mode:        ModeFastest,
			WinningName: "deepseek",
			Duration:    150 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "deepseek", Content: "ok"},
				{ProviderName: "anthropic", Content: "ok"},
			},
		}
		summary := BuildMoASummary(result)
		if !strings.Contains(summary, "MoA:") {
			t.Error("expected MoA: prefix")
		}
		if !strings.Contains(summary, "fastest=deepseek") {
			t.Errorf("expected fastest=deepseek, got %q", summary)
		}
		if !strings.Contains(summary, "2/2 ok") {
			t.Errorf("expected 2/2 ok, got %q", summary)
		}
	})

	t.Run("consensus mode", func(t *testing.T) {
		result := &MoAResult{
			Mode:        ModeConsensus,
			WinningName: "openai",
			Duration:    200 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "openai", Content: "ok"},
				{ProviderName: "anthropic", Content: "ok"},
			},
		}
		summary := BuildMoASummary(result)
		if !strings.Contains(summary, "consensus=openai") {
			t.Errorf("expected consensus=openai, got %q", summary)
		}
	})

	t.Run("consensus no winner", func(t *testing.T) {
		result := &MoAResult{
			Mode:        ModeConsensus,
			WinningName: "",
			Duration:    100 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "A", Content: "ok"},
			},
		}
		summary := BuildMoASummary(result)
		if !strings.Contains(summary, "no consensus") {
			t.Errorf("expected 'no consensus', got %q", summary)
		}
	})

	t.Run("majority mode", func(t *testing.T) {
		result := &MoAResult{
			Mode:        ModeMajority,
			WinningName: "deepseek",
			Duration:    100 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "deepseek", Content: "ok"},
			},
		}
		summary := BuildMoASummary(result)
		if !strings.Contains(summary, "majority=deepseek") {
			t.Errorf("expected majority=deepseek, got %q", summary)
		}
	})

	t.Run("synthesize mode", func(t *testing.T) {
		result := &MoAResult{
			Mode:    ModeSynthesize,
			Duration: 300 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "A", Content: "ok"},
				{ProviderName: "B", Content: "ok"},
			},
		}
		summary := BuildMoASummary(result)
		if !strings.Contains(summary, "synthesized") {
			t.Errorf("expected 'synthesized', got %q", summary)
		}
	})

	t.Run("with errors", func(t *testing.T) {
		result := &MoAResult{
			Mode:        ModeFastest,
			WinningName: "deepseek",
			Duration:    100 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "deepseek", Content: "ok"},
				{ProviderName: "anthropic", Error: errTimeout("timeout")},
			},
		}
		summary := BuildMoASummary(result)
		if !strings.Contains(summary, "1/2 ok") {
			t.Errorf("expected 1/2 ok, got %q", summary)
		}
	})
}

func TestBuildMoADetail(t *testing.T) {
	t.Parallel()

	t.Run("fastest detail", func(t *testing.T) {
		result := &MoAResult{
			Mode:    ModeFastest,
			Duration: 150 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "deepseek", Content: "answer", Latency: 50 * time.Millisecond},
				{ProviderName: "anthropic", Content: "different answer", Latency: 100 * time.Millisecond},
			},
		}
		detail := BuildMoADetail(result)
		if !strings.Contains(detail, "fastest mode") {
			t.Error("expected 'fastest mode'")
		}
		if !strings.Contains(detail, "deepseek") {
			t.Error("expected deepseek")
		}
		if !strings.Contains(detail, "✓") {
			t.Error("expected checkmark for success")
		}
	})

	t.Run("error detail", func(t *testing.T) {
		result := &MoAResult{
			Mode:    ModeSynthesize,
			Duration: 100 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "bad", Error: errTimeout("timeout"), Latency: 100 * time.Millisecond},
			},
		}
		detail := BuildMoADetail(result)
		if !strings.Contains(detail, "✗") {
			t.Error("expected x mark for error")
		}
		if !strings.Contains(detail, "timeout") {
			t.Error("expected error message")
		}
	})

	t.Run("consensus detail", func(t *testing.T) {
		result := &MoAResult{
			Mode:    ModeConsensus,
			Duration: 100 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "A", Content: "ok", Latency: 10 * time.Millisecond},
			},
		}
		detail := BuildMoADetail(result)
		if !strings.Contains(detail, "consensus mode") {
			t.Error("expected 'consensus mode'")
		}
	})

	t.Run("majority detail", func(t *testing.T) {
		result := &MoAResult{
			Mode:    ModeMajority,
			Duration: 100 * time.Millisecond,
			Responses: []ParallelResult{
				{ProviderName: "A", Content: "ok", Latency: 10 * time.Millisecond},
			},
		}
		detail := BuildMoADetail(result)
		if !strings.Contains(detail, "majority mode") {
			t.Error("expected 'majority mode'")
		}
	})
}

func TestNewOrchestrator(t *testing.T) {
	t.Run("default timeout and max parallel", func(t *testing.T) {
		cfg := MoAConfig{
			Enabled:     true,
			Mode:        ModeSynthesize,
			TimeoutSec:  0,
			MaxParallel: 0,
		}
		registry := &llm.Registry{}
		o := NewOrchestrator(registry, cfg)
		if o == nil {
			t.Fatal("expected orchestrator, got nil")
		}
		if o.timeout != 30*time.Second {
			t.Errorf("expected default 30s timeout, got %v", o.timeout)
		}
		if o.maxParallel != 3 {
			t.Errorf("expected default maxParallel=3, got %d", o.maxParallel)
		}
	})

	t.Run("custom timeout and max parallel", func(t *testing.T) {
		cfg := MoAConfig{
			Enabled:     true,
			Mode:        ModeFastest,
			TimeoutSec:  10,
			MaxParallel: 5,
		}
		registry := &llm.Registry{}
		o := NewOrchestrator(registry, cfg)
		if o.timeout != 10*time.Second {
			t.Errorf("expected 10s timeout, got %v", o.timeout)
		}
		if o.maxParallel != 5 {
			t.Errorf("expected maxParallel=5, got %d", o.maxParallel)
		}
	})
}

func TestParallelResult(t *testing.T) {
	t.Parallel()

	r := ParallelResult{
		ProviderName: "test",
		Content:      "hello",
		Latency:      50 * time.Millisecond,
		Error:        nil,
	}

	if r.ProviderName != "test" {
		t.Error("expected ProviderName 'test'")
	}
	if r.Content != "hello" {
		t.Error("expected Content 'hello'")
	}
	if r.Latency != 50*time.Millisecond {
		t.Error("expected Latency 50ms")
	}
}

func TestMoAResult(t *testing.T) {
	t.Parallel()

	r := MoAResult{
		FinalContent: "final",
		Mode:         ModeFastest,
		WinningName:  "winner",
		Duration:     100 * time.Millisecond,
		Responses: []ParallelResult{
			{ProviderName: "winner", Content: "final"},
		},
	}

	if r.FinalContent != "final" {
		t.Error("expected FinalContent 'final'")
	}
	if r.Mode != ModeFastest {
		t.Error("expected Mode fastest")
	}
}

func TestModeConstants(t *testing.T) {
	t.Parallel()

	if ModeFastest != "fastest" {
		t.Errorf("expected 'fastest', got %q", ModeFastest)
	}
	if ModeConsensus != "consensus" {
		t.Errorf("expected 'consensus', got %q", ModeConsensus)
	}
	if ModeMajority != "majority" {
		t.Errorf("expected 'majority', got %q", ModeMajority)
	}
	if ModeSynthesize != "synthesize" {
		t.Errorf("expected 'synthesize', got %q", ModeSynthesize)
	}
}

type errTimeout string

func (e errTimeout) Error() string { return string(e) }
