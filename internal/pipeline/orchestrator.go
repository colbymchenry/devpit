package pipeline

import (
	"fmt"
	"strings"
	"time"

	"github.com/colbymchenry/devpit/internal/tmux"
)

// DefaultModel is the default model for pipeline agents.
// Uses 1M context for maximum codebase coverage.
const DefaultModel = "opus[1m]"

// DefaultEffort maps pipeline step names to their default effort levels.
// These are used when the agent .md frontmatter doesn't specify an effort.
var DefaultEffort = map[string]string{
	"architect": "max",
	"coder":     "high",
	"tester":    "high",
	"reviewer":  "high",
	"design-qa": "high",
}

// PipelineOpts configures a pipeline run.
type PipelineOpts struct {
	// Task is the user's task description.
	Task string

	// ProjectDir is the root directory of the project.
	ProjectDir string

	// AgentPreset overrides the AI runtime (e.g., "claude", "gemini", "codex").
	// Empty uses the default.
	AgentPreset string

	// Model overrides the model for all steps (e.g., "opus[1m]", "sonnet").
	// Empty uses DefaultModel.
	Model string

	// StepTimeout is the max time per pipeline step.
	StepTimeout time.Duration

	// MaxRetries is the max coder↔tester or coder↔design-qa retry loops.
	MaxRetries int

	// SkipReview skips the reviewer step.
	SkipReview bool

	// SkipQA skips the design-qa step even if available.
	SkipQA bool

	// OnStepStart is called when a pipeline step begins. May be nil.
	OnStepStart func(step string, attempt int)

	// OnStepDone is called when a pipeline step completes. May be nil.
	OnStepDone func(step string, passed bool, output string)

	// SessionMap holds pre-generated session IDs for each agent step.
	// If nil, session IDs are not passed (legacy behavior).
	SessionMap *SessionMap

	// IsFollowUp indicates this is a follow-up run using --resume.
	// When true, prompts use BuildFollowUpPrompt and agents are resumed
	// with their prior conversation history.
	IsFollowUp bool
}

// PipelineResult contains the results of a full pipeline run.
type PipelineResult struct {
	Steps []StepResult
}

// StepResult contains the result of a single pipeline step.
type StepResult struct {
	Name    string
	Output  string
	Passed  bool
	Skipped bool
}

// Run executes the default pipeline by loading .claude/workflows/default.yaml.
func Run(opts PipelineOpts) (*PipelineResult, error) {
	wf, err := LoadDefaultWorkflow(opts.ProjectDir)
	if err != nil {
		return nil, err
	}
	return RunWorkflow(wf, opts)
}

// runStep executes a single pipeline step: spawn agent, wait for completion,
// capture output, kill session, save artifact.
func runStep(t *tmux.Tmux, name string, opts PipelineOpts, spawnOpts SpawnOptions, buildPrompt func() string) (string, error) {
	prompt := buildPrompt()

	// Save the prompt for debugging — lets the user inspect exactly what each agent received.
	_ = SaveArtifact(opts.ProjectDir, name+"-prompt", prompt)

	session, err := SpawnAgent(t, name, opts.ProjectDir, prompt, spawnOpts)
	if err != nil {
		return "", err
	}
	defer KillAgent(t, session)

	if err := WaitForCompletion(t, session, opts.StepTimeout); err != nil {
		return "", fmt.Errorf("wait for completion: %w", err)
	}

	output, err := ExtractOutput(t, session, opts.ProjectDir, name)
	if err != nil {
		return "", fmt.Errorf("extract output: %w", err)
	}

	// Always save the artifact so the user can inspect what happened
	_ = SaveArtifact(opts.ProjectDir, name, output)

	// Check for errors in the output before passing to the next step
	if err := ValidateAgentOutput(name, output); err != nil {
		return "", err
	}

	return output, nil
}

// notifyStart safely calls OnStepStart if non-nil.
func notifyStart(fn func(string, int), name string, attempt int) {
	if fn != nil {
		fn(name, attempt)
	}
}

// notifyDone safely calls OnStepDone if non-nil.
func notifyDone(fn func(string, bool, string), name string, passed bool, output string) {
	if fn != nil {
		fn(name, passed, output)
	}
}

// FormatSummary creates a terminal-friendly summary of pipeline results.
func FormatSummary(result *PipelineResult) string {
	var b strings.Builder
	b.WriteString("\n═══ Pipeline Summary ═══\n\n")

	for _, step := range result.Steps {
		status := "✅"
		if step.Skipped {
			status = "⏭️ "
		} else if !step.Passed {
			status = "❌"
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", status, step.Name))
	}

	b.WriteString(fmt.Sprintf("\nArtifacts saved to %s/\n", ArtifactDir))
	return b.String()
}
