package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/colbymchenry/devpit/internal/pipeline"
	"github.com/colbymchenry/devpit/internal/tmux"
	"github.com/colbymchenry/devpit/internal/tui/core"
	"github.com/colbymchenry/devpit/internal/tui/dashboard"
	"github.com/colbymchenry/devpit/internal/tui/detail"
	"github.com/colbymchenry/devpit/internal/tui/history"
	"github.com/colbymchenry/devpit/internal/tui/launch"
)

const tickInterval = 500 * time.Millisecond

var titleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FFFFFF")).
	Background(lipgloss.Color("#7C3AED")).
	Padding(0, 1)

var helpStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#6B7280")).
	Padding(1, 0, 0, 0)

// shared holds mutable state shared across all copies of Model.
// Bubbletea copies the model by value, so we need a pointer to
// shared state for the goroutine-spawned pipeline to call program.Send().
type shared struct {
	program *tea.Program
}

// Model is the top-level bubbletea model.
type Model struct {
	activeView core.View
	dashboard  dashboard.Model
	detail     detail.Model
	launch     launch.Model
	history    history.Model

	projectDir string
	tmux       *tmux.Tmux
	keys       core.KeyMap
	width      int
	height     int
	shared     *shared
}

// Run starts the TUI application.
func Run() error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	m := NewModel(projectDir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.shared.program = p // shared pointer — visible to bubbletea's copy of m

	_, err = p.Run()
	return err
}

// NewModel creates the top-level TUI model.
func NewModel(projectDir string) Model {
	t := tmux.NewTmux()
	return Model{
		activeView: core.ViewDashboard,
		dashboard:  dashboard.New(),
		detail:     detail.New(),
		launch:     launch.New(),
		history:    history.New(),
		projectDir: projectDir,
		tmux:       t,
		keys:       core.DefaultKeyMap(),
		shared:     &shared{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		m.refreshSessions(),
		m.loadHistory(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.dashboard = m.dashboard.SetSize(msg.Width, msg.Height-4)
		m.detail = m.detail.SetSize(msg.Width, msg.Height-4)
		m.history = m.history.SetSize(msg.Width, msg.Height-4)
		m.launch = m.launch.SetSize(msg.Width, msg.Height-4)
		return m, nil

	case tea.KeyMsg:
		// Global quit (not in launch form where q is valid input)
		if key.Matches(msg, m.keys.Quit) && m.activeView != core.ViewLaunch {
			return m, tea.Quit
		}

		// Global back
		if key.Matches(msg, m.keys.Back) {
			switch m.activeView {
			case core.ViewDetail, core.ViewLaunch, core.ViewHistory:
				m.activeView = core.ViewDashboard
				return m, nil
			case core.ViewDashboard:
				return m, tea.Quit
			}
		}

	case core.TickMsg:
		cmds = append(cmds, tickCmd())
		cmds = append(cmds, m.refreshSessions())
		if m.activeView == core.ViewDetail {
			cmds = append(cmds, m.captureDetailOutput())
		}
		return m, tea.Batch(cmds...)

	case core.SessionsUpdatedMsg:
		m.dashboard = m.dashboard.UpdateSessions(msg.Sessions)
		return m, nil

	case core.PaneOutputMsg:
		m.detail = m.detail.UpdateOutput(msg)
		return m, nil

	case core.NavigateMsg:
		m.activeView = msg.View
		switch msg.View {
		case core.ViewDetail:
			m.detail = m.detail.SetRunID(msg.RunID, m.projectDir)
			cmds = append(cmds, m.captureDetailOutput())
		case core.ViewHistory:
			cmds = append(cmds, m.loadHistory())
		case core.ViewLaunch:
			m.launch = launch.New()
			m.launch = m.launch.SetSize(m.width, m.height-4)
			m.launch.Focus()
		}
		return m, tea.Batch(cmds...)

	case core.HistoryLoadedMsg:
		m.history = m.history.SetRecords(msg.Records)
		m.dashboard = m.dashboard.UpdateHistory(msg.Records)
		return m, nil

	case core.StepStartMsg:
		m.detail = m.detail.StepStarted(msg)
		return m, nil

	case core.StepDoneMsg:
		m.detail = m.detail.StepDone(msg)
		return m, nil

	case core.PipelineFinishedMsg:
		cmds = append(cmds, m.loadHistory())
		return m, tea.Batch(cmds...)

	case core.PipelineStartedMsg:
		m.activeView = core.ViewDetail
		m.detail = m.detail.SetRunID(msg.Record.ID, m.projectDir)
		cmds = append(cmds, m.loadHistory())
		return m, tea.Batch(cmds...)
	}

	// Delegate to active view
	switch m.activeView {
	case core.ViewDashboard:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case core.ViewDetail:
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case core.ViewLaunch:
		var cmd tea.Cmd
		m.launch, cmd = m.launch.Update(msg, m.projectDir, m.shared.program)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case core.ViewHistory:
		var cmd tea.Cmd
		m.history, cmd = m.history.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	var content string

	title := titleStyle.Render(" DevPit ")

	switch m.activeView {
	case core.ViewDashboard:
		content = m.dashboard.View()
	case core.ViewDetail:
		content = m.detail.View()
	case core.ViewLaunch:
		content = m.launch.View()
	case core.ViewHistory:
		content = m.history.View()
	}

	help := m.helpView()

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		content,
		help,
	)
}

func (m Model) helpView() string {
	var keys []string
	switch m.activeView {
	case core.ViewDashboard:
		keys = []string{"n:new", "h:history", "enter:view", "q:quit"}
	case core.ViewDetail:
		keys = []string{"enter:view output", "esc:back", "q:quit"}
	case core.ViewLaunch:
		keys = []string{"tab:next field", "enter:submit", "esc:cancel"}
	case core.ViewHistory:
		keys = []string{"enter:view", "esc:back"}
	}
	return helpStyle.Render(strings.Join(keys, "  |  "))
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return core.TickMsg(t)
	})
}

func (m Model) refreshSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.tmux.ListSessions()
		if err != nil {
			return core.SessionsUpdatedMsg{}
		}

		var infos []core.SessionInfo
		for _, s := range sessions {
			if !strings.HasPrefix(s, pipeline.SessionPrefix) {
				continue
			}
			agent := strings.TrimPrefix(s, pipeline.SessionPrefix)
			idle := m.tmux.IsIdle(s)

			var lastLine string
			lines, err := m.tmux.CapturePaneLines(s, 3)
			if err == nil && len(lines) > 0 {
				for i := len(lines) - 1; i >= 0; i-- {
					trimmed := strings.TrimSpace(lines[i])
					if trimmed != "" {
						lastLine = trimmed
						break
					}
				}
			}

			infos = append(infos, core.SessionInfo{
				Agent:    agent,
				IsIdle:   idle,
				LastLine: lastLine,
			})
		}

		return core.SessionsUpdatedMsg{Sessions: infos}
	}
}

func (m Model) captureDetailOutput() tea.Cmd {
	agent := m.detail.SelectedAgent()
	if agent == "" {
		return nil
	}
	session := pipeline.SessionPrefix + agent
	return func() tea.Msg {
		output, err := m.tmux.CapturePane(session, 200)
		if err != nil {
			return core.PaneOutputMsg{Agent: agent}
		}
		idle := m.tmux.IsIdle(session)
		return core.PaneOutputMsg{Agent: agent, Output: output, IsIdle: idle}
	}
}

func (m Model) loadHistory() tea.Cmd {
	return func() tea.Msg {
		records, _ := pipeline.ListRunRecords(m.projectDir)
		return core.HistoryLoadedMsg{Records: records}
	}
}
