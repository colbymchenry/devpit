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
	ti.Prompt = "  "
	ti.PromptStyle = lipgloss.NewStyle()
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA"))

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
	panelWidth := m.width
	if panelWidth < 40 {
		panelWidth = 40
	}
	if panelWidth > 80 {
		panelWidth = 80
	}

	bc := lipgloss.Color(core.ColorBorder)
	label := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted))
	focusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Bold(true)

	var lines []string
	lines = append(lines, core.PanelTop("New Pipeline", panelWidth, bc))
	lines = append(lines, core.PanelEmpty(panelWidth, bc))

	// Task input
	var taskLabel string
	if m.focus == fieldTask {
		taskLabel = focusLabel.Render("Task")
	} else {
		taskLabel = label.Render("Task")
	}
	lines = append(lines, core.PanelRow(taskLabel, panelWidth, bc))
	lines = append(lines, core.PanelRow(m.taskInput.View(), panelWidth, bc))

	// Underline
	inputWidth := panelWidth - 6
	if inputWidth > 80 {
		inputWidth = 80
	}
	if m.focus == fieldTask {
		lines = append(lines, core.PanelRow(
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurple)).Render(strings.Repeat("─", inputWidth)),
			panelWidth, bc))
	} else {
		lines = append(lines, core.PanelRow(
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Render(strings.Repeat("─", inputWidth)),
			panelWidth, bc))
	}

	lines = append(lines, core.PanelEmpty(panelWidth, bc))

	// Agent selector
	var agentLabel string
	if m.focus == fieldAgent {
		agentLabel = focusLabel.Render("Agent")
	} else {
		agentLabel = label.Render("Agent")
	}
	lines = append(lines, core.PanelRow(agentLabel, panelWidth, bc))

	arrowColor := core.ColorDim
	if m.focus == fieldAgent {
		arrowColor = core.ColorPurpleLight
	}
	arrowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(arrowColor))
	agentBadge := lipgloss.NewStyle().
		Foreground(lipgloss.Color(core.ColorWhite)).
		Background(lipgloss.Color(core.ColorPurple)).
		Padding(0, 1).
		Bold(true)

	agentRow := arrowStyle.Render("◀ ") + agentBadge.Render(m.agent) + arrowStyle.Render(" ▶")
	lines = append(lines, core.PanelRow(agentRow, panelWidth, bc))

	lines = append(lines, core.PanelEmpty(panelWidth, bc))

	// Submit button
	if m.focus == fieldSubmit {
		btn := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1F2937")).
			Background(lipgloss.Color(core.ColorGreen)).
			Bold(true).
			Padding(0, 3)
		lines = append(lines, core.PanelRow(btn.Render("Start Pipeline"), panelWidth, bc))
	} else {
		btn := lipgloss.NewStyle().
			Foreground(lipgloss.Color(core.ColorMuted))
		lines = append(lines, core.PanelRow(btn.Render("[ Start Pipeline ]"), panelWidth, bc))
	}

	// Error message
	if m.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorRed))
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
		lines = append(lines, core.PanelRow(errStyle.Render(m.err), panelWidth, bc))
	}

	lines = append(lines, core.PanelEmpty(panelWidth, bc))
	lines = append(lines, core.PanelBottom(panelWidth, bc))

	return strings.Join(lines, "\n")
}
