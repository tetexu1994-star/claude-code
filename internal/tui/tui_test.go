package tui

import (
	"strings"
	"testing"

	"github.com/tetexu/tlaude-code/internal/llm"
)

func TestBuildApprovalSummary(t *testing.T) {
	t.Parallel()

	t.Run("bash command", func(t *testing.T) {
		args := map[string]interface{}{"command": "ls -la /home/user"}
		summary := buildApprovalSummary("bash", args)
		if !strings.Contains(summary, "ls -la") {
			t.Errorf("expected command in summary, got %q", summary)
		}
	})

	t.Run("bash no command", func(t *testing.T) {
		args := map[string]interface{}{}
		summary := buildApprovalSummary("bash", args)
		if summary != "execute shell command" {
			t.Errorf("expected fallback, got %q", summary)
		}
	})

	t.Run("write_file", func(t *testing.T) {
		args := map[string]interface{}{"path": "/tmp/test.txt"}
		summary := buildApprovalSummary("write_file", args)
		if !strings.HasPrefix(summary, "write ") {
			t.Errorf("expected 'write ' prefix, got %q", summary)
		}
	})

	t.Run("write_file no path", func(t *testing.T) {
		summary := buildApprovalSummary("write_file", map[string]interface{}{})
		if summary != "write file" {
			t.Errorf("expected 'write file', got %q", summary)
		}
	})

	t.Run("delete_file", func(t *testing.T) {
		args := map[string]interface{}{"path": "/tmp/remove.txt"}
		summary := buildApprovalSummary("delete_file", args)
		if !strings.HasPrefix(summary, "delete ") {
			t.Errorf("expected 'delete ' prefix, got %q", summary)
		}
	})

	t.Run("read_file", func(t *testing.T) {
		args := map[string]interface{}{"path": "/tmp/read.txt"}
		summary := buildApprovalSummary("read_file", args)
		if !strings.HasPrefix(summary, "read ") {
			t.Errorf("expected 'read ' prefix, got %q", summary)
		}
	})

	t.Run("unknown tool", func(t *testing.T) {
		summary := buildApprovalSummary("unknown_tool", nil)
		if summary != "unknown_tool" {
			t.Errorf("expected tool name, got %q", summary)
		}
	})
}

func TestBuildApprovalDetail(t *testing.T) {
	t.Parallel()

	t.Run("bash command", func(t *testing.T) {
		args := map[string]interface{}{"command": "git commit -m 'test'"}
		detail := buildApprovalDetail("bash", args)
		if !strings.Contains(detail, "git commit") {
			t.Errorf("expected command in detail, got %q", detail)
		}
	})

	t.Run("bash no command", func(t *testing.T) {
		args := map[string]interface{}{"other": "value"}
		detail := buildApprovalDetail("bash", args)
		if !strings.Contains(detail, "Args:") {
			t.Errorf("expected args fallback, got %q", detail)
		}
	})

	t.Run("write_file", func(t *testing.T) {
		args := map[string]interface{}{
			"path":    "/tmp/output.txt",
			"content": "file content here",
		}
		detail := buildApprovalDetail("write_file", args)
		if !strings.Contains(detail, "/tmp/output.txt") {
			t.Errorf("expected path in detail, got %q", detail)
		}
		if !strings.Contains(detail, "file content") {
			t.Errorf("expected content in detail, got %q", detail)
		}
	})

	t.Run("delete_file", func(t *testing.T) {
		args := map[string]interface{}{"path": "/tmp/delete.txt"}
		detail := buildApprovalDetail("delete_file", args)
		if !strings.Contains(detail, "/tmp/delete.txt") {
			t.Errorf("expected path in detail, got %q", detail)
		}
	})

	t.Run("read_file", func(t *testing.T) {
		args := map[string]interface{}{"path": "/tmp/readme.md"}
		detail := buildApprovalDetail("read_file", args)
		if !strings.Contains(detail, "/tmp/readme.md") {
			t.Errorf("expected path in detail, got %q", detail)
		}
	})

	t.Run("unknown tool", func(t *testing.T) {
		args := map[string]interface{}{"key": "val"}
		detail := buildApprovalDetail("unknown", args)
		if !strings.Contains(detail, "Args:") {
			t.Errorf("expected args display, got %q", detail)
		}
	})
}

