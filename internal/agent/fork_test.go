package agent

import (
	"strings"
	"testing"

	"github.com/tetexu/tlaude-code/internal/llm"
)

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
	assistantMsg := llm.Message{
		Role:    "assistant",
		Content: "Let me search.",
		ToolCalls: []llm.ToolCall{
			{ID: "toolu_001", Name: "Glob"},
		},
	}
	forkMsgs := BuildForkedMessages("Do research", assistantMsg)
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

func TestBuildForkedMessages_NoToolUses(t *testing.T) {
	assistantMsg := llm.Message{
		Role:    "assistant",
		Content: "Hi there!",
	}

	msgs := BuildForkedMessages("Research this codebase", assistantMsg)
	if len(msgs) != 1 {
		t.Fatalf("message count = %d, want 1", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("message role = %q, want user", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "<"+ForkBoilerplateTag+">") {
		t.Error("message should contain fork boilerplate tag")
	}
	if !strings.Contains(msgs[0].Content, "Research this codebase") {
		t.Error("message should contain directive")
	}
	if !strings.Contains(msgs[0].Content, ForkDirectivePrefix) {
		t.Error("message should contain directive prefix")
	}
}

func TestBuildForkedMessages_WithToolUses(t *testing.T) {
	assistantMsg := llm.Message{
		Role:    "assistant",
		Content: "Let me search for Go files.",
		ToolCalls: []llm.ToolCall{
			{ID: "toolu_001", Name: "Glob", Args: map[string]interface{}{"pattern": "**/*.go"}},
			{ID: "toolu_002", Name: "Grep", Args: map[string]interface{}{"pattern": "func main"}},
		},
	}

	msgs := BuildForkedMessages("Review the results", assistantMsg)
	if len(msgs) != 2 {
		t.Fatalf("message count = %d, want 2 (assistant clone + placeholder user)", len(msgs))
	}

	// First message should be the cloned assistant
	if msgs[0].Role != "assistant" {
		t.Errorf("first message role = %q, want assistant", msgs[0].Role)
	}
	if len(msgs[0].ToolCalls) != 2 {
		t.Errorf("assistant tool calls = %d, want 2", len(msgs[0].ToolCalls))
	}

	// Second message should be user with placeholder results + directive
	second := msgs[1]
	if second.Role != "user" {
		t.Errorf("second message role = %q, want user", second.Role)
	}
	if !strings.Contains(second.Content, ForkPlaceholderResult) {
		t.Error("should contain placeholder results")
	}
	if !strings.Contains(second.Content, "toolu_001") {
		t.Error("should reference toolu_001")
	}
	if !strings.Contains(second.Content, "toolu_002") {
		t.Error("should reference toolu_002")
	}
	if !strings.Contains(second.Content, "<"+ForkBoilerplateTag+">") {
		t.Error("should contain fork boilerplate tag")
	}
	if !strings.Contains(second.Content, "Review the results") {
		t.Error("should contain directive")
	}
}

func TestBuildForkedMessages_PromptCacheIdenticalPrefix(t *testing.T) {
	// Multiple fork children with different directives should produce
	// byte-identical prefixes for maximum cache sharing.
	assistantMsg := llm.Message{
		Role:    "assistant",
		Content: "Let me search.",
		ToolCalls: []llm.ToolCall{
			{ID: "toolu_001", Name: "Glob", Args: map[string]interface{}{"pattern": "**/*.go"}},
		},
	}

	msgs1 := BuildForkedMessages("Directive A: check auth", assistantMsg)
	msgs2 := BuildForkedMessages("Directive B: check logging", assistantMsg)

	if len(msgs1) != len(msgs2) {
		t.Error("message counts should be identical")
	}

	// First message (cloned assistant) should be identical
	if msgs1[0].Content != msgs2[0].Content {
		t.Error("cloned assistant messages should be identical")
	}
	if len(msgs1[0].ToolCalls) != len(msgs2[0].ToolCalls) {
		t.Error("tool call counts should be identical")
	}

	// Second messages should differ only in the directive portion
	last1 := msgs1[1].Content
	last2 := msgs2[1].Content
	if last1 == last2 {
		t.Error("directives should differ")
	}

	// The placeholder portions should be the same (before boilerplate tag)
	placeholderIdx1 := strings.Index(last1, "<"+ForkBoilerplateTag+">")
	placeholderIdx2 := strings.Index(last2, "<"+ForkBoilerplateTag+">")
	if placeholderIdx1 < 0 || placeholderIdx2 < 0 {
		t.Fatal("boilerplate tag not found")
	}
	prefix1 := last1[:placeholderIdx1]
	prefix2 := last2[:placeholderIdx2]
	if prefix1 != prefix2 {
		t.Error("tool_result placeholder prefixes should be identical (cache sharing)")
	}
}

func TestBuildChildMessage(t *testing.T) {
	tests := []struct {
		name      string
		directive string
		contains  []string
	}{
		{
			name:      "simple directive",
			directive: "Do X",
			contains:  []string{"<" + ForkBoilerplateTag + ">", "</" + ForkBoilerplateTag + ">", ForkDirectivePrefix, "Do X", "STOP. READ THIS FIRST.", "Scope:", "Result:", "Key files:", "Files changed:", "Issues:"},
		},
		{
			name:      "empty directive",
			directive: "",
			contains:  []string{"<" + ForkBoilerplateTag + ">", ForkDirectivePrefix, "STOP. READ THIS FIRST"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := BuildChildMessage(tt.directive)
			for _, want := range tt.contains {
				if !strings.Contains(msg, want) {
					t.Errorf("BuildChildMessage(%q) missing %q", tt.directive, want)
				}
			}
		})
	}
}

func TestBuildWorktreeNotice(t *testing.T) {
	notice := BuildWorktreeNotice("/home/user/project", "/tmp/worktree-abc")
	if !strings.Contains(notice, "/home/user/project") {
		t.Error("should contain parent cwd")
	}
	if !strings.Contains(notice, "/tmp/worktree-abc") {
		t.Error("should contain worktree cwd")
	}
	if !strings.Contains(notice, "isolated git worktree") {
		t.Error("should mention isolated worktree")
	}
	if !strings.Contains(notice, "translate them to your worktree root") {
		t.Error("should include path translation guidance")
	}
}

func TestIsForkSubagentEnabled(t *testing.T) {
	// Cannot reliably test env var state in parallel, but verify the function exists and returns bool
	_ = IsForkSubagentEnabled()
}

func TestBuildForkedMessages_CloneDoesNotMutate(t *testing.T) {
	// Verify that BuildForkedMessages does not mutate the original assistant message
	assistantMsg := llm.Message{
		Role:    "assistant",
		Content: "Original content",
		ToolCalls: []llm.ToolCall{
			{ID: "toolu_001", Name: "Glob", Args: map[string]interface{}{"pattern": "**/*.go"}},
		},
	}

	originalContent := assistantMsg.Content
	originalToolCallsLen := len(assistantMsg.ToolCalls)

	BuildForkedMessages("Test directive", assistantMsg)

	// Original should be unchanged
	if assistantMsg.Content != originalContent {
		t.Error("original assistant message content was mutated")
	}
	if len(assistantMsg.ToolCalls) != originalToolCallsLen {
		t.Error("original assistant message tool calls were mutated")
	}
}
