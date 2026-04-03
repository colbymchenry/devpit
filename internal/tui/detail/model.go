package detail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/colbymchenry/devpit/internal/pipeline"
	"github.com/colbymchenry/devpit/internal/tui/core"
)

// StepRow represents a step in the detail view.
type StepRow struct {
	Name    string
	Status  pipeline.RunStatus
	Attempt int
}

// spinnerGlyphs mirrors Claude Code's thinking spinner sequence.
var spinnerGlyphs = []string{"·", "✢", "✳", "✶", "✻", "✽"}

const animFramesPerGlyph = 3

// Model is the pipeline detail view model.
type Model struct {
	runID      string
	run        *pipeline.RunRecord
	steps      []StepRow
	cursor     int
	showOutput bool
	viewport   viewport.Model
	output     string
	outputIdle bool
	animFrame  int
	width      int
	height     int
	keys       core.KeyMap
}

// New creates a new detail model.
func New() Model {
	return Model{
		keys: core.DefaultKeyMap(),
	}
}

// SetSize updates the available dimensions.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	m.viewport.Width = w - 4 // account for panel borders
	m.viewport.Height = h - 10
	return m
}

// canRetry returns true when the pipeline is in a retryable state.
func (m Model) canRetry() bool {
	return m.run != nil &&
		(m.run.Status == pipeline.StatusFailed || m.run.Status == pipeline.StatusCancelled)
}

// SetRunID loads a run record for display.
func (m Model) SetRunID(runID, projectDir string) Model {
	m.runID = runID
	m.cursor = 0
	m.showOutput = false
	m.output = ""

	record, err := pipeline.LoadRunRecord(projectDir, runID)
	if err != nil {
		m.run = nil
		m.steps = nil
		return m
	}

	m.run = record

	// Build a map of step status from the run record.
	stepMap := make(map[string]StepRow, len(record.Steps))
	for _, s := range record.Steps {
		stepMap[s.Name] = StepRow{
			Name:    s.Name,
			Status:  s.Status,
			Attempt: s.Attempt,
		}
	}

	// Always show the full pipeline — discover steps from project config,
	// merging actual status where available and defaulting to Pending.
	allSteps := pipeline.DiscoverPipeline(projectDir)
	if len(allSteps) == 0 {
		// Fallback if no agent files exist yet
		allSteps = append(pipeline.CoreAgents, pipeline.VisualQAAgent)
	}
	m.steps = nil
	for _, name := range allSteps {
		if row, ok := stepMap[name]; ok {
			m.steps = append(m.steps, row)
			delete(stepMap, name)
		} else {
			m.steps = append(m.steps, StepRow{
				Name:   name,
				Status: pipeline.StatusPending,
			})
		}
	}
	// Append any steps from the record not in the discovered pipeline
	for _, row := range stepMap {
		m.steps = append(m.steps, row)
	}

	// Auto-focus retry button for failed/cancelled pipelines
	if m.canRetry() {
		m.cursor = -1
	}

	return m
}

// RefreshRun reloads the run record from disk without resetting cursor or
// output state. Use this when the pipeline finishes to update the status
// badge and step states.
func (m Model) RefreshRun(projectDir string) Model {
	if m.runID == "" {
		return m
	}
	record, err := pipeline.LoadRunRecord(projectDir, m.runID)
	if err != nil {
		return m
	}
	m.run = record

	// Update step statuses from the record
	stepMap := make(map[string]pipeline.StepRecord, len(record.Steps))
	for _, s := range record.Steps {
		stepMap[s.Name] = s
	}
	for i := range m.steps {
		if rec, ok := stepMap[m.steps[i].Name]; ok {
			m.steps[i].Status = rec.Status
			m.steps[i].Attempt = rec.Attempt
		}
	}

	// Focus retry button for failed/cancelled pipelines
	if m.canRetry() && m.cursor >= 0 {
		m.cursor = -1
	}

	return m
}

