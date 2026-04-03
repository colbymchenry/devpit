package pipeline

import (
	"fmt"
	"os"
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

// Run executes the full pipeline: architect → coder → tester (retry) → reviewer → design-qa (retry).
func Run(opts PipelineOpts) (*PipelineResult, error) {
	if opts.StepTimeout == 0 {
		opts.StepTimeout = DefaultStepTimeout
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = DefaultMaxRetries
	}

	// Clear stale artifacts from previous runs. ExtractOutput checks for
	// .pipeline/<agent>.md files first — stale files from a failed run would
	// be returned instead of the actual tmux output from the current run.
	artifactDir := fmt.Sprintf("%s/%s", opts.ProjectDir, ArtifactDir)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}
	for _, name := range []string{"architect", "coder", "tester", "reviewer", VisualQAAgent} {
		_ = os.Remove(fmt.Sprintf("%s/%s.md", artifactDir, name))
		_ = os.Remove(fmt.Sprintf("%s/%s-raw.md", artifactDir, name))
	}

	t := tmux.NewTmux()
	result := &PipelineResult{}

	// Verify required agent files exist (loaded via --agent flag at spawn time)
	for _, name := range []string{"architect", "coder", "tester", "reviewer"} {
		if !AgentExists(opts.ProjectDir, name) {
			return nil, fmt.Errorf("agent %q not found (run dp setup-agents first)", name)
		}
	}
	hasDesignQA := AgentExists(opts.ProjectDir, VisualQAAgent)

	model := opts.Model
	if model == "" {
		model = DefaultModel
	}

	stepSpawn := func(step string) SpawnOptions {
		effort := DefaultEffort[step]
		so := SpawnOptions{
			AgentPreset: opts.AgentPreset,
			StepTimeout: opts.StepTimeout,
			Model:       model,
			Effort:      effort,
		}
		if opts.SessionMap != nil {
			if opts.IsFollowUp {
				so.ResumeID = opts.SessionMap.Agents[step]
			} else {
				so.SessionID = opts.SessionMap.Agents[step]
			}
		}
		return so
	}

	// --- Step 1: Architect ---
	notifyStart(opts.OnStepStart, "architect", 1)
	architectOutput, err := runStep(t, "architect", opts, stepSpawn("architect"),
		func() string {
			if opts.IsFollowUp {
				return BuildFollowUpPrompt("architect", opts.Task)
			}
			return BuildArchitectPrompt(opts.Task)
		})
	if err != nil {
		notifyDone(opts.OnStepDone, "architect", false, "")
		return nil, fmt.Errorf("architect: %w", err)
	}
	notifyDone(opts.OnStepDone, "architect", true, architectOutput)
	result.Steps = append(result.Steps, StepResult{Name: "architect", Output: architectOutput, Passed: true})

	// --- Step 2: Coder ---
	notifyStart(opts.OnStepStart, "coder", 1)
	coderOutput, err := runStep(t, "coder", opts, stepSpawn("coder"),
		func() string {
			if opts.IsFollowUp {
				return BuildFollowUpPrompt("coder", opts.Task)
			}
			return BuildCoderPrompt(opts.Task, architectOutput)
		})
	if err != nil {
		notifyDone(opts.OnStepDone, "coder", false, "")
		return nil, fmt.Errorf("coder: %w", err)
	}
	notifyDone(opts.OnStepDone, "coder", true, coderOutput)
	result.Steps = append(result.Steps, StepResult{Name: "coder", Output: coderOutput, Passed: true})

	// --- Step 3: Tester with coder retry loop ---
	testerOutput := ""
	testerPassed := false
	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		notifyStart(opts.OnStepStart, "tester", attempt)

		testerOutput, err = runStep(t, "tester", opts, stepSpawn("tester"),
			func() string {
				if opts.IsFollowUp {
					return BuildFollowUpPrompt("tester", opts.Task)
				}
				return BuildTesterPrompt(opts.Task, coderOutput)
			})
		if err != nil {
			notifyDone(opts.OnStepDone, "tester", false, "")
			return nil, fmt.Errorf("tester (attempt %d): %w", attempt, err)
		}

		testerPassed = ParseResult(testerOutput, ResultPass, ResultFail)
		notifyDone(opts.OnStepDone, "tester", testerPassed, testerOutput)

		if testerPassed || attempt == opts.MaxRetries {
			break
		}

		// Save failed attempt
		_ = SaveArtifact(opts.ProjectDir, fmt.Sprintf("tester-attempt-%d", attempt), testerOutput)

		// Retry coder with failure context
		notifyStart(opts.OnStepStart, "coder", attempt+1)
		coderOutput, err = runStep(t, "coder", opts, stepSpawn("coder"),
			func() string {
				if opts.IsFollowUp {
					return BuildFollowUpPrompt("coder", fmt.Sprintf("%s\n\nTest failures (attempt %d of %d):\n\n%s", opts.Task, attempt, opts.MaxRetries, testerOutput))
				}
				return BuildCoderRetryPrompt(opts.Task, architectOutput, testerOutput, attempt)
			})
		if err != nil {
			notifyDone(opts.OnStepDone, "coder", false, "")
			return nil, fmt.Errorf("coder retry (attempt %d): %w", attempt, err)
		}
		notifyDone(opts.OnStepDone, "coder", true, coderOutput)
	}
	result.Steps = append(result.Steps, StepResult{Name: "tester", Output: testerOutput, Passed: testerPassed})

	// --- Step 4: Reviewer ---
	if opts.SkipReview {
		result.Steps = append(result.Steps, StepResult{Name: "reviewer", Skipped: true})
	} else {
		notifyStart(opts.OnStepStart, "reviewer", 1)
		reviewerOutput, err := runStep(t, "reviewer", opts, stepSpawn("reviewer"),
			func() string {
				if opts.IsFollowUp {
					return BuildFollowUpPrompt("reviewer", opts.Task)
				}
				return BuildReviewerPrompt(opts.Task, architectOutput, coderOutput, testerOutput)
			})
		if err != nil {
			notifyDone(opts.OnStepDone, "reviewer", false, "")
			return nil, fmt.Errorf("reviewer: %w", err)
		}
		notifyDone(opts.OnStepDone, "reviewer", true, reviewerOutput)
		result.Steps = append(result.Steps, StepResult{Name: "reviewer", Output: reviewerOutput, Passed: true})
	}

	// --- Step 5: Design QA with coder retry loop ---
	if opts.SkipQA || !hasDesignQA {
		if hasDesignQA && opts.SkipQA {
			result.Steps = append(result.Steps, StepResult{Name: VisualQAAgent, Skipped: true})
		}
	} else {
		// Get reviewer output for context
		reviewerOut := ""
		for _, s := range result.Steps {
			if s.Name == "reviewer" && !s.Skipped {
				reviewerOut = s.Output
			}
		}

		qaOutput := ""
		qaPassed := false
		for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
			notifyStart(opts.OnStepStart, VisualQAAgent, attempt)

			qaOutput, err = runStep(t, VisualQAAgent, opts, stepSpawn(VisualQAAgent),
				func() string {
					if opts.IsFollowUp {
						return BuildFollowUpPrompt(VisualQAAgent, opts.Task)
					}
					return BuildDesignQAPrompt(opts.Task, coderOutput, reviewerOut)
				})
			if err != nil {
				notifyDone(opts.OnStepDone, VisualQAAgent, false, "")
				return nil, fmt.Errorf("design-qa (attempt %d): %w", attempt, err)
			}

			qaPassed = ParseResult(qaOutput, ResultAllClear, ResultIssuesFound)
			notifyDone(opts.OnStepDone, VisualQAAgent, qaPassed, qaOutput)

			if qaPassed || attempt == opts.MaxRetries {
				break
			}

			// Save failed attempt
			_ = SaveArtifact(opts.ProjectDir, fmt.Sprintf("design-qa-attempt-%d", attempt), qaOutput)

			// Retry coder with visual issues context
			notifyStart(opts.OnStepStart, "coder", attempt+1)
			coderOutput, err = runStep(t, "coder", opts, stepSpawn("coder"),
				func() string {
					if opts.IsFollowUp {
						return BuildFollowUpPrompt("coder", fmt.Sprintf("%s\n\nVisual issues found (attempt %d of %d):\n\n%s", opts.Task, attempt, opts.MaxRetries, qaOutput))
					}
					return BuildCoderDesignFixPrompt(opts.Task, architectOutput, qaOutput, attempt)
				})
			if err != nil {
				notifyDone(opts.OnStepDone, "coder", false, "")
				return nil, fmt.Errorf("coder design fix (attempt %d): %w", attempt, err)
			}
			notifyDone(opts.OnStepDone, "coder", true, coderOutput)
		}
		result.Steps = append(result.Steps, StepResult{Name: VisualQAAgent, Output: qaOutput, Passed: qaPassed})
	}

	return result, nil
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
