package compact

import "github.com/tetexu/tlaude-code/internal/llm"

// ClearableTools is the set of tools whose results can be cleared by
// MicroCompact.
var ClearableTools = map[string]bool{
	"Bash":     true,
	"Grep":     true,
	"Glob":     true,
	"Read":     true,
	"WebFetch": true,
	"WebSearch": true,
	"FileEdit":  true,
	"FileWrite": true,
}

// ContentClearableTools are tools where only the result content can be cleared
// (not the tool call itself). FileEdit and FileWrite are handled separately
// below because they also have clearable inputs.
var ContentClearableTools = map[string]bool{
	"Bash":      true,
	"Grep":      true,
	"Glob":      true,
	"Read":      true,
	"WebFetch":  true,
	"WebSearch": true,
}

const clearedToolResult = "[tool result cleared]"

// MicroCompact clears verbose tool results from older messages, keeping only
// the most recent keepRecent results per tool type. This is lightweight
// garbage collection that runs after each API response.
func MicroCompact(messages []llm.Message, keepRecent int) []llm.Message {
	if keepRecent <= 0 {
		keepRecent = 3
	}

	// Track how many recent results we've seen per tool type.
	toolResultCounts := make(map[string]int)

	// First pass: count from the end to know which results to keep.
	type toolResultRef struct {
		msgIdx int
		tcIdx  int
		keep   bool
	}
	var refs []toolResultRef

	// Walk messages in reverse to mark which tool results to keep.
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role != "assistant" || len(msg.ToolCalls) == 0 {
			continue
		}
		for j := range msg.ToolCalls {
			tc := &msg.ToolCalls[j]
			if !ClearableTools[tc.Name] {
				continue
			}
			count := toolResultCounts[tc.Name]
			keep := count < keepRecent
			if tc.Result == "" || tc.Result == clearedToolResult {
				keep = false // nothing to clear
			}
			refs = append(refs, toolResultRef{
				msgIdx: i,
				tcIdx:  j,
				keep:   keep,
			})
			if keep {
				toolResultCounts[tc.Name]++
			}
		}
	}

	// Second pass: clear old results.
	result := make([]llm.Message, len(messages))
	copy(result, messages)

	for _, ref := range refs {
		if ref.keep {
			continue
		}
		tc := &result[ref.msgIdx].ToolCalls[ref.tcIdx]
		if tc.Result != "" && tc.Result != clearedToolResult {
			tc.Result = clearedToolResult
		}
	}

	// Third pass: clear tool results stored as separate user messages
	// (tool_id-prefixed messages).
	keepToolIDs := make(map[string]bool)
	for _, msg := range messages {
		if msg.Role == "assistant" {
			for _, tc := range msg.ToolCalls {
				if toolResultCounts[tc.Name] > 0 {
					keepToolIDs[tc.ID] = true
					toolResultCounts[tc.Name]--
				}
			}
		}
	}

	for i := range result {
		if result[i].Role == "tool" && result[i].ToolID != "" {
			if !keepToolIDs[result[i].ToolID] {
				if result[i].Content != "" && result[i].Content != clearedToolResult {
					result[i].Content = clearedToolResult
				}
			}
		}
	}

	return result
}
