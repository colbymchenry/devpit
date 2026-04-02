package core

import (
	"time"

	"github.com/colbymchenry/devpit/internal/pipeline"
)

// TickMsg fires on a regular interval to refresh tmux state.
type TickMsg time.Time

// SessionInfo holds live status for a pipeline tmux session.
type SessionInfo struct {
	Agent    string
	IsIdle   bool
	LastLine string
}

// SessionsUpdatedMsg carries refreshed tmux session state.
type SessionsUpdatedMsg struct {
	Sessions []SessionInfo
}

// PaneOutputMsg carries captured tmux output for a specific agent.
type PaneOutputMsg struct {
	Agent  string
	Output string
	IsIdle bool
}

// StepStartMsg is sent when a pipeline step begins.
type StepStartMsg struct {
	RunID   string
	Step    string
	Attempt int
}

// StepDoneMsg is sent when a pipeline step completes.
type StepDoneMsg struct {
	RunID  string
	Step   string
	Passed bool
	Output string
}

// PipelineFinishedMsg is sent when a pipeline run completes.
type PipelineFinishedMsg struct {
	RunID  string
	Result *pipeline.PipelineResult
	Err    error
}

// PipelineStartedMsg is sent when a new pipeline has been launched.
type PipelineStartedMsg struct {
	Record *pipeline.RunRecord
}

// HistoryLoadedMsg carries loaded history records.
type HistoryLoadedMsg struct {
	Records []*pipeline.RunRecord
}

// NavigateMsg requests a view change.
type NavigateMsg struct {
	View  View
	RunID string
}
