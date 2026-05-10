package moa

import (
	"fmt"
	"strings"
	"time"
)

// BuildSynthesisPrompt constructs a prompt that asks a synthesizer model to merge
// multiple provider responses into a single coherent answer.
func BuildSynthesisPrompt(results []ParallelResult, userPrompt string) string {
	var sb strings.Builder

	if userPrompt != "" {
		sb.WriteString("The user asked: ")
		sb.WriteString(userPrompt)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Multiple AI responses were generated. Synthesize them into the best possible single response, combining strengths and resolving conflicts:\n\n")

	for _, r := range results {
		sb.WriteString(fmt.Sprintf("[Provider: %s - %v]:\n", r.ProviderName, r.Latency.Round(time.Millisecond)))
		sb.WriteString(r.Content)
		sb.WriteString("\n---\n\n")
	}

	sb.WriteString("Synthesized response:")
	return sb.String()
}

// BuildConsensusReport generates a report showing the disagreement between
// provider responses when consensus mode fails to find agreement.
func BuildConsensusReport(results []ParallelResult) string {
	var sb strings.Builder
	sb.WriteString("⚠️ No consensus among providers. Here are the differing responses:\n\n")

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("### %s (%v)\n", r.ProviderName, r.Latency.Round(time.Millisecond)))
		sb.WriteString(strings.TrimSpace(r.Content))
		if i < len(results)-1 {
			sb.WriteString("\n\n---\n\n")
		}
	}

	return sb.String()
}

// BuildMoASummary creates a one-line summary of MoA results for the status bar.
func BuildMoASummary(result *MoAResult) string {
	var sb strings.Builder
	sb.WriteString("MoA: ")

	switch result.Mode {
	case ModeFastest:
		sb.WriteString(fmt.Sprintf("fastest=%s", result.WinningName))
	case ModeMajority:
		sb.WriteString(fmt.Sprintf("majority=%s", result.WinningName))
	case ModeConsensus:
		if result.WinningName != "" {
			sb.WriteString(fmt.Sprintf("consensus=%s", result.WinningName))
		} else {
			sb.WriteString("no consensus")
		}
	default:
		sb.WriteString("synthesized")
	}

	sb.WriteString(fmt.Sprintf(" | %v", result.Duration.Round(time.Millisecond)))

	successCount := 0
	for _, r := range result.Responses {
		if r.Error == nil && r.Content != "" {
			successCount++
		}
	}
	sb.WriteString(fmt.Sprintf(" | %d/%d ok", successCount, len(result.Responses)))

	return sb.String()
}

// BuildMoADetail formats all provider responses and latencies for display in chat.
func BuildMoADetail(result *MoAResult) string {
	var sb strings.Builder
	sb.WriteString("─── MoA Details (")

	switch result.Mode {
	case ModeFastest:
		sb.WriteString("fastest mode")
	case ModeConsensus:
		sb.WriteString("consensus mode")
	case ModeMajority:
		sb.WriteString("majority mode")
	default:
		sb.WriteString("synthesize mode")
	}
	sb.WriteString(fmt.Sprintf(", %v total", result.Duration.Round(time.Millisecond)))
	sb.WriteString(") ───\n")

	for _, r := range result.Responses {
		status := "✓"
		if r.Error != nil {
			status = fmt.Sprintf("✗ %v", r.Error)
		}
		sb.WriteString(fmt.Sprintf("  %s %-14s %8v  %s\n",
			status, r.ProviderName, r.Latency.Round(time.Millisecond),
			truncateContent(r.Content, 80)))
	}

	return sb.String()
}

func truncateContent(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	// Take the first line or first maxLen characters.
	if idx := strings.IndexByte(s, '\n'); idx >= 0 && idx < maxLen {
		return s[:idx] + "..."
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
