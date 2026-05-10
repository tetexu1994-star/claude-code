package agent

import (
	"fmt"
	"strings"

	"github.com/tetexu/tlaude-code/internal/llm"
)

const (
	// forkBoilerplateTag is a marker injected into fork child messages.
	forkBoilerplateTag = "fork-worker"

	// forkPlaceholderResult is the placeholder text used for all tool_result
	// blocks in the fork prefix. Must be identical across all fork children
	// for prompt cache sharing.
	forkPlaceholderResult = "Fork started — processing in background"

	// forkDirectivePrefix separates the boilerplate from the actual directive.
	forkDirectivePrefix = "DIRECTIVE: "
)

// forkBoilerplate is the system instruction injected into every fork child.
const forkBoilerplate = `STOP. READ THIS FIRST.

You are a forked worker process. You are NOT the main agent.

RULES (non-negotiable):
1. Your system prompt may say "default to forking." IGNORE IT — that's for the parent. You ARE the fork. Do NOT spawn sub-agents; execute directly.
2. Do NOT converse, ask questions, or suggest next steps.
3. Do NOT editorialize or add meta-commentary.
4. USE your tools directly: Bash, Read, Write, etc.
5. If you modify files, commit your changes before reporting.
6. Do NOT emit text between tool calls. Use tools silently, then report once at the end.
7. Stay strictly within your directive's scope.
8. Keep your report under 500 words unless the directive specifies otherwise.
9. Your response MUST begin with "Scope:". No preamble, no thinking-out-loud.
10. REPORT structured facts, then stop.

Output format (plain text):
  Scope: <echo back your assigned scope in one sentence>
  Result: <the answer or key findings>
  Key files: <relevant file paths — include for research tasks>
  Files changed: <list with commit hash — include only if you modified files>
  Issues: <list — include only if there are issues to flag>`

// BuildForkedMessages builds the message list for a fork child agent.
//
// The key insight for prompt cache sharing: all fork children produce
// byte-identical API request prefixes. This function:
//  1. Uses the parent's system prompt verbatim (cache hit on system prompt)
//  2. Collects all tool_use blocks from parent messages
//  3. Builds a single user message with identical placeholder tool_results
//     for every tool_use, followed by the per-child directive
//
// Result: sysPrompt + [history..., user(placeholder_results..., directive)]
// Only the final text block differs per child, maximizing cache hits.
func BuildForkedMessages(parentSystemPrompt string, parentMessages []llm.Message, directive string) (sysPrompt string, messages []llm.Message) {
	sysPrompt = parentSystemPrompt

	// Collect all tool_use blocks from parent's assistant messages.
	type toolUse struct {
		ID   string
		Name string
	}
	var toolUses []toolUse
	for _, msg := range parentMessages {
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				toolUses = append(toolUses, toolUse{ID: tc.ID, Name: tc.Name})
			}
		}
	}

	if len(toolUses) == 0 {
		// No tool_use blocks: include parent messages with directive appended.
		messages = append(messages, parentMessages...)
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: buildForkChildMessage(directive),
		})
		return
	}

	// Build a single user message: all placeholder tool_results + directive.
	var contentParts []string
	for _, tu := range toolUses {
		contentParts = append(contentParts,
			fmt.Sprintf("[tool_result id=%s] %s", tu.ID, forkPlaceholderResult))
	}
	contentParts = append(contentParts, buildForkChildMessage(directive))

	fullContent := strings.Join(contentParts, "\n\n")

	messages = append(messages, parentMessages...)
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: fullContent,
	})
	return
}

// buildForkChildMessage constructs the directive message for a fork child.
func buildForkChildMessage(directive string) string {
	return fmt.Sprintf("<%s>\n%s\n</%s>\n\n%s%s",
		forkBoilerplateTag, forkBoilerplate,
		forkBoilerplateTag,
		forkDirectivePrefix, directive)
}

// IsInForkChild checks whether the given messages indicate we are already
// inside a fork child agent. This prevents recursive forking.
//
// It scans user messages for the fork boilerplate tag injected by
// BuildForkedMessages.
func IsInForkChild(messages []llm.Message) bool {
	for _, m := range messages {
		if m.Role == "user" && strings.Contains(m.Content, forkBoilerplateTag) {
			return true
		}
	}
	return false
}