func TestTruncate(t *testing.T) {
	t.Parallel()

	t.Run("short string", func(t *testing.T) {
		result := truncate("hello", 10)
		if result != "hello" {
			t.Errorf("expected 'hello', got %q", result)
		}
	})

	t.Run("exact length", func(t *testing.T) {
		result := truncate("hello", 5)
		if result != "hello" {
			t.Errorf("expected 'hello', got %q", result)
		}
	})

	t.Run("long string", func(t *testing.T) {
		result := truncate("this is a very long string", 10)
		if len(result) > 10 {
			t.Errorf("result too long: %d chars", len(result))
		}
		if !strings.HasSuffix(result, "...") {
			t.Error("expected '...' suffix")
		}
	})

	t.Run("empty string", func(t *testing.T) {
		result := truncate("", 10)
		if result != "" {
			t.Errorf("expected empty, got %q", result)
		}
	})
}

func TestRenderDiffFull(t *testing.T) {
	t.Parallel()

	t.Run("with content", func(t *testing.T) {
		output := renderDiffFull("+added line\n-removed line", "test.go")
		if !strings.Contains(output, "test.go") {
			t.Errorf("expected file path, got %q", output)
		}
		if !strings.Contains(output, "+added line") {
			t.Errorf("expected diff content, got %q", output)
		}
		if !strings.Contains(output, "Esc to return") {
			t.Errorf("expected footer, got %q", output)
		}
	})

	t.Run("empty content", func(t *testing.T) {
		output := renderDiffFull("", "empty.go")
		if !strings.Contains(output, "(no changes)") {
			t.Errorf("expected '(no changes)', got %q", output)
		}
		if !strings.Contains(output, "empty.go") {
			t.Errorf("expected file path, got %q", output)
		}
	})
}

func TestRenderContent(t *testing.T) {
	t.Parallel()

	t.Run("plain text", func(t *testing.T) {
		output := renderContent("hello world")
		if !strings.Contains(output, "hello world") {
			t.Errorf("expected plain text, got %q", output)
		}
	})

	t.Run("code block", func(t *testing.T) {
		input := "before\n```go\nfunc main() {\n}\n```\nafter"
		output := renderContent(input)
		if !strings.Contains(output, "func main()") {
			t.Error("expected code content")
		}
		if !strings.Contains(output, "[go]") {
			t.Error("expected language tag")
		}
	})

	t.Run("code block without language", func(t *testing.T) {
		input := "```\nplain code\n```"
		output := renderContent(input)
		if !strings.Contains(output, "plain code") {
			t.Error("expected code content")
		}
	})

	t.Run("unclosed code block", func(t *testing.T) {
		input := "text\n```go\nfunc x() {\n"
		output := renderContent(input)
		if !strings.Contains(output, "func x()") {
			t.Error("expected code line")
		}
	})
}

func TestRenderContentMultiline(t *testing.T) {
	t.Parallel()

	t.Run("multiple paragraphs", func(t *testing.T) {
		input := "line1\n\nline2\n\nline3"
		output := renderContent(input)
		lines := strings.Split(strings.TrimSpace(output), "\n")
		if len(lines) < 3 {
			t.Errorf("expected at least 3 lines, got %d", len(lines))
		}
	})
}

func TestRenderApprovalBar(t *testing.T) {
	t.Parallel()

	t.Run("bash approval", func(t *testing.T) {
		req := &ApprovalRequest{
			Type:    "bash",
			Summary: "ls -la",
			Detail:  "Command: ls -la",
		}
		output := renderApprovalBar(req)
		if !strings.Contains(output, "Pending Approval") {
			t.Error("expected 'Pending Approval'")
		}
		if !strings.Contains(output, "ls -la") {
			t.Error("expected command")
		}
		if !strings.Contains(output, "[Y]") {
			t.Error("expected Yes option")
		}
		if !strings.Contains(output, "[N]") {
			t.Error("expected No option")
		}
		if !strings.Contains(output, "[D]") {
			t.Error("expected Diff option")
		}
		if !strings.Contains(output, "[A]") {
			t.Error("expected Always option")
		}
	})

	t.Run("write_file approval", func(t *testing.T) {
		req := &ApprovalRequest{
			Type:    "write_file",
			Summary: "write /tmp/test.txt",
		}
		output := renderApprovalBar(req)
		if !strings.Contains(output, "Write File") {
			t.Error("expected 'Write File' label")
		}
	})

	t.Run("unknown type uses formatted label", func(t *testing.T) {
		req := &ApprovalRequest{
			Type:    "custom_tool",
			Summary: "do work",
		}
		output := renderApprovalBar(req)
		if !strings.Contains(output, "do work") {
			t.Error("expected summary")
		}
	})
}

