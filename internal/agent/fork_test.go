package agent

import (
	"strings"
	"testing"

	"github.com/tetexu/tlaude-code/internal/llm"
)

func TestBuildForkedMessages_NoToolUses(t *testing.T) {
	sysPrompt := "You are a helpful assistant."
	parentMessages := []llm.Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	sys, msgs := BuildForkedMessages(sysPrompt, parentMessages, "Research this codebase")
	if sys != sysPrompt {
		t.Errorf("system prompt mismatch: got %q, want %q", sys, sysPrompt)
	}
	if len(msgs) != 3 {
		t.Errorf("message count = %d, want 3 (2 parent + 1 directive)", len(msgs))
	}
	// Last message should contain the boilerplate
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		t.Errorf("last message role = %q, want user", last.Role)
	}
	if !strings.Contains(last.Content, forkBoilerplateTag) {
		t.Error("last message should contain fork boilerplate tag")
	}
	if !strings.Contains(last.Content, "Research this codebase") {
		t.Error("last message should contain directive")
	}
}

func TestBuildForkedMessages_WithToolUses(t *testing.T) {
	sysPrompt := "You are a helpful assistant with tools."
	parentMessages := []llm.Message{
		{Role: "user", Content: "Find all Go files"},
		{
			Role:    "assistant",
			Content: "Let me search for Go files.",
			ToolCalls: []llm.ToolCall{
				{ID: "toolu_001", Name: "Glob", Args: map[string]interface{}{"pattern": "**/*.go"}},
				{ID: "toolu_002", Name: "Grep", Args: map[string]interface{}{"pattern": "func main"}},
			},
		},
	}

	sys, msgs := BuildForkedMessages(sysPrompt, parentMessages, "Review the results")
	if sys != sysPrompt {
		t.Errorf("system prompt mismatch: got %q, want %q", sys, sysPrompt)
	}
	if len(msgs) != 3 {
		t.Errorf("message count = %d, want 3 (2 parent + 1 placeholder+directive)", len(msgs))
	}

	// The last message should contain placeholder results for both tool_use blocks
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		t.Errorf("last message role = %q, want user", last.Role)
	}
	if !strings.Contains(last.Content, forkPlaceholderResult) {
		t.Error("last message should contain placeholder results")
	}
	if !strings.Contains(last.Content, "toolu_001") {
		t.Error("last message should reference toolu_001")
	}
	if !strings.Contains(last.Content, "toolu_002") {
		t.Error("last message should reference toolu_002")
	}
	if !strings.Contains(last.Content, forkBoilerplateTag) {
		t.Error("last message should contain fork boilerplate tag")
	}
}

func TestBuildForkedMessages_PromptCacheIdenticalPrefix(t *testing.T) {
	// Multiple fork children with different directives should produce
	// byte-identical prefixes for maximum cache sharing.
	sysPrompt := "You are a helpful assistant with tools."
	parentMessages := []llm.Message{
		{Role: "user", Content: "Find all Go files"},
		{
			Role:    "assistant",
			Content: "Let me search.",
			ToolCalls: []llm.ToolCall{
				{ID: "toolu_001", Name: "Glob", Args: map[string]interface{}{"pattern": "**/*.go"}},
			},
		},
	}

	sys1, msgs1 := BuildForkedMessages(sysPrompt, parentMessages, "Directive A: check auth")
	sys2, msgs2 := BuildForkedMessages(sysPrompt, parentMessages, "Directive B: check logging")

	if sys1 != sys2 {
		t.Error("system prompts should be identical for cache sharing")
	}
	if len(msgs1) != len(msgs2) {
		t.Error("message counts should be identical")
	}
	// Parent messages (prefix) should be identical
	for i := 0; i < len(parentMessages); i++ {
		if msgs1[i].Content != msgs2[i].Content {
			t.Errorf("parent message %d differs", i)
		}
	}
	// Placeholder results should be identical
	last1 := msgs1[len(msgs1)-1].Content
	last2 := msgs2[len(msgs2)-1].Content

	// The last messages differ only in the directive portion
	if last1 == last2 {
		t.Error("directives should differ")
	}
	// But the placeholder portion should be the same
	placeholderIdx1 := strings.Index(last1, forkBoilerplateTag)
	placeholderIdx2 := strings.Index(last2, forkBoilerplateTag)
	if placeholderIdx1 < 0 || placeholderIdx2 < 0 {
		t.Fatal("boilerplate tag not found")
	}
	prefix1 := last1[:placeholderIdx1]
	prefix2 := last2[:placeholderIdx2]
	if prefix1 != prefix2 {
		t.Error("tool_result placeholder prefixes should be identical (cache sharing)")
	}
}

func TestBuildForkedMessages_EmptyParentMessages(t *testing.T) {
	sysPrompt := "System prompt"
	sys, msgs := BuildForkedMessages(sysPrompt, nil, "Fresh directive")
	if sys != sysPrompt {
		t.Errorf("sys = %q, want %q", sys, sysPrompt)
	}
	if len(msgs) != 1 {
		t.Errorf("message count = %d, want 1", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("role = %q, want user", msgs[0].Role)
	}
}

func TestIsInForkChild_NotFork(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "Hello, how are you?"},
		{Role: "assistant", Content: "I'm doing well, thanks!"},
	}
	if IsInForkChild(messages) {
		t.Error("should NOT detect fork child in normal messages")
	}
}

func TestIsInForkChild_IsFork(t *testing.T) {
	sysPrompt := "System"
	parentMsgs := []llm.Message{
		{Role: "user", Content: "Find files"},
		{
			Role:      "assistant",
			Content:   "Let me search.",
			ToolCalls: []llm.ToolCall{{ID: "toolu_001", Name: "Glob"}},
		},
	}
	_, forkMsgs := BuildForkedMessages(sysPrompt, parentMsgs, "Do research")
	if !IsInForkChild(forkMsgs) {
		t.Error("should detect fork child in forked messages")
	}
}

func TestIsInForkChild_EmptyMessages(t *testing.T) {
	if IsInForkChild(nil) {
		t.Error("nil messages should not be fork child")
	}
	if IsInForkChild([]llm.Message{}) {
		t.Error("empty messages should not be fork child")
	}
}

func TestBuildForkChildMessage_ContainsBoilerplate(t *testing.T) {
	msg := buildForkChildMessage("Do X")
	if !strings.Contains(msg, forkBoilerplateTag) {
		t.Error("should contain boilerplate tag")
	}
	if !strings.Contains(msg, forkDirectivePrefix) {
		t.Error("should contain directive prefix")
	}
	if !strings.Contains(msg, "Do X") {
		t.Error("should contain the directive")
	}
	if !strings.Contains(msg, "STOP. READ THIS FIRST.") {
		t.Error("should contain boilerplate")
	}
}
