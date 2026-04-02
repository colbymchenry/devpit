package core

import "github.com/charmbracelet/lipgloss"

// StatusStyle returns the appropriate style for a run status.
func StatusStyle(status string) lipgloss.Style {
	switch status {
	case "running":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true)
	case "passed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Bold(true)
	case "failed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
	case "skipped":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	}
}

// StatusIcon returns a status indicator character.
func StatusIcon(status string) string {
	switch status {
	case "running":
		return "~"
	case "passed":
		return "*"
	case "failed":
		return "x"
	case "skipped":
		return "-"
	default:
		return "."
	}
}