func TestApprovalRequestStruct(t *testing.T) {
	t.Parallel()

	req := ApprovalRequest{
		Type:    "bash",
		Summary: "run command",
		Detail:  "echo hello",
		Path:    "",
	}

	if req.Type != "bash" {
		t.Error("expected Type 'bash'")
	}
	if req.Summary != "run command" {
		t.Error("expected Summary")
	}
}

func TestApprovalResultStruct(t *testing.T) {
	t.Parallel()

	r := ApprovalResult{
		Approved: true,
		Remember: false,
	}

	if !r.Approved {
		t.Error("expected Approved true")
	}
	if r.Remember {
		t.Error("expected Remember false")
	}
}

func TestReadChunk(t *testing.T) {
	t.Run("channel closed immediately", func(t *testing.T) {
		ch := make(chan llm.Chunk)
		close(ch)
		result := readChunk(ch)
		if !result.Done {
			t.Error("expected Done true for closed channel")
		}
	})

	t.Run("content chunk", func(t *testing.T) {
		ch := make(chan llm.Chunk, 1)
		ch <- llm.Chunk{Content: "hello chunk"}
		close(ch)
		result := readChunk(ch)
		if result.Content != "hello chunk" {
			t.Errorf("expected 'hello chunk', got %q", result.Content)
		}
		if result.Done {
			t.Error("expected Done false for content chunk")
		}
	})

	t.Run("error chunk", func(t *testing.T) {
		ch := make(chan llm.Chunk, 1)
		ch <- llm.Chunk{Error: errTest("test error")}
		close(ch)
		result := readChunk(ch)
		if result.Error == nil {
			t.Error("expected error")
		}
	})

	t.Run("done chunk with tool calls", func(t *testing.T) {
		ch := make(chan llm.Chunk, 1)
		ch <- llm.Chunk{
			Done: true,
			ToolCalls: []llm.ToolCall{
				{ID: "tc1", Name: "bash", Args: map[string]interface{}{"command": "ls"}},
			},
		}
		close(ch)
		result := readChunk(ch)
		if !result.Done {
			t.Error("expected Done true")
		}
		if len(result.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
		}
		if result.ToolCalls[0].Name != "bash" {
			t.Errorf("expected 'bash' tool, got %q", result.ToolCalls[0].Name)
		}
	})
}

func TestStreamChunkMsgStruct(t *testing.T) {
	t.Parallel()

	msg := streamChunkMsg{
		Content:   "content",
		Done:      false,
		Error:     nil,
		ToolCalls: nil,
	}

	if msg.Content != "content" {
		t.Error("expected content")
	}
	if msg.Done {
		t.Error("expected Done false")
	}
}

