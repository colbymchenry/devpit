package edit

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/colbymchenry/devpit/internal/pipeline"
	"github.com/colbymchenry/devpit/internal/tui/core"
)

// ── Modes & focus targets ─────────────────────────────────────────────

type editorMode int

const (
	modePicker   editorMode = iota // workflow selection list
	modeEditor                     // workflow-level fields + step list
	modeStepEdit                   // one step expanded inline
)

// Editor-level focus targets.
const (
	focusName = iota
	focusDesc
	focusSteps
)

// Step-edit focus targets.
const (
	sfName = iota
	sfAgent
	sfDirective
	sfContext
	sfOptional
	sfLoop
	sfLoopGoto
	sfLoopMax
	sfLoopPass
	sfLoopFail
	sfCount
)

// ── Model ─────────────────────────────────────────────────────────────

// Model is the workflow editor view model.
type Model struct {
	mode editorMode

	// picker
	workflows    []string
	pickerCursor int

	// workflow being edited
	workflowPath string
	origName     string // original name for rename detection
	dirty        bool

	// editor fields
	focus     int
	nameInput textinput.Model
	descInput textarea.Model
	steps     []pipeline.StepConfig
	stepCur   int

	// step editor fields
	stepFocus    int
	stepName     textinput.Model
	stepAgent    textinput.Model
	stepDir      textarea.Model
	stepCtx      textinput.Model
	stepOptional bool
	stepLoop     bool
	stepLoopGoto textinput.Model
	stepLoopMax  textinput.Model
	stepLoopPass textinput.Model
	stepLoopFail textinput.Model
	editIdx      int

	// status
	statusMsg string
	statusErr bool
	escOnce   bool // true after first esc with unsaved changes

	width  int
	height int
	keys   core.KeyMap
}

// New creates a new editor model.
func New() Model {
	return Model{
		keys: core.DefaultKeyMap(),
	}
}

// Focus discovers workflows and enters the editor.
func (m *Model) Focus(projectDir string) {
	m.workflows = pipeline.DiscoverWorkflows(projectDir)
	m.statusMsg = ""
	m.statusErr = false
	m.escOnce = false

	if len(m.workflows) == 1 {
		m.loadWorkflow(projectDir, m.workflows[0])
	} else {
		m.mode = modePicker
		m.pickerCursor = 0
	}
}

// SetSize updates available dimensions.
func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

// ── Workflow loading ──────────────────────────────────────────────────

func (m *Model) loadWorkflow(projectDir, name string) {
	path, err := pipeline.FindWorkflow(projectDir, name)
	if err != nil {
		m.statusMsg = "Cannot find workflow: " + err.Error()
		m.statusErr = true
		return
	}
	wf, err := pipeline.LoadWorkflow(path)
	if err != nil {
		m.statusMsg = "Cannot load workflow: " + err.Error()
		m.statusErr = true
		return
	}

	m.mode = modeEditor
	m.workflowPath = path
	m.origName = wf.Name
	m.dirty = false
	m.steps = make([]pipeline.StepConfig, len(wf.Steps))
	copy(m.steps, wf.Steps)
	m.stepCur = 0
	m.focus = focusSteps
	m.statusMsg = ""

	m.nameInput = newTextInput(wf.Name, "Workflow name...", m.inputWidth())
	m.descInput = newTextArea(wf.Description, "Description...", m.inputWidth(), 3)
	m.nameInput.Blur()
	m.descInput.Blur()
}

// ── Save ──────────────────────────────────────────────────────────────

