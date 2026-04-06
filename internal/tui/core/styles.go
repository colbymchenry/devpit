package core

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Color palette ──────────────────────────────────────────────────────

const (
	ColorPurple      = "#7C3AED"
	ColorPurpleLight = "#A78BFA"
	ColorPurpleDim   = "#6D28D9"
	ColorGreen       = "#22C55E"
	ColorRed         = "#EF4444"
	ColorAmber       = "#F59E0B"
	ColorText        = "#D1D5DB"
	ColorMuted       = "#6B7280"
	ColorDim         = "#4B5563"
	ColorBorder      = "#374151"
	ColorBorderFocus = "#7C3AED"
	ColorSelectedBg  = "#1E1B4B"
	ColorHeaderBg    = "#111827"
	ColorWhite       = "#FFFFFF"
)

// ── Status helpers ─────────────────────────────────────────────────────

// StatusStyle returns the appropriate style for a run status.
func StatusStyle(status string) lipgloss.Style {
	switch status {
	case "running":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAmber)).Bold(true)
	case "passed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(ColorGreen)).Bold(true)
	case "failed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(ColorRed)).Bold(true)
	case "skipped", "canceled":
		return lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMuted))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim))
	}
}

// ShimmerStyle returns a style with a color that oscillates between dim and
// bright amber based on the animation frame.
func ShimmerStyle(frame int) lipgloss.Style {
	t := math.Sin(float64(frame) * 2 * math.Pi / 13)
	t = (t + 1) / 2

	r := lerp(160, 251, t)
	g := lerp(100, 191, t)
	b := lerp(9, 36, t)

	color := fmt.Sprintf("#%02X%02X%02X", r, g, b)
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true)
}

func lerp(a, b int, t float64) int {
	return a + int(float64(b-a)*t)
}

// StatusIcon returns a status indicator character.
func StatusIcon(status string) string {
	switch status {
	case "running":
		return "●"
	case "passed":
		return "✓"
	case "failed":
		return "✗"
	case "skipped":
		return "○"
	case "canceled":
		return "⊘"
	case "pending":
		return "·"
	default:
		return "·"
	}
}

// StatusLabel returns a human-readable label for a status.
func StatusLabel(status string) string {
	switch status {
	case "running":
		return "Running"
	case "passed":
		return "Passed"
	case "failed":
		return "Failed"
	case "skipped":
		return "Skipped"
	case "canceled":
		return "Canceled"
	case "pending":
		return "Pending"
	default:
		return "Unknown"
	}
}

// ── Panel rendering ────────────────────────────────────────────────────

// PanelTop draws the top border with an embedded title.
//
//	╭─ Title ──────────────────────╮
func PanelTop(title string, width int, color lipgloss.Color) string {
	bs := lipgloss.NewStyle().Foreground(color)
	ts := lipgloss.NewStyle().Foreground(color).Bold(true)

	if title == "" {
		return bs.Render("╭" + strings.Repeat("─", width-2) + "╮")
	}

	titleRendered := ts.Render(title)
	titleWidth := lipgloss.Width(titleRendered)
	fill := width - 5 - titleWidth // "╭─ " (3) + " ╮" (2)
	if fill < 0 {
		fill = 0
	}
	return bs.Render("╭─ ") + titleRendered + bs.Render(" "+strings.Repeat("─", fill)+"╮")
}

// PanelRow draws a single row inside a panel, padded to width.
//
//	│ content                       │
func PanelRow(content string, width int, color lipgloss.Color) string {
	bs := lipgloss.NewStyle().Foreground(color)
	contentWidth := lipgloss.Width(content)
	inner := width - 4 // "│ " + " │"
	pad := inner - contentWidth
	if pad < 0 {
		pad = 0
	}
	return bs.Render("│") + " " + content + strings.Repeat(" ", pad) + " " + bs.Render("│")
}

// PanelBottom draws the bottom border.
//
//	╰──────────────────────────────╯
func PanelBottom(width int, color lipgloss.Color) string {
	bs := lipgloss.NewStyle().Foreground(color)
	return bs.Render("╰" + strings.Repeat("─", width-2) + "╯")
}

// PanelSeparator draws a horizontal line inside a panel.
//
//	├──────────────────────────────┤
func PanelSeparator(width int, color lipgloss.Color) string {
	bs := lipgloss.NewStyle().Foreground(color)
	return bs.Render("├" + strings.Repeat("─", width-2) + "┤")
}

// PanelEmpty draws an empty row.
func PanelEmpty(width int, color lipgloss.Color) string {
	return PanelRow("", width, color)
}

// ── Table helpers ──────────────────────────────────────────────────────

// PadRight pads a visible string to a given width. It's ANSI-safe —
// it measures the visible width with lipgloss.Width.
func PadRight(s string, width int) string {
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

// Truncate truncates a string to maxWidth visible characters,
// adding "…" if truncated.
func Truncate(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	// Simple rune-based truncation (works for plain text without ANSI)
	runes := []rune(s)
	if maxWidth <= 1 {
		return "…"
	}
	if len(runes) > maxWidth-1 {
		return string(runes[:maxWidth-1]) + "…"
	}
	return s
}
