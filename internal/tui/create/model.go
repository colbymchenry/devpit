package create

import (
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/colbymchenry/devpit/internal/createpipeline"
	"github.com/colbymchenry/devpit/internal/tui/core"
)

const (
	fieldPrompt = iota
	fieldDefault
	fieldSubmit
)

// Model is the workflow creation form model.
type Model struct {
	promptInput textinput.Model
	useDefault  bool
	focus       int
	width       int
	height      int
	keys        core.KeyMap
	err         string
}

// New creates a new create form model.
func New() Model {
	ti := textinput.New()
	ti.Placeholder = "Describe the workflow you want to create..."
	ti.CharLimit = 500
	ti.Width = 60
	ti.Prompt = "  "
	ti.PromptStyle = lipgloss.NewStyle()
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA"))

	return Model{
		promptInput: ti,
		keys:        core.DefaultKeyMap(),
	}
}

// Focus activates the prompt input field.
func (m *Model) Focus() {
	m.promptInput.Focus()
	m.focus = fieldPrompt
}

func (m *Model) updatePlaceholder() {
	if m.useDefault {
		m.promptInput.Placeholder = "Leave blank to use defaults, or add context..."
	} else {
		m.promptInput.Placeholder = "Describe the workflow you want to create..."
	}
}

// SetSize updates the available dimensions.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	m.promptInput.Width = w - 20
	if m.promptInput.Width > 80 {
		m.promptInput.Width = 80
	}
	return m
}

// Update handles input for the create form.
func (m Model) Update(msg tea.Msg, projectDir string) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.Down):
			m.focus = (m.focus + 1) % 3
			if m.focus == fieldPrompt {
				m.promptInput.Focus()
			} else {
				m.promptInput.Blur()
			}
			return m, nil

		case key.Matches(msg, m.keys.Up):
			m.focus = (m.focus + 2) % 3 // -1 mod 3
			if m.focus == fieldPrompt {
				m.promptInput.Focus()
			} else {
				m.promptInput.Blur()
			}
			return m, nil

		case key.Matches(msg, m.keys.Enter):
			if m.focus == fieldSubmit {
				return m.submit(projectDir)
			}
			if m.focus == fieldDefault {
				m.useDefault = !m.useDefault
				m.updatePlaceholder()
				return m, nil
			}

		case msg.String() == " ":
			if m.focus == fieldDefault {
				m.useDefault = !m.useDefault
				m.updatePlaceholder()
				return m, nil
			}
		}
	}

	// Pass to text input when focused
	if m.focus == fieldPrompt {
		var cmd tea.Cmd
		m.promptInput, cmd = m.promptInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) submit(projectDir string) (Model, tea.Cmd) {
	prompt := strings.TrimSpace(m.promptInput.Value())
	if !m.useDefault && prompt == "" {
		m.err = "Enter a description or select 'Use standard template'"
		return m, nil
	}

	m.err = ""

	// Spawn the Claude session in tmux.
	_, err := createpipeline.SpawnSession(projectDir, prompt, "claude", m.useDefault)
	if err != nil {
		m.err = "Failed to start session: " + err.Error()
		return m, nil
	}

	// Attach to the tmux session using tea.Exec.
	// This suspends the TUI and gives the user the interactive Claude session.
	// When they detach (ctrl+b d) or the session ends, the TUI resumes.
	cmd := createpipeline.TmuxAttachCmd()
	if cmd == nil {
		m.err = "tmux not found"
		return m, nil
	}

	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return CreateSessionEndedMsg{Err: err}
	})
}

// CreateSessionEndedMsg is sent when the user returns from the tmux create session.
type CreateSessionEndedMsg struct {
	Err error
}

// AttachToExisting returns a tea.Cmd that attaches to an already-running create session.
func AttachToExisting() tea.Cmd {
	cmd := createpipeline.TmuxAttachCmd()
	if cmd == nil {
		return nil
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return CreateSessionEndedMsg{Err: err}
	})
}