// ShowingOutput returns true when the detail view is displaying an agent's output viewport.
func (m Model) ShowingOutput() bool {
	return m.showOutput
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

// SelectedAgent returns the currently selected agent name.
func (m Model) SelectedAgent() string {
	if m.cursor >= 0 && m.cursor < len(m.steps) {
		return m.steps[m.cursor].Name
	}
	return ""
}

// UpdateOutput sets the live tmux output for the selected agent.
func (m Model) UpdateOutput(msg core.PaneOutputMsg) Model {
	if m.showOutput && msg.Agent == m.SelectedAgent() {
		if strings.TrimSpace(msg.Output) == "" {
			m.outputIdle = true
			return m
		}
		m.output = msg.Output
		m.outputIdle = msg.IsIdle
		// colorizeOutput passes through ANSI-colored live captures and
		// adds lipgloss styling to plain-text artifact fallbacks.
		m.viewport.SetContent(colorizeOutput(msg.Output))
		m.viewport.GotoBottom()
	}
	return m
}

// StepStarted marks a step as running.
func (m Model) StepStarted(msg core.StepStartMsg) Model {
	if msg.RunID != m.runID {
		return m
	}
	for i := range m.steps {
		if m.steps[i].Name == msg.Step {
			m.steps[i].Status = pipeline.StatusRunning
			m.steps[i].Attempt = msg.Attempt
			return m
		}
	}
	m.steps = append(m.steps, StepRow{
		Name:    msg.Step,
		Status:  pipeline.StatusRunning,
		Attempt: msg.Attempt,
	})
	return m
}

// StepDone marks a step as completed.
func (m Model) StepDone(msg core.StepDoneMsg) Model {
	if msg.RunID != m.runID {
		return m
	}
	for i := range m.steps {
		if m.steps[i].Name == msg.Step {
			if msg.Passed {
				m.steps[i].Status = pipeline.StatusPassed
			} else {
				m.steps[i].Status = pipeline.StatusFailed
			}
			return m
		}
	}
	return m
}

// Update handles input for the detail view.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.showOutput {
			switch {
			case key.Matches(msg, m.keys.Back):
				m.showOutput = false
				return m, nil
			default:
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
		}

		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			} else if m.cursor == 0 && m.canRetry() {
				m.cursor = -1 // focus retry button
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor == -1 {
				m.cursor = 0
			} else if m.cursor < len(m.steps)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Enter):
			if m.cursor == -1 && m.canRetry() {
				return m, m.retryCmd()
			}
			m.showOutput = true
			m.viewport = viewport.New(m.width-4, m.height-10)
			m.viewport.SetContent(colorizeOutput(m.output))
			m.viewport.GotoBottom()
			return m, nil
		case key.Matches(msg, m.keys.Retry):
			if m.canRetry() {
				return m, m.retryCmd()
			}
		}
	}
	return m, nil
}

func (m Model) retryCmd() tea.Cmd {
	task := m.run.Task
	agent := m.run.Agent
	if agent == "" {
		agent = "claude"
	}
	return func() tea.Msg {
		return core.RetryPipelineMsg{Task: task, Agent: agent}
	}
}

// View renders the detail view.
func (m Model) View() string {
	panelWidth := m.width
	if panelWidth < 40 {
		panelWidth = 40
	}
	if panelWidth > 120 {
		panelWidth = 120
	}

	bc := lipgloss.Color(core.ColorBorder)

	if m.showOutput {
		return m.viewOutput(panelWidth, bc)
	}
	return m.viewSteps(panelWidth, bc)
}