func (m *Model) save() {
	// If in step edit, flush step fields first.
	if m.mode == modeStepEdit {
		m.flushStepFields()
		m.mode = modeEditor
		m.focus = focusSteps
	}

	wf := &pipeline.WorkflowConfig{
		Name:        strings.TrimSpace(m.nameInput.Value()),
		Description: strings.TrimSpace(m.descInput.Value()),
		Steps:       m.steps,
	}

	// Handle rename: if name changed, rename the file.
	newName := wf.Name
	if newName == "" {
		m.statusMsg = "Workflow name cannot be empty"
		m.statusErr = true
		return
	}

	targetPath := m.workflowPath
	if newName != m.origName {
		dir := filepath.Dir(m.workflowPath)
		targetPath = filepath.Join(dir, newName+".yaml")
		// Check if target already exists (and it's not the same file).
		if targetPath != m.workflowPath {
			if _, err := os.Stat(targetPath); err == nil {
				m.statusMsg = fmt.Sprintf("Workflow %q already exists", newName)
				m.statusErr = true
				return
			}
		}
	}

	if err := pipeline.SaveWorkflow(targetPath, wf); err != nil {
		m.statusMsg = err.Error()
		m.statusErr = true
		return
	}

	// Remove old file if renamed.
	if targetPath != m.workflowPath {
		_ = os.Remove(m.workflowPath)
		m.workflowPath = targetPath
		m.origName = newName
	}

	m.dirty = false
	m.statusMsg = "Saved"
	m.statusErr = false
}

// ── Update ────────────────────────────────────────────────────────────

// Update handles input for the edit view.
func (m Model) Update(msg tea.Msg, projectDir string) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// ctrl+s saves from any mode.
		if msg.String() == "ctrl+s" {
			m.save()
			return m, func() tea.Msg { return core.WorkflowSavedMsg{} }
		}

		switch m.mode {
		case modePicker:
			return m.updatePicker(msg, projectDir)
		case modeEditor:
			return m.updateEditor(msg)
		case modeStepEdit:
			return m.updateStepEdit(msg)
		}
	}

	return m, nil
}

func (m Model) updatePicker(msg tea.KeyMsg, projectDir string) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.pickerCursor > 0 {
			m.pickerCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.pickerCursor < len(m.workflows)-1 {
			m.pickerCursor++
		}
	case key.Matches(msg, m.keys.Enter):
		if m.pickerCursor < len(m.workflows) {
			m.loadWorkflow(projectDir, m.workflows[m.pickerCursor])
		}
	case key.Matches(msg, m.keys.Back):
		return m, func() tea.Msg {
			return core.NavigateMsg{View: core.ViewDashboard}
		}
	}
	return m, nil
}

func (m Model) updateEditor(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Esc — back (with dirty check).
	if key.Matches(msg, m.keys.Back) {
		if m.dirty && !m.escOnce {
			m.escOnce = true
			m.statusMsg = "Unsaved changes — press esc again to discard"
			m.statusErr = true
			return m, nil
		}
		return m, func() tea.Msg {
			return core.NavigateMsg{View: core.ViewDashboard}
		}
	}
	m.escOnce = false

	switch m.focus {
	case focusName:
		return m.updateFocusName(msg)
	case focusDesc:
		return m.updateFocusDesc(msg)
	case focusSteps:
		return m.updateFocusSteps(msg)
	}
	return m, nil
}

func (m Model) updateFocusName(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Tab) || key.Matches(msg, m.keys.Down) {
		m.nameInput.Blur()
		m.descInput.Focus()
		m.focus = focusDesc
		return m, nil
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	m.dirty = true
	return m, cmd
}

func (m Model) updateFocusDesc(msg tea.KeyMsg) (Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Tab) {
		m.descInput.Blur()
		m.focus = focusSteps
		return m, nil
	}
	var cmd tea.Cmd
	m.descInput, cmd = m.descInput.Update(msg)
	m.dirty = true
	return m, cmd
}

func (m Model) updateFocusSteps(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Tab):
		m.nameInput.Focus()
		m.focus = focusName
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.stepCur > 0 {
			m.stepCur--
		}

	case key.Matches(msg, m.keys.Down):
		if m.stepCur < len(m.steps)-1 {
			m.stepCur++
		}

	case msg.String() == "ctrl+up":
		if m.stepCur > 0 {
			m.steps[m.stepCur], m.steps[m.stepCur-1] = m.steps[m.stepCur-1], m.steps[m.stepCur]
			m.stepCur--
			m.dirty = true
		}

	case msg.String() == "ctrl+down":
		if m.stepCur < len(m.steps)-1 {
			m.steps[m.stepCur], m.steps[m.stepCur+1] = m.steps[m.stepCur+1], m.steps[m.stepCur]
			m.stepCur++
			m.dirty = true
		}

	case key.Matches(msg, m.keys.Enter):
		if len(m.steps) > 0 {
			m.expandStep(m.stepCur)
		}

	case msg.String() == "a":
		newStep := pipeline.StepConfig{Name: fmt.Sprintf("step-%d", len(m.steps)+1)}
		if m.stepCur < len(m.steps) {
			// Insert after cursor.
			m.steps = append(m.steps[:m.stepCur+1], append([]pipeline.StepConfig{newStep}, m.steps[m.stepCur+1:]...)...)
			m.stepCur++
		} else {
			m.steps = append(m.steps, newStep)
			m.stepCur = len(m.steps) - 1
		}
		m.dirty = true
		m.expandStep(m.stepCur)

	case msg.String() == "x" || key.Matches(msg, m.keys.Delete):
		if len(m.steps) > 0 {
			m.steps = append(m.steps[:m.stepCur], m.steps[m.stepCur+1:]...)
			if m.stepCur >= len(m.steps) && len(m.steps) > 0 {
				m.stepCur = len(m.steps) - 1
			}
			m.dirty = true
		}
	}

	return m, nil
}