// HasActiveSession checks if a create session is running in tmux.
func HasActiveSession() bool {
	cmd := exec.Command("tmux", "has-session", "-t", createpipeline.SessionName) //nolint:gosec
	return cmd.Run() == nil
}

// View renders the create form.
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
	lines = append(lines, core.PanelTop("Create Workflow", panelWidth, bc))
	lines = append(lines, core.PanelEmpty(panelWidth, bc))

	// Prompt input
	var promptLabel string
	if m.useDefault {
		if m.focus == fieldPrompt {
			promptLabel = focusLabel.Render("Additional context (optional)")
		} else {
			promptLabel = label.Render("Additional context (optional)")
		}
	} else {
		if m.focus == fieldPrompt {
			promptLabel = focusLabel.Render("Describe your custom workflow")
		} else {
			promptLabel = label.Render("Describe your custom workflow")
		}
	}
	lines = append(lines, core.PanelRow(promptLabel, panelWidth, bc))
	lines = append(lines, core.PanelRow(m.promptInput.View(), panelWidth, bc))

	// Underline
	inputWidth := panelWidth - 6
	if inputWidth > 80 {
		inputWidth = 80
	}
	if m.focus == fieldPrompt {
		lines = append(lines, core.PanelRow(
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurple)).Render(strings.Repeat("─", inputWidth)),
			panelWidth, bc))
	} else {
		lines = append(lines, core.PanelRow(
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Render(strings.Repeat("─", inputWidth)),
			panelWidth, bc))
	}

	lines = append(lines, core.PanelEmpty(panelWidth, bc))

	// Default toggle
	var defaultLabel string
	if m.focus == fieldDefault {
		defaultLabel = focusLabel.Render("Template")
	} else {
		defaultLabel = label.Render("Template")
	}
	lines = append(lines, core.PanelRow(defaultLabel, panelWidth, bc))

	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight))
	textStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorWhite))
	if m.useDefault {
		lines = append(lines, core.PanelRow(
			checkStyle.Render("[x] ")+textStyle.Render("Use standard template (architect, coder, tester, reviewer)"),
			panelWidth, bc))
	} else {
		lines = append(lines, core.PanelRow(
			checkStyle.Render("[ ] ")+textStyle.Render("Use standard template (architect, coder, tester, reviewer)"),
			panelWidth, bc))
	}

	lines = append(lines, core.PanelEmpty(panelWidth, bc))

	// Submit button
	if m.focus == fieldSubmit {
		btn := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1F2937")).
			Background(lipgloss.Color(core.ColorGreen)).
			Bold(true).
			Padding(0, 3)
		lines = append(lines, core.PanelRow(btn.Render("Create"), panelWidth, bc))
	} else {
		btn := lipgloss.NewStyle().
			Foreground(lipgloss.Color(core.ColorMuted))
		lines = append(lines, core.PanelRow(btn.Render("[ Create ]"), panelWidth, bc))
	}

	// Error message
	if m.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorRed))
		lines = append(lines, core.PanelEmpty(panelWidth, bc))
		lines = append(lines, core.PanelRow(errStyle.Render(m.err), panelWidth, bc))
	}

	lines = append(lines, core.PanelEmpty(panelWidth, bc))

	// Help text
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Italic(true)
	if m.useDefault {
		lines = append(lines, core.PanelRow(helpStyle.Render("Check the box and hit Create — Claude will scan your"), panelWidth, bc))
		lines = append(lines, core.PanelRow(helpStyle.Render("project and generate agents tailored to your stack."), panelWidth, bc))
	} else {
		lines = append(lines, core.PanelRow(helpStyle.Render("Describe your workflow and Claude will design a"), panelWidth, bc))
		lines = append(lines, core.PanelRow(helpStyle.Render("custom workflow with specialized agents."), panelWidth, bc))
	}
	lines = append(lines, core.PanelRow(helpStyle.Render("Detach with ctrl+b d to return here."), panelWidth, bc))

	lines = append(lines, core.PanelEmpty(panelWidth, bc))
	lines = append(lines, core.PanelBottom(panelWidth, bc))

	return strings.Join(lines, "\n")
}
