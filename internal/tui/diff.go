package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	diffAddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	diffDelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("203"))

	diffChgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("226"))

	diffHdrStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	diffMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))
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