// ── Step edit ─────────────────────────────────────────────────────────

func (m *Model) expandStep(idx int) {
	if idx < 0 || idx >= len(m.steps) {
		return
	}
	step := m.steps[idx]
	m.mode = modeStepEdit
	m.editIdx = idx
	m.stepFocus = sfName

	w := m.inputWidth() - 8

	m.stepName = newTextInput(step.Name, "Step name...", w)
	m.stepAgent = newTextInput(step.Agent, "Agent (blank = same as name)...", w)
	m.stepDir = newTextArea(step.Directive, "Directive...", w, 3)
	m.stepCtx = newTextInput(strings.Join(step.Context, ", "), "Context steps (comma-separated)...", w)
	m.stepOptional = step.Optional

	m.stepLoop = step.Loop != nil
	if step.Loop != nil {
		m.stepLoopGoto = newTextInput(step.Loop.Goto, "Goto step...", w/2)
		m.stepLoopMax = newTextInput(itoa(step.Loop.Max), "Max retries...", w/2)
		m.stepLoopPass = newTextInput(step.Loop.PassMarker, "Pass marker...", w/2)
		m.stepLoopFail = newTextInput(step.Loop.FailMarker, "Fail marker...", w/2)
	} else {
		m.stepLoopGoto = newTextInput("", "Goto step...", w/2)
		m.stepLoopMax = newTextInput("3", "Max retries...", w/2)
		m.stepLoopPass = newTextInput("PASS", "Pass marker...", w/2)
		m.stepLoopFail = newTextInput("FAIL", "Fail marker...", w/2)
	}

	m.blurAllStepFields()
	m.stepName.Focus()
}

func (m *Model) flushStepFields() {
	if m.editIdx < 0 || m.editIdx >= len(m.steps) {
		return
	}
	s := &m.steps[m.editIdx]
	s.Name = strings.TrimSpace(m.stepName.Value())
	s.Agent = strings.TrimSpace(m.stepAgent.Value())
	s.Directive = strings.TrimSpace(m.stepDir.Value())

	// Parse context.
	raw := strings.TrimSpace(m.stepCtx.Value())
	if raw == "" {
		s.Context = nil
	} else {
		parts := strings.Split(raw, ",")
		s.Context = nil
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				s.Context = append(s.Context, p)
			}
		}
	}

	s.Optional = m.stepOptional

	if m.stepLoop {
		maxVal, _ := strconv.Atoi(strings.TrimSpace(m.stepLoopMax.Value()))
		s.Loop = &pipeline.LoopConfig{
			Goto:       strings.TrimSpace(m.stepLoopGoto.Value()),
			Max:        maxVal,
			PassMarker: strings.TrimSpace(m.stepLoopPass.Value()),
			FailMarker: strings.TrimSpace(m.stepLoopFail.Value()),
		}
	} else {
		s.Loop = nil
	}
	m.dirty = true
}

