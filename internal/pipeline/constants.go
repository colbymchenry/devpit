// Package pipeline implements a sequential multi-agent pipeline for the gt CLI.
// Each step spawns a single AI agent in a tmux session, waits for completion,
// captures output, and passes context to the next step.
package pipeline

import "time"

const (
	// SessionPrefix is prepended to agent names for tmux session naming.
	// e.g., "pipeline-architect", "pipeline-coder"
	SessionPrefix = "pipeline-"

	// DefaultStepTimeout is how long to wait for a single pipeline step.
	DefaultStepTimeout = 10 * time.Minute

	// DefaultMaxRetries is the maximum coder↔tester or coder↔design-qa retry loops.
	DefaultMaxRetries = 3

	// BusyWaitTimeout is how long Phase A waits to see the agent become busy
	// before falling through to idle detection. Prevents false idle detection
	// when the agent hasn't started processing yet.
	BusyWaitTimeout = 30 * time.Second

	// ScrollbackLimit is the tmux history-limit set on pipeline sessions.
	// Needs to be large enough to capture full agent output.
	ScrollbackLimit = 50000

	// ArtifactDir is the directory where pipeline step outputs are saved.
	ArtifactDir = ".pipeline"

	// ResultPass is the marker tester/design-qa agents use to signal success.
	ResultPass = "PASS"
	// ResultFail is the marker tester agents use to signal test failures.
	ResultFail = "FAIL"
	// ResultAllClear is the marker design-qa agents use to signal no visual issues.
	ResultAllClear = "ALL CLEAR"
	// ResultIssuesFound is the marker design-qa agents use to signal visual issues.
	ResultIssuesFound = "ISSUES FOUND"
)

// CoreAgents are the agents that are always part of the pipeline.
var CoreAgents = []string{"architect", "coder", "tester", "reviewer"}

// VisualQAAgent is the optional agent for visual projects.
const VisualQAAgent = "design-qa"
