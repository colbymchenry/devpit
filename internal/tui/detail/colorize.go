package detail

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/colbymchenry/devpit/internal/tui/core"
)

var (
	styleH1     = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Bold(true)
	styleH2     = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurple)).Bold(true)
	styleH3     = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorText)).Bold(true)
	styleBullet = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight))
	styleCode   = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))
	styleQuote  = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).Italic(true)
	stylePass   = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorGreen)).Bold(true)
	styleFail   = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorRed)).Bold(true)
	styleBold   = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorWhite)).Bold(true)
	styleHR     = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))
	styleFence  = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))

	boldRe = regexp.MustCompile(`\*\*(.+?)\*\*`)
)

// colorizeOutput adds lipgloss colors to plain-text / markdown output.
// If the text already contains ANSI escape codes (live tmux capture), it is
// returned as-is since it already has colors.
func colorizeOutput(s string) string {
	if strings.Contains(s, "\x1b[") {
		return s
	}

	lines := strings.Split(s, "\n")
	inCodeBlock := false
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Code fence toggle
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			out = append(out, styleFence.Render(line))
			continue
		}
		if inCodeBlock {
			out = append(out, styleCode.Render(line))
			continue
		}

		// Headers (check ### before ## before #)
		if strings.HasPrefix(trimmed, "### ") {
			out = append(out, styleH3.Render(line))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			out = append(out, styleH2.Render(line))
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			out = append(out, styleH1.Render(line))
			continue
		}

		// Horizontal rules
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			out = append(out, styleHR.Render(line))
			continue
		}

		// Blockquotes
		if strings.HasPrefix(trimmed, "> ") {
			out = append(out, styleQuote.Render(line))
			continue
		}

		// Result markers
		upper := strings.ToUpper(trimmed)
		if upper == "PASS" || upper == "ALL CLEAR" {
			out = append(out, stylePass.Render(line))
			continue
		}
		if upper == "FAIL" || upper == "ISSUES FOUND" {
			out = append(out, styleFail.Render(line))
			continue
		}

		// Bullets
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
			strings.HasPrefix(trimmed, "● ") || strings.HasPrefix(trimmed, "• ") {
			out = append(out, colorBulletLine(line, trimmed))
			continue
		}

		// Inline bold **text**
		if boldRe.MatchString(line) {
			line = boldRe.ReplaceAllStringFunc(line, func(m string) string {
				return styleBold.Render(m[2 : len(m)-2])
			})
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

// colorBulletLine colors the bullet character while leaving the rest as default text.
func colorBulletLine(line, trimmed string) string {
	indent := line[:len(line)-len(trimmed)]

	// Find the bullet prefix and rest
	for _, prefix := range []string{"● ", "• ", "- ", "* "} {
		if strings.HasPrefix(trimmed, prefix) {
			rest := trimmed[len(prefix):]
			// Apply inline bold to the rest
			if boldRe.MatchString(rest) {
				rest = boldRe.ReplaceAllStringFunc(rest, func(m string) string {
					return styleBold.Render(m[2 : len(m)-2])
				})
			}
			return indent + styleBullet.Render(prefix) + rest
		}
	}
	return line
}