func (m Model) updateStepEdit(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Esc — collapse back to editor.
	if key.Matches(msg, m.keys.Back) {
		m.flushStepFields()
		m.mode = modeEditor
		m.focus = focusSteps
		return m, nil
	}

	// Tab — cycle fields.
	if key.Matches(msg, m.keys.Tab) {
		m.blurAllStepFields()
		m.stepFocus = m.nextStepFocus(m.stepFocus)
		m.focusStepField(m.stepFocus)
		return m, nil
	}

	// Space on toggles.
	if msg.String() == " " {
		if m.stepFocus == sfOptional {
			m.stepOptional = !m.stepOptional
			m.dirty = true
			return m, nil
		}
		if m.stepFocus == sfLoop {
			m.stepLoop = !m.stepLoop
			m.dirty = true
			return m, nil
		}
	}

	// Delegate to focused input.
	var cmd tea.Cmd
	switch m.stepFocus {
	case sfName:
		m.stepName, cmd = m.stepName.Update(msg)
	case sfAgent:
		m.stepAgent, cmd = m.stepAgent.Update(msg)
	case sfDirective:
		m.stepDir, cmd = m.stepDir.Update(msg)
	case sfContext:
		m.stepCtx, cmd = m.stepCtx.Update(msg)
	case sfLoopGoto:
		m.stepLoopGoto, cmd = m.stepLoopGoto.Update(msg)
	case sfLoopMax:
		m.stepLoopMax, cmd = m.stepLoopMax.Update(msg)
	case sfLoopPass:
		m.stepLoopPass, cmd = m.stepLoopPass.Update(msg)
	case sfLoopFail:
		m.stepLoopFail, cmd = m.stepLoopFail.Update(msg)
	}
	m.dirty = true
	return m, cmd
}

func (m *Model) nextStepFocus(cur int) int {
	for {
		cur = (cur + 1) % sfCount
		// Skip loop sub-fields when loop is off.
		if !m.stepLoop && cur >= sfLoopGoto && cur <= sfLoopFail {
			continue
		}
		return cur
	}
}

func (m *Model) blurAllStepFields() {
	m.stepName.Blur()
	m.stepAgent.Blur()
	m.stepDir.Blur()
	m.stepCtx.Blur()
	m.stepLoopGoto.Blur()
	m.stepLoopMax.Blur()
	m.stepLoopPass.Blur()
	m.stepLoopFail.Blur()
}

func (m *Model) focusStepField(f int) {
	switch f {
	case sfName:
		m.stepName.Focus()
	case sfAgent:
		m.stepAgent.Focus()
	case sfDirective:
		m.stepDir.Focus()
	case sfContext:
		m.stepCtx.Focus()
	case sfLoopGoto:
		m.stepLoopGoto.Focus()
	case sfLoopMax:
		m.stepLoopMax.Focus()
	case sfLoopPass:
		m.stepLoopPass.Focus()
	case sfLoopFail:
		m.stepLoopFail.Focus()
	}
}

// ── View ──────────────────────────────────────────────────────────────

// View renders the edit view.
func (m Model) View() string {
	switch m.mode {
	case modePicker:
		return m.viewPicker()
	default:
		return m.viewEditor()
	}
}

func (m Model) viewPicker() string {
	pw := m.panelWidth()
	bc := lipgloss.Color(core.ColorBorder)

	var lines []string
	lines = append(lines, core.PanelTop("Edit Workflow", pw, bc))
	lines = append(lines, core.PanelEmpty(pw, bc))

	if len(m.workflows) == 0 {
		msg := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).
			Render("No workflows found. Press esc, then c to create one.")
		lines = append(lines, core.PanelRow(msg, pw, bc))
	} else {
		label := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).
			Render("Select a workflow:")
		lines = append(lines, core.PanelRow(label, pw, bc))
		lines = append(lines, core.PanelEmpty(pw, bc))

		for i, name := range m.workflows {
			cursor := "  "
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorText))
			if i == m.pickerCursor {
				cursor = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Render("▸ ")
				style = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorWhite)).Bold(true)
			}
			lines = append(lines, core.PanelRow(cursor+style.Render(name), pw, bc))
		}
	}

	lines = append(lines, core.PanelEmpty(pw, bc))
	lines = append(lines, core.PanelBottom(pw, bc))
	return strings.Join(lines, "\n")
}

