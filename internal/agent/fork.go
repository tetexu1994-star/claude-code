package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/tetexu/tlaude-code/internal/llm"
)

const (
	// ForkBoilerplateTag is the XML tag wrapping fork child instructions.
	// Detected by IsInForkChild to prevent recursive forking.
	ForkBoilerplateTag = "fork-boilerplate"

	// ForkDirectivePrefix separates boilerplate from the actual directive.
	ForkDirectivePrefix = "DIRECTIVE: "

	// ForkPlaceholderResult is the placeholder text used for all tool_result
	// blocks in the fork prefix. Must be identical across all fork children
	// for prompt cache sharing.
	ForkPlaceholderResult = "Fork started - processing in background"

	// ForkSubagentType is the synthetic agent type for fork sub-agents.
	ForkSubagentType = "fork"
)

// IsForkSubagentEnabled checks whether fork subagent mode is enabled.
//
// When enabled:
//   - Omitting subagent_type in the Agent tool triggers an implicit fork
//   - Fork child inherits parent's conversation context and system prompt
//   - All spawns run in background (async) for <task-notification> model
//
// Mutually exclusive with coordinator mode.
func IsForkSubagentEnabled() bool {
	return os.Getenv("TLAUDE_CODE_FORK_SUBAGENT") == "1"
}

// IsInForkChild checks whether the given messages indicate we are already
// inside a fork child agent. This prevents recursive forking.
//
// It scans user messages for the fork boilerplate tag injected by
// BuildForkedMessages.
func IsInForkChild(messages []llm.Message) bool {
	for _, m := range messages {
		if m.Role == "user" && strings.Contains(m.Content, "<"+ForkBoilerplateTag+">") {
			return true
		}
	}
	return false
}

// BuildForkedMessages builds the conversation context for a fork child.
//
// For prompt cache sharing, all fork children must produce byte-identical
// API request prefixes. This function:
//  1. Clones the assistant message with all tool_use blocks
//  2. Builds a single user message with identical placeholder tool_results
//     for every tool_use, followed by the per-child directive
//
// Result: [assistant(all_tool_uses), user(placeholder_results..., directive)]
// Only the final text block differs per child, maximizing cache hits.
func BuildForkedMessages(directive string, assistantMsg llm.Message) []llm.Message {
	// Clone the assistant message to avoid mutating the original
	clonedAssistant := llm.Message{
		Role:      assistantMsg.Role,
		Content:   assistantMsg.Content,
		ToolCalls: make([]llm.ToolCall, len(assistantMsg.ToolCalls)),
		ToolID:    assistantMsg.ToolID,
	}
	copy(clonedAssistant.ToolCalls, assistantMsg.ToolCalls)

	toolUseBlocks := assistantMsg.ToolCalls

	if len(toolUseBlocks) == 0 {
		// No tool_use blocks: just the directive with boilerplate
		return []llm.Message{
			{
				Role:    "user",
				Content: BuildChildMessage(directive),
			},
		}
	}

	// Build placeholder tool_result strings for every tool_use block
	var contentParts []string
	for _, tc := range toolUseBlocks {
		contentParts = append(contentParts,
			fmt.Sprintf("[tool_result id=%s] %s", tc.ID, ForkPlaceholderResult))
	}
	// Append the per-child directive
	contentParts = append(contentParts, BuildChildMessage(directive))

	return []llm.Message{
		clonedAssistant,
		{
			Role:    "user",
			Content: strings.Join(contentParts, "\n\n"),
		},
	}
}

// BuildChildMessage constructs the fork boilerplate message with directive.
//
// Wraps the directive in fork-boilerplate XML tags with strict rules for the
// forked worker. The child must NOT spawn sub-agents, must NOT converse, and
// must output structured facts.
func BuildChildMessage(directive string) string {
	return fmt.Sprintf(`<%[1]s>
STOP. READ THIS FIRST.

You are a forked worker process. You are NOT the main agent.

RULES (non-negotiable):
1. Your system prompt says "default to forking." IGNORE IT — that's for the parent. You ARE the fork. Do NOT spawn sub-agents; execute directly.
2. Do NOT converse, ask questions, or suggest next steps
3. Do NOT editorialize or add meta-commentary
4. USE your tools directly: Bash, Read, Write, etc.
5. If you modify files, commit your changes before reporting. Include the commit hash in your report.
6. Do NOT emit text between tool calls. Use tools silently, then report once at the end.
7. Stay strictly within your directive's scope. If you discover related systems outside your scope, mention them in one sentence at most — other workers cover those areas.
8. Keep your report under 500 words unless the directive specifies otherwise. Be factual and concise.
9. Your response MUST begin with "Scope:". No preamble, no thinking-out-loud.
10. REPORT structured facts, then stop

Output format (plain text labels, not markdown headers):
  Scope: <echo back your assigned scope in one sentence>
  Result: <the answer or key findings, limited to the scope above>
  Key files: <relevant file paths — include for research tasks>
  Files changed: <list with commit hash — include only if you modified files>
  Issues: <list — include only if there are issues to flag>
</%[1]s>

%s%s`, ForkBoilerplateTag, ForkDirectivePrefix, directive)
}

// BuildWorktreeNotice returns a path translation notice for fork children
// running in an isolated git worktree.
//
// Tells the child to translate paths from inherited context, re-read
// potentially stale files, and that its changes are isolated.
func BuildWorktreeNotice(parentCwd, worktreeCwd string) string {
	return fmt.Sprintf(
		"You've inherited the conversation context above from a parent agent working in %s. "+
			"You are operating in an isolated git worktree at %s — same repository, same relative file structure, separate working copy. "+
			"Paths in the inherited context refer to the parent's working directory; translate them to your worktree root. "+
			"Re-read files before editing if the parent may have modified them since they appear in the context. "+
			"Your changes stay in this worktree and will not affect the parent's files.",
		parentCwd, worktreeCwd)
}
