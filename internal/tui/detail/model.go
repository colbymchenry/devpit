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
	m.viewport.Width = w
	m.viewport.Height = h - 6
	return m
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
	m.steps = nil
	for _, s := range record.Steps {
		m.steps = append(m.steps, StepRow{
			Name:    s.Name,
			Status:  s.Status,
			Attempt: s.Attempt,
		})
	}

	// If no steps yet (just started), show default pipeline steps
	if len(m.steps) == 0 {
		for _, name := range []string{"architect", "coder", "tester", "reviewer", "design-qa"} {
			m.steps = append(m.steps, StepRow{
				Name:   name,
				Status: pipeline.StatusPending,
			})
		}
	}

	return m
}

// SelectedAgent returns the currently selected agent name.
func (m Model) SelectedAgent() string {
	if m.cursor < len(m.steps) {
		return m.steps[m.cursor].Name
	}
	return ""
}

// UpdateOutput sets the live tmux output for the selected agent.
func (m Model) UpdateOutput(msg core.PaneOutputMsg) Model {
	if m.showOutput && msg.Agent == m.SelectedAgent() {
		// Don't overwrite existing output with empty content —
		// the session may have been killed after the step completed.
		if strings.TrimSpace(msg.Output) == "" {
			m.outputIdle = true
			return m
		}
		m.output = msg.Output
		m.outputIdle = msg.IsIdle
		m.viewport.SetContent(msg.Output)
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
	// Step not in list yet, add it
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
			// Output viewport mode
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

		// Step list mode
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.steps)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Enter):
			m.showOutput = true
			m.viewport = viewport.New(m.width, m.height-6)
			m.viewport.SetContent(m.output)
			m.viewport.GotoBottom()
			return m, nil
		}
	}
	return m, nil
}

// View renders the detail view.
func (m Model) View() string {
	var b strings.Builder

	// Header
	task := ""
	status := ""
	if m.run != nil {
		task = m.run.Task
		status = string(m.run.Status)
	}

	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)

	if task != "" {
		b.WriteString(header.Render(task) + "\n")
		b.WriteString(core.StatusStyle(status).Render(status) + "\n\n")
	} else {
		b.WriteString(header.Render("Pipeline Detail") + "\n\n")
	}

	if m.showOutput {
		// Show output viewport
		agent := m.SelectedAgent()
		label := fmt.Sprintf("Output: %s", agent)
		if m.outputIdle {
			label += " (idle)"
		} else {
			label += " (running)"
		}
		b.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A78BFA")).
			Bold(true).
			Render(label) + "\n\n")
		if strings.TrimSpace(m.output) == "" {
			b.WriteString(lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280")).
				Render("  Waiting for output..."))
		} else {
			b.WriteString(m.viewport.View())
		}
		return b.String()
	}

	// Step list
	for i, step := range m.steps {
		cursor := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
		if i == m.cursor {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
		}

		icon := core.StatusIcon(string(step.Status))
		iconStyle := core.StatusStyle(string(step.Status))

		attempt := ""
		if step.Attempt > 1 {
			attempt = fmt.Sprintf(" (attempt %d)", step.Attempt)
		}

		line := fmt.Sprintf("%s%s %s%s",
			cursor,
			iconStyle.Render(icon),
			style.Render(step.Name),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(attempt),
		)
		b.WriteString(line + "\n")
	}

	return b.String()
}
