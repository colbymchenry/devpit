package history

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/colbymchenry/devpit/internal/pipeline"
	"github.com/colbymchenry/devpit/internal/tui/core"
)

// Model is the history browser model.
type Model struct {
	records []*pipeline.RunRecord
	cursor  int
	filter  string
	width   int
	height  int
	keys    core.KeyMap
}

// New creates a new history model.
func New() Model {
	return Model{
		keys: core.DefaultKeyMap(),
	}
}

// SetSize updates the available dimensions.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

// SetRecords updates the history records.
func (m Model) SetRecords(records []*pipeline.RunRecord) Model {
	m.records = records
	if m.cursor >= len(m.records) && len(m.records) > 0 {
		m.cursor = len(m.records) - 1
	}
	return m
}

func (m Model) filtered() []*pipeline.RunRecord {
	if m.filter == "" {
		return m.records
	}
	var out []*pipeline.RunRecord
	lower := strings.ToLower(m.filter)
	for _, r := range m.records {
		if strings.Contains(strings.ToLower(r.Task), lower) {
			out = append(out, r)
		}
	}
	return out
}

// Update handles input for the history view.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			records := m.filtered()
			if m.cursor < len(records)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Enter):
			records := m.filtered()
			if m.cursor < len(records) {
				r := records[m.cursor]
				return m, func() tea.Msg {
					return core.NavigateMsg{View: core.ViewDetail, RunID: r.ID}
				}
			}
		}
	}
	return m, nil
}

// View renders the history browser.
func (m Model) View() string {
	var b strings.Builder

	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)
	b.WriteString(header.Render("Pipeline History") + "\n\n")

	records := m.filtered()

	if len(records) == 0 {
		muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		b.WriteString(muted.Render("  No pipeline runs found.") + "\n")
		return b.String()
	}

	maxRows := m.height - 4
	if maxRows < 1 {
		maxRows = 10
	}
	visible := records
	if len(visible) > maxRows {
		visible = visible[:maxRows]
	}

	for i, r := range visible {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
		if i == m.cursor {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
		}

		icon := core.StatusIcon(string(r.Status))
		iconStyle := core.StatusStyle(string(r.Status))

		task := r.Task
		if len(task) > 50 {
			task = task[:47] + "..."
		}

		steps := fmt.Sprintf("%d steps", len(r.Steps))
		ago := timeAgo(r.StartedAt)

		line := fmt.Sprintf("%s%s %s  %s  %s  %s",
			cursor,
			iconStyle.Render(icon),
			style.Render(task),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(r.Agent),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(steps),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(ago),
		)
		b.WriteString(line + "\n")
	}

	return b.String()
}

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
