package pipeline

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/colbymchenry/devpit/internal/tmux"
)

// PipelineOpts configures a pipeline run.
type PipelineOpts struct {
	// Task is the user's task description.
	Task string

	// ProjectDir is the root directory of the project.
	ProjectDir string

	// AgentPreset overrides the AI runtime (e.g., "claude", "gemini", "codex").
	// Empty uses the default.
	AgentPreset string

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

	// Ensure artifact directory exists
	if err := os.MkdirAll(fmt.Sprintf("%s/%s", opts.ProjectDir, ArtifactDir), 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}

	t := tmux.NewTmux()
	result := &PipelineResult{}

	// Load agent configs
	agents := make(map[string]*AgentConfig)
	for _, name := range []string{"architect", "coder", "tester", "reviewer", VisualQAAgent} {
		cfg, err := LoadAgentConfig(opts.ProjectDir, name)
		if err != nil {
			if name == VisualQAAgent {
				continue // design-qa is optional
			}
			return nil, fmt.Errorf("load agent %q: %w", name, err)
		}
		agents[name] = cfg
	}

	spawnOpts := SpawnOptions{
		AgentPreset: opts.AgentPreset,
		StepTimeout: opts.StepTimeout,
	}

	// --- Step 1: Architect ---
	architectOutput, err := runStep(t, "architect", opts, agents, spawnOpts,
		func() string {
			return BuildArchitectPrompt(agents["architect"].Body, opts.Task)
		})
	if err != nil {
		return nil, fmt.Errorf("architect: %w", err)
	}
	result.Steps = append(result.Steps, StepResult{Name: "architect", Output: architectOutput, Passed: true})

	// --- Step 2: Coder ---
	coderOutput, err := runStep(t, "coder", opts, agents, spawnOpts,
		func() string {
			return BuildCoderPrompt(agents["coder"].Body, opts.Task, architectOutput)
		})
	if err != nil {
		return nil, fmt.Errorf("coder: %w", err)
	}
	result.Steps = append(result.Steps, StepResult{Name: "coder", Output: coderOutput, Passed: true})

	// --- Step 3: Tester with coder retry loop ---
	testerOutput := ""
	testerPassed := false
	for attempt := 1; attempt <= opts.MaxRetries; attempt++ {
		notifyStart(opts.OnStepStart, "tester", attempt)

		testerOutput, err = runStep(t, "tester", opts, agents, spawnOpts,
			func() string {
				return BuildTesterPrompt(agents["tester"].Body, opts.Task, coderOutput)
			})
		if err != nil {
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
		coderOutput, err = runStep(t, "coder", opts, agents, spawnOpts,
			func() string {
				return BuildCoderRetryPrompt(agents["coder"].Body, opts.Task, architectOutput, testerOutput, attempt)
			})
		if err != nil {
			return nil, fmt.Errorf("coder retry (attempt %d): %w", attempt, err)
		}
		notifyDone(opts.OnStepDone, "coder", true, coderOutput)
	}
	result.Steps = append(result.Steps, StepResult{Name: "tester", Output: testerOutput, Passed: testerPassed})

	// --- Step 4: Reviewer ---
	if opts.SkipReview {
		result.Steps = append(result.Steps, StepResult{Name: "reviewer", Skipped: true})
	} else {
		reviewerOutput, err := runStep(t, "reviewer", opts, agents, spawnOpts,
			func() string {
				return BuildReviewerPrompt(agents["reviewer"].Body, opts.Task, architectOutput, coderOutput, testerOutput)
			})
		if err != nil {
			return nil, fmt.Errorf("reviewer: %w", err)
		}
		result.Steps = append(result.Steps, StepResult{Name: "reviewer", Output: reviewerOutput, Passed: true})
	}

	// --- Step 5: Design QA with coder retry loop ---
	_, hasDesignQA := agents[VisualQAAgent]
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

			qaOutput, err = runStep(t, VisualQAAgent, opts, agents, spawnOpts,
				func() string {
					return BuildDesignQAPrompt(agents[VisualQAAgent].Body, opts.Task, coderOutput, reviewerOut)
				})
			if err != nil {
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
			coderOutput, err = runStep(t, "coder", opts, agents, spawnOpts,
				func() string {
					return BuildCoderDesignFixPrompt(agents["coder"].Body, opts.Task, architectOutput, qaOutput, attempt)
				})
			if err != nil {
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
func runStep(t *tmux.Tmux, name string, opts PipelineOpts, agents map[string]*AgentConfig, spawnOpts SpawnOptions, buildPrompt func() string) (string, error) {
	prompt := buildPrompt()

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

	_ = SaveArtifact(opts.ProjectDir, name, output)

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
