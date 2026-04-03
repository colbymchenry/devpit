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
	panelWidth := m.width
	if panelWidth < 40 {
		panelWidth = 40
	}
	if panelWidth > 120 {
		panelWidth = 120
	}

	bc := lipgloss.Color(core.ColorBorder)
	records := m.filtered()

	title := "History"
	if len(records) > 0 {
		title = fmt.Sprintf("History  %s",
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).
				Render(fmt.Sprintf("%d runs", len(records))))
	}

	var lines []string
	lines = append(lines, core.PanelTop(title, panelWidth, bc))

	if len(records) == 0 {
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
		emptyMsg := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).
			Render("No pipeline runs found.")
		lines = append(lines, core.PanelRow(emptyMsg, panelWidth, bc))
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
		lines = append(lines, core.PanelBottom(panelWidth, bc))
		return strings.Join(lines, "\n")
	}

	// Column widths
	colStatus := 10
	colAgent := 10
	colSteps := 8
	colTime := 8
	colTask := panelWidth - 4 - colStatus - colAgent - colSteps - colTime - 8
	if colTask < 20 {
		colTask = 20
	}

	// Table header
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).Bold(true)
	header := fmt.Sprintf("%s%s%s%s%s",
		headerStyle.Render(core.PadRight("STATUS", colStatus)),
		headerStyle.Render(core.PadRight("  TASK", colTask+2)),
		headerStyle.Render(core.PadRight("AGENT", colAgent)),
		headerStyle.Render(core.PadRight("STEPS", colSteps)),
		headerStyle.Render(core.PadRight("TIME", colTime)),
	)
	lines = append(lines, core.PanelEmpty(panelWidth, bc))
	lines = append(lines, core.PanelRow(header, panelWidth, bc))

	dimLine := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))
	underline := fmt.Sprintf("%s%s%s%s%s",
		dimLine.Render(core.PadRight(strings.Repeat("─", colStatus-1), colStatus)),
		dimLine.Render(core.PadRight("  "+strings.Repeat("─", colTask-1), colTask+2)),
		dimLine.Render(core.PadRight(strings.Repeat("─", colAgent-1), colAgent)),
		dimLine.Render(core.PadRight(strings.Repeat("─", colSteps-1), colSteps)),
		dimLine.Render(core.PadRight(strings.Repeat("─", colTime-1), colTime)),
	)
	lines = append(lines, core.PanelRow(underline, panelWidth, bc))

	// Rows
	maxRows := m.height - 10
	if maxRows < 3 {
		maxRows = 3
	}
	visible := records
	if len(visible) > maxRows {
		visible = visible[:maxRows]
	}

	for i, r := range visible {
		isSelected := i == m.cursor

		// Status
		icon := core.StatusIcon(string(r.Status))
		label := core.StatusLabel(string(r.Status))
		statusCell := core.PadRight(
			core.StatusStyle(string(r.Status)).Render(icon+" "+label),
			colStatus,
		)

		// Task
		task := core.Truncate(r.Task, colTask-1)
		taskStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorText))
		cursor := "  "
		if isSelected {
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Render("▸ ")
			taskStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorWhite)).Bold(true)
		}
		taskCell := cursor + core.PadRight(taskStyle.Render(task), colTask)

		// Agent
		agentCell := core.PadRight(
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).Render(r.Agent),
			colAgent,
		)

		// Steps
		stepsCell := core.PadRight(
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Render(fmt.Sprintf("%d", len(r.Steps))),
			colSteps,
		)

		// Time
		timeCell := core.PadRight(
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Render(timeAgo(r.StartedAt)),
			colTime,
		)

		lines = append(lines, core.PanelRow(statusCell+taskCell+agentCell+stepsCell+timeCell, panelWidth, bc))
	}

	if len(records) > maxRows {
		indicator := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).
			Render(fmt.Sprintf("  ↕ %d of %d", maxRows, len(records)))
		lines = append(lines, core.PanelRow(indicator, panelWidth, bc))
	}

	lines = append(lines, core.PanelEmpty(panelWidth, bc))
	lines = append(lines, core.PanelBottom(panelWidth, bc))

	return strings.Join(lines, "\n")
}

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
