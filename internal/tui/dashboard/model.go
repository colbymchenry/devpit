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

// spinnerGlyphs mirrors Claude Code's thinking spinner sequence.
var spinnerGlyphs = []string{"·", "✢", "✳", "✶", "✻", "✽"}

// animFramesPerGlyph controls how many fast animation ticks pass before
// advancing to the next spinner glyph. At 150ms per anim tick this gives
// ~450ms per glyph — close to Claude Code's cadence.
const animFramesPerGlyph = 3

// Model is the dashboard view model.
type Model struct {
	rows      []PipelineRow
	sessions  []core.SessionInfo
	cursor    int
	animFrame int // fast counter for shimmer (increments every 150ms)
	width     int
	height    int
	keys      core.KeyMap
}

// AnimTick advances the fast animation frame (shimmer + spinner).
func (m Model) AnimTick() Model {
	m.animFrame++
	return m
}

func (m Model) spinnerGlyph() string {
	idx := (m.animFrame / animFramesPerGlyph) % len(spinnerGlyphs)
	return spinnerGlyphs[idx]
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
	m.rows = nil
	for _, r := range records {
		isLive := false
		if r.Status == pipeline.StatusRunning {
			for range m.sessions {
				isLive = true
				break
			}
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
	}

	if m.cursor >= len(m.rows) && len(m.rows) > 0 {
		m.cursor = len(m.rows) - 1
	}
	return m
}

func (m Model) rebuildRows() Model {
	for i := range m.rows {
		m.rows[i].IsLive = false
		if m.rows[i].Status == pipeline.StatusRunning {
			for range m.sessions {
				m.rows[i].IsLive = true
				break
			}
		}
	}

	// Remove ad-hoc rows (no RunID) whose session no longer exists
	var kept []PipelineRow
	for _, row := range m.rows {
		if row.RunID == "" {
			agentName := strings.TrimPrefix(row.Task, "Agent: ")
			found := false
			for _, s := range m.sessions {
				if s.Agent == agentName {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		kept = append(kept, row)
	}
	m.rows = kept

	if m.cursor >= len(m.rows) && len(m.rows) > 0 {
		m.cursor = len(m.rows) - 1
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
		case key.Matches(msg, m.keys.Delete):
			if m.cursor < len(m.rows) {
				row := m.rows[m.cursor]
				if row.RunID != "" {
					return m, func() tea.Msg {
						return core.RunDeletedMsg{RunID: row.RunID}
					}
				}
			}
		case key.Matches(msg, m.keys.Kill):
			if m.cursor < len(m.rows) {
				row := m.rows[m.cursor]
				if row.IsLive || row.Status == pipeline.StatusRunning {
					agent := row.Agent
					if agent == "" {
						// Ad-hoc session rows store the agent in the Task field
						agent = strings.TrimPrefix(row.Task, "Agent: ")
					}
					return m, func() tea.Msg {
						return core.SessionKillMsg{Agent: agent, RunID: row.RunID}
					}
				}
			}
		case key.Matches(msg, m.keys.Retry):
			if m.cursor < len(m.rows) {
				row := m.rows[m.cursor]
				if row.Status == pipeline.StatusFailed || row.Status == pipeline.StatusCancelled {
					task := row.Task
					agent := row.Agent
					if agent == "" {
						agent = "claude"
					}
					return m, func() tea.Msg {
						return core.RetryPipelineMsg{Task: task, Agent: agent}
					}
				}
			}
		case key.Matches(msg, m.keys.History):
			return m, func() tea.Msg {
				return core.NavigateMsg{View: core.ViewHistory}
			}
		case key.Matches(msg, m.keys.Create):
			return m, func() tea.Msg {
				return core.NavigateMsg{View: core.ViewCreate}
			}
		case key.Matches(msg, m.keys.Edit):
			return m, func() tea.Msg {
				return core.NavigateMsg{View: core.ViewEditWorkflow}
			}
		}
	}
	return m, nil
}

// View renders the dashboard.
func (m Model) View() string {
	panelWidth := m.width
	if panelWidth < 40 {
		panelWidth = 40
	}
	if panelWidth > 120 {
		panelWidth = 120
	}

	bc := lipgloss.Color(core.ColorBorder)

	// Count stats for the title
	title := "Runs"
	if len(m.rows) > 0 {
		running := 0
		for _, r := range m.rows {
			if r.Status == pipeline.StatusRunning {
				running++
			}
		}
		if running > 0 {
			title = fmt.Sprintf("Runs  %s",
				lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorAmber)).Render(
					fmt.Sprintf("%d running", running)))
		}
	}

	var lines []string
	lines = append(lines, core.PanelTop(title, panelWidth, bc))

	if len(m.rows) == 0 && len(m.sessions) == 0 {
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
		emptyMsg := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).
			Render("No runs yet. Press n to start one, or c to create a workflow.")
		lines = append(lines, core.PanelRow(emptyMsg, panelWidth, bc))
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
		lines = append(lines, core.PanelBottom(panelWidth, bc))
		return strings.Join(lines, "\n")
	}

	// Column widths
	colStatus := 10 // "✗ Failed" = ~8 + pad
	colAgent := 10
	colTime := 8
	colTask := panelWidth - 4 - colStatus - colAgent - colTime - 6 // 4 border, 6 gaps
	if colTask < 20 {
		colTask = 20
	}

	// Table header
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).Bold(true)
	header := fmt.Sprintf("%s%s%s%s",
		headerStyle.Render(core.PadRight("STATUS", colStatus)),
		headerStyle.Render(core.PadRight("  TASK", colTask+2)),
		headerStyle.Render(core.PadRight("AGENT", colAgent)),
		headerStyle.Render(core.PadRight("TIME", colTime)),
	)
	lines = append(lines, core.PanelEmpty(panelWidth, bc))
	lines = append(lines, core.PanelRow(header, panelWidth, bc))

	// Header underline
	dimLine := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))
	underline := fmt.Sprintf("%s%s%s%s",
		dimLine.Render(core.PadRight(strings.Repeat("─", colStatus-1), colStatus)),
		dimLine.Render(core.PadRight("  "+strings.Repeat("─", colTask-1), colTask+2)),
		dimLine.Render(core.PadRight(strings.Repeat("─", colAgent-1), colAgent)),
		dimLine.Render(core.PadRight(strings.Repeat("─", colTime-1), colTime)),
	)
	lines = append(lines, core.PanelRow(underline, panelWidth, bc))

	// Rows
	maxRows := m.height - 10
	if maxRows < 3 {
		maxRows = 3
	}
	visibleRows := m.rows
	if len(visibleRows) > maxRows {
		visibleRows = visibleRows[:maxRows]
	}

	for i, row := range visibleRows {
		isSelected := i == m.cursor

		// Status cell
		var statusCell string
		if row.Status == pipeline.StatusRunning {
			icon := m.spinnerGlyph()
			statusCell = core.ShimmerStyle(m.animFrame).Render(icon+" Running")
		} else {
			icon := core.StatusIcon(string(row.Status))
			label := core.StatusLabel(string(row.Status))
			statusCell = core.StatusStyle(string(row.Status)).Render(icon + " " + label)
		}
		statusCell = core.PadRight(statusCell, colStatus)

		// Task cell
		task := core.Truncate(row.Task, colTask-1)
		taskStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorText))
		if isSelected {
			taskStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorWhite)).Bold(true)
		}

		// Cursor indicator
		cursor := "  "
		if isSelected {
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Render("▸ ")
		}
		taskCell := cursor + core.PadRight(taskStyle.Render(task), colTask)

		// Agent cell
		agentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted))
		agentCell := core.PadRight(agentStyle.Render(row.Agent), colAgent)

		// Time cell
		timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))
		timeCell := core.PadRight(timeStyle.Render(timeAgo(row.StartedAt)), colTime)

		rowContent := statusCell + taskCell + agentCell + timeCell

		if isSelected {
			// Wrap the entire row content in a background highlight
			rowRendered := core.PanelRow(rowContent, panelWidth, bc)
			// Apply selection highlight to the inner content
			lines = append(lines, rowRendered)
		} else {
			lines = append(lines, core.PanelRow(rowContent, panelWidth, bc))
		}
	}

	// Scroll indicator
	if len(m.rows) > maxRows {
		indicator := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).
			Render(fmt.Sprintf("  ↕ %d of %d", maxRows, len(m.rows)))
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