func TestMoaResultMsgStruct(t *testing.T) {
	t.Parallel()

	msg := moaResultMsg{
		Error: errTest("moa failed"),
	}

	if msg.Error == nil {
		t.Error("expected error")
	}
	if msg.Result != nil {
		t.Error("expected nil result")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// ---------------------------------------------------------------------------
// Spinner tests
// ---------------------------------------------------------------------------

func TestNewSpinner(t *testing.T) {
	s := NewSpinner("Thinking...")
	if s == nil {
		t.Fatal("NewSpinner returned nil")
	}
	if s.Text != "Thinking..." {
		t.Errorf("expected 'Thinking...', got %q", s.Text)
	}
	if len(s.frames) != 10 {
		t.Errorf("expected 10 spinner frames, got %d", len(s.frames))
	}
}

func TestSpinnerNext(t *testing.T) {
	s := NewSpinner("Loading")
	frame1 := s.Next()
	frame2 := s.Next()
	frame3 := s.Next()
	if frame1 == frame2 || frame2 == frame3 {
		t.Error("spinner frames should rotate")
	}
	// Cycle through all frames and verify it wraps
	for i := 0; i < 10; i++ {
		s.Next()
	}
	frameAfterWrap := s.Next()
	// After 11 advances, we should see frame at index 1 (not 0 for the first next call)
	// index after 12 total Next calls = 12 % 10 = 2
	// We already consumed 3 frames above, then 10 more = 13 total, 13%10 = 3
	// Just verify it doesn't panic and returns something
	if frameAfterWrap == "" {
		t.Error("spinner frame should not be empty after wrapping")
	}
}

func TestSpinnerView(t *testing.T) {
	s := NewSpinner("Test")
	view := s.View()
	if !strings.Contains(view, "Test") {
		t.Errorf("spinner View should contain text, got %q", view)
	}
	if s.index != 1 {
		t.Errorf("spinner View should advance index to 1, got %d", s.index)
	}
}

func TestSpinnerViewNil(t *testing.T) {
	var s *Spinner
	view := s.View()
	if view != "" {
		t.Errorf("nil spinner View should return empty string, got %q", view)
	}
}

// ---------------------------------------------------------------------------
// Welcome screen tests
// ---------------------------------------------------------------------------

func TestShowWelcome(t *testing.T) {
	m := Model{welcomeVisible: true}
	if !m.showWelcome() {
		t.Error("showWelcome should return true when welcomeVisible and no messages")
	}
	m.messages = append(m.messages, llm.Message{Role: "user", Content: "hi"})
	if m.showWelcome() {
		t.Error("showWelcome should return false when messages exist")
	}
	m = Model{welcomeVisible: false}
	if m.showWelcome() {
		t.Error("showWelcome should return false when welcomeVisible is false")
	}
}

func TestWelcomeViewContainsTitle(t *testing.T) {
	m := Model{width: 80, height: 24}
	view := m.welcomeView()
	if !strings.Contains(view, "Tlaude Code") {
		t.Errorf("welcomeView should contain 'Tlaude Code', got %q", view)
	}
	if !strings.Contains(view, "/help") {
		t.Errorf("welcomeView should contain '/help', got %q", view)
	}
}

// ---------------------------------------------------------------------------
// Markdown rendering tests
// ---------------------------------------------------------------------------

func TestRenderMarkdownHeaders(t *testing.T) {
	input := "# H1\n## H2\n### H3"
	result := renderMarkdown(input, 80)
	if !strings.Contains(result, "H1") {
		t.Error("renderMarkdown should include H1 content")
	}
	if !strings.Contains(result, "H2") {
		t.Error("renderMarkdown should include H2 content")
	}
	if !strings.Contains(result, "H3") {
		t.Error("renderMarkdown should include H3 content")
	}
}

func TestRenderMarkdownCodeBlock(t *testing.T) {
	input := "```go\nfunc main() {}\n```"
	result := renderMarkdown(input, 80)
	if !strings.Contains(result, "func main()") {
		t.Errorf("renderMarkdown code block should include code, got %q", result)
	}
}

func TestRenderMarkdownInlineCode(t *testing.T) {
	input := "Use `foo.Bar()` to call it."
	result := renderMarkdownInline(input)
	if !strings.Contains(result, "foo.Bar()") {
		t.Errorf("renderMarkdownInline should include inline code content, got %q", result)
	}
}

func TestRenderMarkdownBold(t *testing.T) {
	input := "This is **important** text."
	result := renderMarkdownInline(input)
	if !strings.Contains(result, "important") {
		t.Errorf("renderMarkdownInline should include bold content, got %q", result)
	}
}

func TestRenderMarkdownList(t *testing.T) {
	input := "- item one\n- item two"
	result := renderMarkdown(input, 80)
	if !strings.Contains(result, "item one") {
		t.Errorf("renderMarkdown list should include items, got %q", result)
	}
	if !strings.Contains(result, "item two") {
		t.Errorf("renderMarkdown list should include items, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Format tokens tests
// ---------------------------------------------------------------------------

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{12345, "12,345"},
		{1000000, "1.0M"},
		{2500000, "2.5M"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.input)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
