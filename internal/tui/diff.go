package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	diffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#22C55E"))

	diffDelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444"))

	diffChgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FBBF24"))

	diffHdrStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A1A1AA"))

	diffMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A1A1AA"))
)

// renderDiffFull generates a full-screen diff view with a header and footer.
func renderDiffFull(diffContent, filePath string) string {
	var sb strings.Builder

	sb.WriteString(diffMetaStyle.Render("─── Diff: " + filePath))
	sb.WriteString(strings.Repeat("─", 40))
	sb.WriteString("\n\n")

	if diffContent == "" {
		sb.WriteString(diffMetaStyle.Render("(no changes)"))
	} else {
		sb.WriteString(diffContent)
	}

	sb.WriteString("\n")
	sb.WriteString(diffMetaStyle.Render("─── Press Esc to return"))
	sb.WriteString(strings.Repeat("─", 40))

	return sb.String()
}