func (m Model) viewSteps(panelWidth int, bc lipgloss.Color) string {
	// Build title from run info
	task := "Pipeline Detail"
	var statusBadge string
	if m.run != nil {
		task = m.run.Task
		if len(task) > panelWidth-30 {
			task = core.Truncate(task, panelWidth-30)
		}
		status := string(m.run.Status)
		if status == "running" {
			statusBadge = core.ShimmerStyle(m.animFrame).Render("● " + core.StatusLabel(status))
		} else {
			statusBadge = core.StatusStyle(status).Render(core.StatusIcon(status) + " " + core.StatusLabel(status))
		}
	}

	var lines []string
	lines = append(lines, core.PanelTop(task, panelWidth, bc))
	if statusBadge != "" {
		lines = append(lines, core.PanelRow(statusBadge, panelWidth, bc))
	}

	// Retry button for failed/cancelled pipelines
	if m.canRetry() {
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
		if m.cursor == -1 {
			btn := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1F2937")).
				Background(lipgloss.Color(core.ColorPurple)).
				Bold(true).
				Padding(0, 3)
			lines = append(lines, core.PanelRow(btn.Render("↻ Retry Pipeline"), panelWidth, bc))
		} else {
			btn := lipgloss.NewStyle().
				Foreground(lipgloss.Color(core.ColorMuted)).
				Padding(0, 0)
			lines = append(lines, core.PanelRow(btn.Render("  [ ↻ Retry Pipeline ]"), panelWidth, bc))
		}
	}

	lines = append(lines, core.PanelEmpty(panelWidth, bc))

	// Progress bar
	if len(m.steps) > 0 {
		progBar := m.renderProgressBar(panelWidth - 6)
		lines = append(lines, core.PanelRow(progBar, panelWidth, bc))
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
	}

	// Step header
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).Bold(true)
	colStep := 20
	colStatus := 12
	colAttempt := 10
	header := fmt.Sprintf("%s%s%s",
		headerStyle.Render(core.PadRight("  STEP", colStep)),
		headerStyle.Render(core.PadRight("STATUS", colStatus)),
		headerStyle.Render(core.PadRight("ATTEMPT", colAttempt)),
	)
	lines = append(lines, core.PanelRow(header, panelWidth, bc))

	dimLine := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))
	underline := fmt.Sprintf("%s%s%s",
		dimLine.Render(core.PadRight("  "+strings.Repeat("─", colStep-3), colStep)),
		dimLine.Render(core.PadRight(strings.Repeat("─", colStatus-1), colStatus)),
		dimLine.Render(core.PadRight(strings.Repeat("─", colAttempt-1), colAttempt)),
	)
	lines = append(lines, core.PanelRow(underline, panelWidth, bc))

	// Step rows
	for i, step := range m.steps {
		isSelected := i == m.cursor

		// Cursor + name
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorText))
		cursor := "  "
		if isSelected {
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Render("▸ ")
			nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorWhite)).Bold(true)
		}
		nameCell := core.PadRight(cursor+nameStyle.Render(step.Name), colStep)

		// Status
		var statusCell string
		if step.Status == pipeline.StatusRunning {
			icon := m.spinnerGlyph()
			statusCell = core.ShimmerStyle(m.animFrame).Render(icon + " Running")
		} else {
			icon := core.StatusIcon(string(step.Status))
			label := core.StatusLabel(string(step.Status))
			statusCell = core.StatusStyle(string(step.Status)).Render(icon + " " + label)
		}
		statusCell = core.PadRight(statusCell, colStatus)

		// Attempt
		attemptStr := ""
		if step.Attempt > 0 {
			attemptStr = fmt.Sprintf("#%d", step.Attempt)
		}
		attemptCell := core.PadRight(
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Render(attemptStr),
			colAttempt,
		)

		lines = append(lines, core.PanelRow(nameCell+statusCell+attemptCell, panelWidth, bc))
	}

	lines = append(lines, core.PanelEmpty(panelWidth, bc))
	lines = append(lines, core.PanelBottom(panelWidth, bc))

	return strings.Join(lines, "\n")
}

func (m Model) renderProgressBar(width int) string {
	if len(m.steps) == 0 || width < 10 {
		return ""
	}

	segWidth := width / len(m.steps)
	if segWidth < 3 {
		segWidth = 3
	}

	var parts []string
	for _, step := range m.steps {
		var style lipgloss.Style
		char := "─"
		switch step.Status {
		case pipeline.StatusPassed:
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorGreen))
			char = "━"
		case pipeline.StatusFailed:
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorRed))
			char = "━"
		case pipeline.StatusRunning:
			style = core.ShimmerStyle(m.animFrame)
			char = "━"
		default:
			style = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))
		}
		parts = append(parts, style.Render(strings.Repeat(char, segWidth)))
	}

	return strings.Join(parts, "")
}

func (m Model) viewOutput(panelWidth int, bc lipgloss.Color) string {
	agent := m.SelectedAgent()
	title := fmt.Sprintf("Output: %s", agent)

	var statusIndicator string
	if !m.outputIdle {
		statusIndicator = core.ShimmerStyle(m.animFrame).Render("● live")
	} else {
		statusIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).Render("● idle")
	}

	var lines []string
	lines = append(lines, core.PanelTop(title, panelWidth, bc))
	lines = append(lines, core.PanelRow(statusIndicator, panelWidth, bc))
	lines = append(lines, core.PanelSeparator(panelWidth, bc))

	if strings.TrimSpace(m.output) == "" {
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
		waiting := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).
			Render("Waiting for output...")
		lines = append(lines, core.PanelRow(waiting, panelWidth, bc))
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
	} else {
		// Viewport content — render inside the panel
		vpContent := m.viewport.View()
		for _, line := range strings.Split(vpContent, "\n") {
			lines = append(lines, core.PanelRow(line, panelWidth, bc))
		}
	}

	lines = append(lines, core.PanelBottom(panelWidth, bc))
	return strings.Join(lines, "\n")
}
