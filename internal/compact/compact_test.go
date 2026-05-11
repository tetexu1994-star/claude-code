package compact

import (
	"strings"
	"testing"

	"github.com/tetexu/tlaude-code/internal/llm"
)

func TestFormatCompactSummary(t *testing.T) {
	input := `<analysis>
Some analysis here.
</analysis>

<summary>
1. Primary Request and Intent:
   Test request.

2. Key Technical Concepts:
   - Go
   - Testing
</summary>`

	result := FormatCompactSummary(input)

	if strings.Contains(result, "<analysis>") {
		t.Error("analysis block should be stripped")
	}
	if strings.Contains(result, "</analysis>") {
		t.Error("analysis close tag should be stripped")
	}
	if !strings.Contains(result, "Summary:") {
		t.Error("summary should be prefixed with 'Summary:'")
	}
	if !strings.Contains(result, "Primary Request and Intent") {
		t.Error("result should contain the summary content")
	}
}

func TestGetCompactPrompt(t *testing.T) {
	prompt := GetCompactPrompt("")
	if !strings.Contains(prompt, "CRITICAL: Respond with TEXT ONLY") {
		t.Error("prompt should contain NO_TOOLS_PREAMBLE")
	}
	if !strings.Contains(prompt, "Primary Request and Intent") {
		t.Error("prompt should contain the 9-section template")
	}
	if !strings.Contains(prompt, "Do NOT call any tools") {
		t.Error("prompt should contain NO_TOOLS_TRAILER")
	}
}

func TestGetCompactPromptWithCustomInstructions(t *testing.T) {
	prompt := GetCompactPrompt("Focus on Go code changes.")
	if !strings.Contains(prompt, "Additional Instructions:") {
		t.Error("prompt should contain custom instructions header")
	}
	if !strings.Contains(prompt, "Focus on Go code changes.") {
		t.Error("prompt should contain custom instructions")
	}
}

func TestGetCompactUserSummaryMessage(t *testing.T) {
	summary := "<summary>\nTest content\n</summary>"
	result := GetCompactUserSummaryMessage(summary, false)
	if !strings.Contains(result, "This session is being continued") {
		t.Error("should contain the continuation preamble")
	}
	if strings.Contains(result, "<summary>") {
		t.Error("XML <summary> tags should be stripped")
	}
}

func TestGetCompactUserSummaryMessageSuppressFollowUp(t *testing.T) {
	summary := "<summary>\nTest content\n</summary>"
	result := GetCompactUserSummaryMessage(summary, true)
	if !strings.Contains(result, "Continue the conversation from where it left off") {
		t.Error("should contain the suppress-follow-up instruction")
	}
}

func TestEstimateTokens(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "Hello, this is a test message."},
		{Role: "assistant", Content: "Hi! How can I help you today?"},
	}
	tokens := EstimateTokens(messages)
	if tokens <= 0 {
		t.Error("token estimate should be positive")
	}
}

func TestMicroCompactEmpty(t *testing.T) {
	result := MicroCompact(nil, 3)
	if result != nil {
		t.Error("MicroCompact of nil should return nil")
	}
}

func TestMicroCompactKeepsRecent(t *testing.T) {
	messages := []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "Bash", Result: "old result"},
		}},
		{Role: "tool", Content: "tool result 1", ToolID: "1"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "2", Name: "Bash", Result: "recent result"},
		}},
		{Role: "tool", Content: "tool result 2", ToolID: "2"},
	}
	result := MicroCompact(messages, 1)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	// First Bash result should be cleared.
	if result[0].ToolCalls[0].Result != clearedToolResult {
		t.Errorf("old tool result should be cleared, got: %q", result[0].ToolCalls[0].Result)
	}
	// Recent Bash result should be kept.
	if result[2].ToolCalls[0].Result != "recent result" {
		t.Errorf("recent tool result should be kept, got: %q", result[2].ToolCalls[0].Result)
	}
}

