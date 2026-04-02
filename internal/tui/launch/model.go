package launch

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/colbymchenry/devpit/internal/config"
	"github.com/colbymchenry/devpit/internal/pipeline"
	"github.com/colbymchenry/devpit/internal/tui/core"
)

const (
	fieldTask = iota
	fieldAgent
	fieldSubmit
)

// Model is the pipeline launch form model.
type Model struct {
	taskInput  textinput.Model
	agent      string
	agents     []string
	agentIdx   int
	skipReview bool
	skipQA     bool
	focus      int
	width      int
	height     int
	keys       core.KeyMap
	err        string
}

// New creates a new launch form model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "Describe the task..."
	ti.CharLimit = 500
	ti.Width = 60

	agents := config.ListAgentPresets()
	// Ensure claude is first
	for i, a := range agents {
		if a == "claude" && i != 0 {
			agents[0], agents[i] = agents[i], agents[0]
			break
		}
	}

	return Model{
		taskInput: ti,
		agent:     "claude",
		agents:    agents,
		keys:      core.DefaultKeyMap(),
	}
}

// Focus activates the task input field.
func (m *Model) Focus() {
	m.taskInput.Focus()
	m.focus = fieldTask
}

// SetSize updates the available dimensions.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	m.taskInput.Width = w - 20
	if m.taskInput.Width > 80 {
		m.taskInput.Width = 80
	}
	return m
}

// Update handles input for the launch form.
func (m Model) Update(msg tea.Msg, projectDir string, program *tea.Program) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Tab):
			m.focus = (m.focus + 1) % 3
			if m.focus == fieldTask {
				m.taskInput.Focus()
			} else {
				m.taskInput.Blur()
			}
			return m, nil

		case key.Matches(msg, m.keys.Enter):
			if m.focus == fieldSubmit {
				return m.submit(projectDir, program)
			}
			if m.focus == fieldAgent {
				m.agentIdx = (m.agentIdx + 1) % len(m.agents)
				m.agent = m.agents[m.agentIdx]
				return m, nil
			}

		case msg.String() == "left" || msg.String() == "right":
			if m.focus == fieldAgent {
				if msg.String() == "right" {
					m.agentIdx = (m.agentIdx + 1) % len(m.agents)
				} else {
					m.agentIdx = (m.agentIdx - 1 + len(m.agents)) % len(m.agents)
				}
				m.agent = m.agents[m.agentIdx]
				return m, nil
			}
		}
	}

	// Pass to text input when focused
	if m.focus == fieldTask {
		var cmd tea.Cmd
		m.taskInput, cmd = m.taskInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) submit(projectDir string, program *tea.Program) (Model, tea.Cmd) {
	task := strings.TrimSpace(m.taskInput.Value())
	if task == "" {
		m.err = "Task cannot be empty"
		return m, nil
	}

	m.err = ""

	// Check for agent files
	steps := pipeline.DiscoverPipeline(projectDir)
	if len(steps) == 0 {
		m.err = "No agent files found. Run 'dp setup-agents' first."
		return m, nil
	}

	// Create run record
	record := pipeline.NewRunRecord(task, m.agent)
	if err := pipeline.SaveRunRecord(projectDir, record); err != nil {
		m.err = "Failed to save run record"
		return m, nil
	}

	// Launch pipeline in goroutine
	go func() {
		result, err := pipeline.Run(pipeline.PipelineOpts{
			Task:        task,
			ProjectDir:  projectDir,
			AgentPreset: m.agent,
			OnStepStart: func(step string, attempt int) {
				program.Send(core.StepStartMsg{
					RunID:   record.ID,
					Step:    step,
					Attempt: attempt,
				})
			},
			OnStepDone: func(step string, passed bool, output string) {
				program.Send(core.StepDoneMsg{
					RunID:  record.ID,
					Step:   step,
					Passed: passed,
					Output: output,
				})
			},
		})

		// Update record with final status
		loaded, loadErr := pipeline.LoadRunRecord(projectDir, record.ID)
		if loadErr == nil {
			record = loaded
		}
		if err != nil {
			record.Status = pipeline.StatusFailed
		} else {
			allPassed := true
			if result != nil {
				for _, s := range result.Steps {
					if !s.Passed && !s.Skipped {
						allPassed = false
						break
					}
				}
			}
			if allPassed {
				record.Status = pipeline.StatusPassed
			} else {
				record.Status = pipeline.StatusFailed
			}
		}
		now := time.Now()
		record.EndedAt = &now
		_ = pipeline.SaveRunRecord(projectDir, record)
		program.Send(core.PipelineFinishedMsg{
			RunID:  record.ID,
			Result: result,
			Err:    err,
		})
	}()

	// Reset form
	m.taskInput.SetValue("")

	return m, func() tea.Msg {
		return core.PipelineStartedMsg{Record: record}
	}
}

// View renders the launch form.
func (m Model) View() string {
	var b strings.Builder

	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7C3AED")).
		Bold(true)
	b.WriteString(header.Render("New Pipeline") + "\n\n")

	// Task input
	label := lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
	focusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)

	if m.focus == fieldTask {
		b.WriteString(focusLabel.Render("Task:") + "\n")
	} else {
		b.WriteString(label.Render("Task:") + "\n")
	}
	b.WriteString("  " + m.taskInput.View() + "\n\n")

	// Agent selector
	if m.focus == fieldAgent {
		b.WriteString(focusLabel.Render("Agent:") + "  ")
	} else {
		b.WriteString(label.Render("Agent:") + "  ")
	}
	agentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#7C3AED")).
		Padding(0, 1)
	b.WriteString(agentStyle.Render(m.agent))
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(muted.Render("  (left/right to change)") + "\n\n")

	// Submit button
	if m.focus == fieldSubmit {
		btn := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#22C55E")).
			Bold(true).
			Padding(0, 2)
		b.WriteString("  " + btn.Render("Start Pipeline") + "\n")
	} else {
		btn := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280")).
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#6B7280")).
			Padding(0, 2)
		b.WriteString("  " + btn.Render("Start Pipeline") + "\n")
	}

	// Error message
	if m.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
		b.WriteString("\n" + errStyle.Render(m.err) + "\n")
	}

	return b.String()
}
