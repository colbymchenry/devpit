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
	"github.com/colbymchenry/devpit/internal/tui/create"
	"github.com/colbymchenry/devpit/internal/tui/dashboard"
	"github.com/colbymchenry/devpit/internal/tui/detail"
	"github.com/colbymchenry/devpit/internal/tui/edit"
	"github.com/colbymchenry/devpit/internal/tui/history"
	"github.com/colbymchenry/devpit/internal/tui/launch"
)

const tickInterval = 500 * time.Millisecond
const animInterval = 150 * time.Millisecond

var titleBadge = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color(core.ColorWhite)).
	Background(lipgloss.Color(core.ColorPurple)).
	Padding(0, 1)

var helpStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(core.ColorMuted)).
	Padding(0, 0, 0, 0)

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
	create     create.Model
	edit       edit.Model

	projectDir string
	tmux       *tmux.Tmux
	keys       core.KeyMap
	width      int
	height     int
	shared     *shared
}

// Run starts the TUI application at the dashboard view.
func Run() error {
	return RunAtView("")
}

// RunAtView starts the TUI application at a specific view.
// Pass "" for the default dashboard view.
func RunAtView(view string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	m := NewModel(projectDir)

	switch view {
	case "create":
		m.activeView = core.ViewCreate
		m.create = create.New()
		m.create.Focus()
	}

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
		launch:     launch.NewWithProject(projectDir),
		history:    history.New(),
		create:     create.New(),
		edit:       edit.New(),
		projectDir: projectDir,
		tmux:       t,
		keys:       core.DefaultKeyMap(),
		shared:     &shared{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		animTickCmd(),
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
		m.create = m.create.SetSize(msg.Width, msg.Height-4)
		m.edit = m.edit.SetSize(msg.Width, msg.Height-4)
		return m, nil

	case tea.KeyMsg:
		// Global quit (not in launch form where q is valid input)
		if key.Matches(msg, m.keys.Quit) && m.activeView != core.ViewLaunch && m.activeView != core.ViewCreate && m.activeView != core.ViewEditWorkflow {
			return m, tea.Quit
		}

		// Global back — let sub-views handle Back first when they have
		// internal navigation (e.g., detail output viewport → step list).
		if key.Matches(msg, m.keys.Back) {
			switch m.activeView {
			case core.ViewDetail:
				if !m.detail.ShowingOutput() {
					m.activeView = core.ViewDashboard
					return m, nil
				}
				// Fall through to let detail.Update handle it
			case core.ViewLaunch, core.ViewHistory, core.ViewCreate:
				m.activeView = core.ViewDashboard
				return m, nil
			case core.ViewEditWorkflow:
				// Edit view handles esc internally (dirty check, step collapse).
				// Fall through to let edit.Update handle it.
			case core.ViewDashboard:
				return m, tea.Quit
			}
		}

	case core.AnimTickMsg:
		cmds = append(cmds, animTickCmd())
		m.dashboard = m.dashboard.AnimTick()
		m.detail = m.detail.AnimTick()
		return m, tea.Batch(cmds...)

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
			m.launch = launch.NewWithProject(m.projectDir)
			m.launch = m.launch.SetSize(m.width, m.height-4)
			m.launch.Focus()
		case core.ViewCreate:
			m.create = create.New()
			m.create = m.create.SetSize(m.width, m.height-4)
			m.create.Focus()
		case core.ViewEditWorkflow:
			m.edit = edit.New()
			m.edit = m.edit.SetSize(m.width, m.height-4)
			m.edit.Focus(m.projectDir)
		}
		return m, tea.Batch(cmds...)

	case core.HistoryLoadedMsg:
		m.history = m.history.SetRecords(msg.Records)
		m.dashboard = m.dashboard.UpdateHistory(msg.Records)
		return m, nil

	case core.StepStartMsg:
		m.detail = m.detail.StepStarted(msg)
		// Persist step progress to disk so it survives view re-navigation
		if rec, err := pipeline.LoadRunRecord(m.projectDir, msg.RunID); err == nil {
			found := false
			for i := range rec.Steps {
				if rec.Steps[i].Name == msg.Step {
					rec.Steps[i].Status = pipeline.StatusRunning
					rec.Steps[i].Attempt = msg.Attempt
					found = true
					break
				}
			}
			if !found {
				rec.Steps = append(rec.Steps, pipeline.StepRecord{
					Name:      msg.Step,
					Status:    pipeline.StatusRunning,
					Attempt:   msg.Attempt,
					StartedAt: time.Now(),
				})
			}
			_ = pipeline.SaveRunRecord(m.projectDir, rec)
		}
		return m, nil

	case core.StepDoneMsg:
		m.detail = m.detail.StepDone(msg)
		if rec, err := pipeline.LoadRunRecord(m.projectDir, msg.RunID); err == nil {
			for i := range rec.Steps {
				if rec.Steps[i].Name == msg.Step {
					if msg.Passed {
						rec.Steps[i].Status = pipeline.StatusPassed
					} else {
						rec.Steps[i].Status = pipeline.StatusFailed
					}
					now := time.Now()
					rec.Steps[i].EndedAt = &now
					break
				}
			}
			_ = pipeline.SaveRunRecord(m.projectDir, rec)
		}
		return m, nil

	case core.PipelineFinishedMsg:
		// Reload the detail view's run record so the status badge and
		// step states reflect the final outcome (failed, passed, etc.)
		m.detail = m.detail.RefreshRun(m.projectDir)
		cmds = append(cmds, m.loadHistory())
		return m, tea.Batch(cmds...)

	case core.SessionKillMsg:
		agent := msg.Agent
		runID := msg.RunID
		t := m.tmux
		projectDir := m.projectDir
		killCmd := func() tea.Msg {
			session := pipeline.SessionPrefix + agent
			_ = t.KillSessionWithProcesses(session)
			if runID != "" {
				if rec, err := pipeline.LoadRunRecord(projectDir, runID); err == nil {
					if rec.Status == pipeline.StatusRunning {
						rec.Status = pipeline.StatusCancelled
						now := time.Now()
						rec.EndedAt = &now
						_ = pipeline.SaveRunRecord(projectDir, rec)
					}
				}
			}
			return nil
		}
		return m, tea.Sequence(killCmd, m.refreshSessions(), m.loadHistory())

	case core.RunDeletedMsg:
		_ = pipeline.DeleteRunRecord(m.projectDir, msg.RunID)
		cmds = append(cmds, m.loadHistory())
		return m, tea.Batch(cmds...)

	case core.RetryPipelineMsg:
		record := pipeline.NewRunRecord(msg.Task, msg.Agent)
		if err := pipeline.SaveRunRecord(m.projectDir, record); err != nil {
			return m, nil
		}
		projectDir := m.projectDir
		program := m.shared.program
		go func() {
			result, err := pipeline.Run(pipeline.PipelineOpts{
				Task:        msg.Task,
				ProjectDir:  projectDir,
				AgentPreset: msg.Agent,
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
		m.activeView = core.ViewDetail
		m.detail = m.detail.SetRunID(record.ID, m.projectDir)
		cmds = append(cmds, m.loadHistory())
		return m, tea.Batch(cmds...)

	case create.CreateSessionEndedMsg:
		// User returned from the tmux create session — go back to dashboard.
		m.activeView = core.ViewDashboard
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
	case core.ViewCreate:
		var cmd tea.Cmd
		m.create, cmd = m.create.Update(msg, m.projectDir)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case core.ViewEditWorkflow:
		var cmd tea.Cmd
		m.edit, cmd = m.edit.Update(msg, m.projectDir)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	var content string

	switch m.activeView {
	case core.ViewDashboard:
		content = m.dashboard.View()
	case core.ViewDetail:
		content = m.detail.View()
	case core.ViewLaunch:
		content = m.launch.View()
	case core.ViewHistory:
		content = m.history.View()
	case core.ViewCreate:
		content = m.create.View()
	case core.ViewEditWorkflow:
		content = m.edit.View()
	}

	help := m.helpView()

	// Title bar: badge + subtle line
	badge := titleBadge.Render("DevPit")
	lineWidth := m.width - lipgloss.Width(badge) - 1
	if lineWidth < 0 {
		lineWidth = 0
	}
	titleLine := badge + " " + lipgloss.NewStyle().
		Foreground(lipgloss.Color(core.ColorDim)).
		Render(strings.Repeat("─", lineWidth))

	// Calculate remaining space for content
	titleHeight := 1 // title line
	helpHeight := lipgloss.Height(help)
	contentHeight := m.height - titleHeight - helpHeight - 2 // 2 for spacing
	if contentHeight < 5 {
		contentHeight = 5
	}

	// Pad content to push help bar to bottom
	contentLines := strings.Count(content, "\n") + 1
	if contentLines < contentHeight {
		content += strings.Repeat("\n", contentHeight-contentLines)
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		titleLine,
		"",
		content,
		help,
	)
}

func (m Model) helpView() string {
	var keys []string
	switch m.activeView {
	case core.ViewDashboard:
		keys = []string{"n:new", "c:create", "e:edit", "h:history", "enter:view", "r:retry", "x:kill", "d:delete", "q:quit"}
	case core.ViewDetail:
		keys = []string{"enter:view output", "r:retry", "esc:back", "q:quit"}
	case core.ViewLaunch:
		keys = []string{"tab:next field", "enter:submit", "esc:cancel"}
	case core.ViewCreate:
		keys = []string{"tab:next field", "space:toggle", "enter:submit", "esc:cancel"}
	case core.ViewHistory:
		keys = []string{"enter:view", "esc:back"}
	case core.ViewEditWorkflow:
		keys = []string{"tab:next field", "ctrl+s:save", "ctrl+↑↓:reorder", "enter:edit step", "a:add", "x:delete", "esc:back"}
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted))
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))

	var parts []string
	for _, k := range keys {
		pair := strings.SplitN(k, ":", 2)
		if len(pair) == 2 {
			parts = append(parts, keyStyle.Render(pair[0])+descStyle.Render(" "+pair[1]))
		}
	}
	return helpStyle.Render(strings.Join(parts, sepStyle.Render("  │  ")))
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return core.TickMsg(t)
	})
}

func animTickCmd() tea.Cmd {
	return tea.Tick(animInterval, func(t time.Time) tea.Msg {
		return core.AnimTickMsg(t)
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
	projectDir := m.projectDir
	return func() tea.Msg {
		// Use color-preserving capture so ANSI escape codes come through
		output, err := m.tmux.CapturePaneColor(session, 200)
		if err != nil {
			// Session is dead — fall back to saved artifact so the user
			// can still inspect what happened.
			artifact, _ := pipeline.LoadArtifact(projectDir, agent)
			if artifact == "" {
				// Try the raw capture which includes startup, errors, etc.
				artifact, _ = pipeline.LoadArtifact(projectDir, agent+"-raw")
			}
			return core.PaneOutputMsg{Agent: agent, Output: artifact, IsIdle: true}
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