func TestNeedsCompact(t *testing.T) {
	// Create many large messages to exceed threshold.
	var messages []llm.Message
	for i := 0; i < 100; i++ {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: strings.Repeat("this is a long message to consume tokens ", 100),
		})
	}
	// Should trigger for a small model.
	if !NeedsCompact(messages, "claude-haiku-4", 40000) {
		t.Error("NeedsCompact should return true for token-heavy messages with small model")
	}
}

func TestNeedsCompactSmallConversation(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello"},
	}
	// Should NOT trigger for a large model.
	if NeedsCompact(messages, "claude-opus-4", 40000) {
		t.Error("NeedsCompact should not trigger for a tiny conversation")
	}
}

func TestCalculateTokenState(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "Hi"},
	}
	state := CalculateTokenState(messages, "claude-sonnet-4", 20000)
	if state.PercentLeft != 100 {
		t.Logf("PercentLeft = %d (expected 100 for tiny conversation)", state.PercentLeft)
	}
	if state.IsAtBlockingLimit {
		t.Error("tiny conversation should not be at blocking limit")
	}
}

func TestGetAutoCompactThreshold(t *testing.T) {
	threshold := GetAutoCompactThreshold("claude-sonnet-4", 20000)
	if threshold <= 0 {
		t.Error("threshold should be positive")
	}
	// For a 200k model with 20k reserved: effective = 180k, threshold = 180k - 13k = 167k
	if threshold < 100000 {
		t.Errorf("threshold too low for 200k model: %d", threshold)
	}
}

func TestManagerCircuitBreaker(t *testing.T) {
	mgr := NewManager(DefaultConfig())

	// Record 3 failures — circuit breaker should trip.
	tripped := false
	for i := 0; i < MaxConsecutiveAutoCompactFailures; i++ {
		tripped = mgr.RecordAutoFailure()
	}
	if !tripped {
		t.Error("circuit breaker should trip after 3 consecutive failures")
	}

	// AutoState should show 3 failures.
	state := mgr.AutoState()
	if state.ConsecutiveFailures < MaxConsecutiveAutoCompactFailures {
		t.Error("AutoState should reflect failures")
	}

	// Reset and record success.
	mgr.RecordAutoSuccess()
	state2 := mgr.AutoState()
	if state2.ConsecutiveFailures != 0 {
		t.Error("consecutive failures should reset after success")
	}
}

func TestSelectMessagesToKeep(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "old message 1"},
		{Role: "assistant", Content: "old response 1"},
		{Role: "user", Content: "old message 2"},
		{Role: "assistant", Content: "old response 2"},
		{Role: "user", Content: "recent message"},
		{Role: "assistant", Content: "recent response"},
	}
	// With a large budget, all should be kept.
	kept := selectMessagesToKeep(messages, 40000)
	if len(kept) != 6 {
		t.Errorf("with large budget, all 6 messages should be kept, got %d", len(kept))
	}

	// With a tiny budget, at least 4 (2 pairs) should be kept.
	kept2 := selectMessagesToKeep(messages, 10)
	if len(kept2) < 4 {
		t.Errorf("minimum 4 messages should be kept, got %d", len(kept2))
	}
}

func TestStripImagesFromMessages(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "Here is an image: data:image/png;base64,iVBORw0KGgoAAAANSUhEUg"},
		{Role: "assistant", Content: "I see the image"},
	}
	result := stripImagesFromMessages(messages)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if strings.Contains(result[0].Content, "data:image/") {
		t.Error("image data URI should be stripped")
	}
	if !strings.Contains(result[0].Content, "[image]") {
		t.Error("image data URI should be replaced with [image]")
	}
	// Assistant message should be unchanged.
	if result[1].Content != "I see the image" {
		t.Error("assistant message should be unchanged")
	}
}

func TestTruncateHead(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "msg1"},
		{Role: "assistant", Content: "rsp1"},
		{Role: "user", Content: "msg2"},
		{Role: "assistant", Content: "rsp2"},
		{Role: "user", Content: "compact prompt"},
	}
	result := truncateHead(messages)
	if len(result) >= len(messages) {
		t.Error("truncateHead should reduce message count")
	}
	if len(result) < 2 {
		t.Error("truncateHead should keep at least 2 messages")
	}
}
