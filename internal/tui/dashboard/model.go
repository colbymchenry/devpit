package dashboard

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

// PipelineRow represents a pipeline in the dashboard list.
type PipelineRow struct {
	RunID     string
	Task      string
	Status    pipeline.RunStatus
	Agent     string
	StartedAt time.Time
	IsLive    bool // has an active tmux session
}

// Model is the dashboard view model.
type Model struct {
	rows      []PipelineRow
	sessions  []core.SessionInfo
	cursor    int
	width     int
	height    int
	keys      core.KeyMap
}

// New creates a new dashboard model.
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

// UpdateSessions refreshes the live tmux session data.
func (m Model) UpdateSessions(sessions []core.SessionInfo) Model {
	m.sessions = sessions
	m = m.rebuildRows()
	return m
}

// UpdateHistory merges history records into the dashboard.
func (m Model) UpdateHistory(records []*pipeline.RunRecord) Model {
	// Rebuild rows from history + live sessions
	m.rows = nil
	for _, r := range records {
		isLive := false
		for _, s := range m.sessions {
			if r.Status == pipeline.StatusRunning {
				isLive = true
				break
			}
			_ = s
		}
		m.rows = append(m.rows, PipelineRow{
			RunID:     r.ID,
			Task:      r.Task,
			Status:    r.Status,
			Agent:     r.Agent,
			StartedAt: r.StartedAt,
			IsLive:    isLive,
		})
	}

	// Also add live sessions that don't have history records (ad-hoc agents)
	for _, s := range m.sessions {
		found := false
		for _, row := range m.rows {
			if row.IsLive {
				found = true
				break
			}
		}
		if !found {
			m.rows = append([]PipelineRow{{
				Task:   fmt.Sprintf("Agent: %s", s.Agent),
				Status: pipeline.StatusRunning,
				IsLive: true,
			}}, m.rows...)
		}
		_ = s
	}

	if m.cursor >= len(m.rows) && len(m.rows) > 0 {
		m.cursor = len(m.rows) - 1
	}
	return m
}

func (m Model) rebuildRows() Model {
	// Update live status of existing rows
	for i := range m.rows {
		m.rows[i].IsLive = false
		if m.rows[i].Status == pipeline.StatusRunning {
			for _, s := range m.sessions {
				_ = s
				m.rows[i].IsLive = true
				break
			}
		}
	}
	return m
}

// Update handles input for the dashboard view.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.rows)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Enter):
			if m.cursor < len(m.rows) {
				row := m.rows[m.cursor]
				return m, func() tea.Msg {
					return core.NavigateMsg{View: core.ViewDetail, RunID: row.RunID}
				}
			}
		case key.Matches(msg, m.keys.New):
			return m, func() tea.Msg {
				return core.NavigateMsg{View: core.ViewLaunch}
			}
		case key.Matches(msg, m.keys.History):
			return m, func() tea.Msg {
				return core.NavigateMsg{View: core.ViewHistory}
			}
		}
	}
	return m, nil
}

// View renders the dashboard.
func (m Model) View() string {
	var b strings.Builder

	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true).
		Render("Pipelines")
	b.WriteString(header + "\n\n")

	if len(m.rows) == 0 && len(m.sessions) == 0 {
		muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		b.WriteString(muted.Render("  No pipelines yet. Press n to start one.") + "\n")
		return b.String()
	}

	// Render pipeline rows
	maxRows := m.height - 4
	if maxRows < 1 {
		maxRows = 10
	}
	visibleRows := m.rows
	if len(visibleRows) > maxRows {
		visibleRows = visibleRows[:maxRows]
	}

	for i, row := range visibleRows {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
		if i == m.cursor {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
		}

		icon := core.StatusIcon(string(row.Status))
		iconStyle := core.StatusStyle(string(row.Status))

		task := row.Task
		if len(task) > 60 {
			task = task[:57] + "..."
		}

		ago := timeAgo(row.StartedAt)

		line := fmt.Sprintf("%s%s %s  %s  %s",
			cursor,
			iconStyle.Render(icon),
			style.Render(task),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(row.Agent),
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
