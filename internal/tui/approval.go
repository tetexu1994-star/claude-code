package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/tetexu/tlaude-code/internal/llm"
)

// ApprovalRequest captures a tool invocation that needs user confirmation.
type ApprovalRequest struct {
	Type    string // "bash", "write_file", "delete_file", "read_file"
	Summary string // user-facing one-line description
	Detail  string // full content (command, file content, diff)
	Path    string // file path (for file operations)
	TC      llm.ToolCall
}

// ApprovalResult is the user's decision on an ApprovalRequest.
type ApprovalResult struct {
	Approved bool
	Remember bool // persist to AlwaysAllowPatterns
}

var (
	approvalBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("226")).
				Padding(0, 1).
				Width(70)

	approvalTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("226"))

	approvalSummaryStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))

	approvalDetailStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243"))

	approvalKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	approvalDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))
)

// buildApprovalSummary creates a one-line summary from a tool call.
func buildApprovalSummary(name string, args map[string]interface{}) string {
	switch name {
	case "bash":
		if cmd, ok := args["command"].(string); ok {
			return truncate(cmd, 60)
		}
		return "execute shell command"
	case "write_file":
		if path, ok := args["path"].(string); ok {
			return "write " + truncate(path, 50)
		}
		return "write file"
	case "delete_file":
		if path, ok := args["path"].(string); ok {
			return "delete " + truncate(path, 50)
		}
		return "delete file"
	case "read_file":
		if path, ok := args["path"].(string); ok {
			return "read " + truncate(path, 50)
		}
		return "read file"
	default:
		return name
	}
}

// buildApprovalDetail creates the full detail text for a tool call.
func buildApprovalDetail(name string, args map[string]interface{}) string {
	switch name {
	case "bash":
		if cmd, ok := args["command"].(string); ok {
			return "Command: " + cmd
		}
		return fmt.Sprintf("Args: %v", args)
	case "write_file":
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		return fmt.Sprintf("File: %s\nContent:\n%s", path, truncate(content, 500))
	case "delete_file":
		path, _ := args["path"].(string)
		return fmt.Sprintf("File: %s", path)
	case "read_file":
		path, _ := args["path"].(string)
		return fmt.Sprintf("File: %s", path)
	default:
		return fmt.Sprintf("Args: %v", args)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// renderApprovalBar renders the approval prompt bar shown below the chat.
func renderApprovalBar(req *ApprovalRequest) string {
	var sb strings.Builder
	sb.WriteString(approvalTitleStyle.Render("┌─ Pending Approval"))
	sb.WriteString(strings.Repeat("─", 40))
	sb.WriteByte('\n')

	label := map[string]string{
		"bash":        "Command",
		"write_file":  "Write File",
		"delete_file": "Delete File",
		"read_file":   "Read File",
	}[req.Type]
	if label == "" {
		label = strings.Title(strings.ReplaceAll(req.Type, "_", " "))
	}
	sb.WriteString(fmt.Sprintf("  %s: %s\n", label, approvalSummaryStyle.Render(req.Summary)))

	if req.Detail != "" {
		for _, line := range strings.Split(req.Detail, "\n") {
			sb.WriteString(fmt.Sprintf("  %s\n", approvalDetailStyle.Render(line)))
		}
	}

	sb.WriteByte('\n')
	sb.WriteString(fmt.Sprintf("  %s %s  %s %s  %s %s  %s %s\n",
		approvalKeyStyle.Render("[Y]"), approvalDescStyle.Render("Yes"),
		approvalKeyStyle.Render("[N]"), approvalDescStyle.Render("No"),
		approvalKeyStyle.Render("[D]"), approvalDescStyle.Render("Diff"),
		approvalKeyStyle.Render("[A]"), approvalDescStyle.Render("Always"),
	))
	sb.WriteString(strings.Repeat("─", 70))

	return approvalBoxStyle.Render(sb.String())
}