func (m Model) viewEditor() string {
	pw := m.panelWidth()
	bc := lipgloss.Color(core.ColorBorder)
	label := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted))
	focusLabel := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim))

	title := "Edit: " + strings.TrimSpace(m.nameInput.Value())
	if m.dirty {
		title += " *"
	}

	var lines []string
	lines = append(lines, core.PanelTop(title, pw, bc))
	lines = append(lines, core.PanelEmpty(pw, bc))

	// Name field.
	if m.mode == modeEditor && m.focus == focusName {
		lines = append(lines, core.PanelRow(focusLabel.Render("Name"), pw, bc))
	} else {
		lines = append(lines, core.PanelRow(label.Render("Name"), pw, bc))
	}
	lines = append(lines, core.PanelRow(m.nameInput.View(), pw, bc))
	lines = append(lines, m.fieldUnderline(m.mode == modeEditor && m.focus == focusName, pw, bc))
	lines = append(lines, core.PanelEmpty(pw, bc))

	// Description field.
	if m.mode == modeEditor && m.focus == focusDesc {
		lines = append(lines, core.PanelRow(focusLabel.Render("Description"), pw, bc))
	} else {
		lines = append(lines, core.PanelRow(label.Render("Description"), pw, bc))
	}
	for _, line := range strings.Split(m.descInput.View(), "\n") {
		lines = append(lines, core.PanelRow(line, pw, bc))
	}
	lines = append(lines, core.PanelEmpty(pw, bc))

	// Steps header.
	stepsLabel := "Steps"
	if m.mode == modeEditor && m.focus == focusSteps {
		hint := dimStyle.Render("  ctrl+↑↓ reorder · a add · x delete")
		lines = append(lines, core.PanelRow(focusLabel.Render(stepsLabel)+hint, pw, bc))
	} else {
		lines = append(lines, core.PanelRow(label.Render(stepsLabel), pw, bc))
	}

	if len(m.steps) == 0 {
		empty := dimStyle.Render("(empty — press a to add a step)")
		lines = append(lines, core.PanelRow(empty, pw, bc))
	}

	for i, step := range m.steps {
		if m.mode == modeStepEdit && i == m.editIdx {
			lines = append(lines, m.viewExpandedStep(i, step, pw, bc)...)
			continue
		}

		isSel := (m.mode == modeEditor && m.focus == focusSteps && i == m.stepCur)
		lines = append(lines, m.viewStepRow(i, step, isSel, pw, bc))
	}

	// Status message.
	lines = append(lines, core.PanelEmpty(pw, bc))
	if m.statusMsg != "" {
		var sStyle lipgloss.Style
		if m.statusErr {
			sStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorRed))
		} else {
			sStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorGreen))
		}
		lines = append(lines, core.PanelRow(sStyle.Render(m.statusMsg), pw, bc))
	}

	lines = append(lines, core.PanelEmpty(pw, bc))
	lines = append(lines, core.PanelBottom(pw, bc))
	return strings.Join(lines, "\n")
}

func (m Model) viewStepRow(idx int, step pipeline.StepConfig, selected bool, pw int, bc lipgloss.Color) string {
	cursor := "  "
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorText))
	if selected {
		cursor = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Render("▸ ")
		nameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorWhite)).Bold(true)
	}

	num := fmt.Sprintf("%d. ", idx+1)
	agent := step.AgentName()
	agentStr := ""
	if agent != step.Name {
		agentStr = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted)).Render("  (" + agent + ")")
	}
	loopStr := ""
	if step.Loop != nil {
		loopStr = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorAmber)).Render("  ↻loop")
	}
	optStr := ""
	if step.Optional {
		optStr = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Render("  optional")
	}

	row := cursor + lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Render(num) +
		nameStyle.Render(step.Name) + agentStr + loopStr + optStr

	return core.PanelRow(row, pw, bc)
}

func (m Model) viewExpandedStep(idx int, step pipeline.StepConfig, pw int, bc lipgloss.Color) []string {
	var lines []string
	fl := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Bold(true)
	dl := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorMuted))
	editBadge := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorAmber)).Render(" [editing]")

	num := fmt.Sprintf("%d. ", idx+1)
	header := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight)).Render("▸ ") +
		lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Render(num) +
		lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorWhite)).Bold(true).Render(step.Name) +
		editBadge
	lines = append(lines, core.PanelRow(header, pw, bc))

	pad := "    "

	// Name.
	lbl := dl.Render("Name")
	if m.stepFocus == sfName {
		lbl = fl.Render("Name")
	}
	lines = append(lines, core.PanelRow(pad+lbl, pw, bc))
	lines = append(lines, core.PanelRow(pad+m.stepName.View(), pw, bc))

	// Agent.
	lbl = dl.Render("Agent")
	if m.stepFocus == sfAgent {
		lbl = fl.Render("Agent")
	}
	lines = append(lines, core.PanelRow(pad+lbl, pw, bc))
	lines = append(lines, core.PanelRow(pad+m.stepAgent.View(), pw, bc))

	// Directive.
	lbl = dl.Render("Directive")
	if m.stepFocus == sfDirective {
		lbl = fl.Render("Directive")
	}
	lines = append(lines, core.PanelRow(pad+lbl, pw, bc))
	for _, line := range strings.Split(m.stepDir.View(), "\n") {
		lines = append(lines, core.PanelRow(pad+line, pw, bc))
	}

	// Context.
	lbl = dl.Render("Context")
	if m.stepFocus == sfContext {
		lbl = fl.Render("Context")
	}
	lines = append(lines, core.PanelRow(pad+lbl, pw, bc))
	lines = append(lines, core.PanelRow(pad+m.stepCtx.View(), pw, bc))

	// Optional toggle.
	check := "[ ]"
	if m.stepOptional {
		check = "[x]"
	}
	lbl = dl.Render("Optional ")
	if m.stepFocus == sfOptional {
		lbl = fl.Render("Optional ")
	}
	checkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight))
	lines = append(lines, core.PanelRow(pad+lbl+checkStyle.Render(check), pw, bc))

	// Loop toggle.
	loopCheck := "[ ]"
	if m.stepLoop {
		loopCheck = "[x]"
	}
	lbl = dl.Render("Loop ")
	if m.stepFocus == sfLoop {
		lbl = fl.Render("Loop ")
	}
	lines = append(lines, core.PanelRow(pad+lbl+checkStyle.Render(loopCheck), pw, bc))

	// Loop sub-fields (only if loop enabled).
	if m.stepLoop {
		subPad := pad + "  "
		for _, sf := range []struct {
			label string
			focus int
			input textinput.Model
		}{
			{"Goto", sfLoopGoto, m.stepLoopGoto},
			{"Max", sfLoopMax, m.stepLoopMax},
			{"Pass", sfLoopPass, m.stepLoopPass},
			{"Fail", sfLoopFail, m.stepLoopFail},
		} {
			lbl = dl.Render(sf.label + " ")
			if m.stepFocus == sf.focus {
				lbl = fl.Render(sf.label + " ")
			}
			lines = append(lines, core.PanelRow(subPad+lbl+sf.input.View(), pw, bc))
		}
	}

	lines = append(lines, core.PanelEmpty(pw, bc))
	return lines
}

// ── Helpers ───────────────────────────────────────────────────────────

func (m Model) panelWidth() int {
	pw := m.width
	if pw < 40 {
		pw = 40
	}
	if pw > 90 {
		pw = 90
	}
	return pw
}

func (m Model) inputWidth() int {
	w := m.panelWidth() - 8
	if w > 72 {
		w = 72
	}
	return w
}

func (m Model) fieldUnderline(focused bool, pw int, bc lipgloss.Color) string {
	w := m.inputWidth()
	if focused {
		return core.PanelRow(
			lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurple)).Render(strings.Repeat("─", w)),
			pw, bc)
	}
	return core.PanelRow(
		lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorDim)).Render(strings.Repeat("─", w)),
		pw, bc)
}

func newTextInput(value, placeholder string, width int) textinput.Model {
	ti := textinput.New()
	ti.SetValue(value)
	ti.Placeholder = placeholder
	ti.CharLimit = 200
	ti.Width = width
	ti.Prompt = "  "
	ti.PromptStyle = lipgloss.NewStyle()
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorWhite))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4B5563"))
	ti.Cursor.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(core.ColorPurpleLight))
	return ti
}

func newTextArea(value, placeholder string, width, height int) textarea.Model {
	ta := textarea.New()
	ta.SetValue(value)
	ta.Placeholder = placeholder
	ta.SetWidth(width)
	ta.SetHeight(height)
	ta.CharLimit = 2000
	ta.ShowLineNumbers = false
	ta.Prompt = "  "
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	return ta
}

func itoa(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}
