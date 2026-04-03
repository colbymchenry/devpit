package pipeline

import (
	"fmt"
	"os"

	"github.com/colbymchenry/devpit/internal/tmux"
)

// RunWorkflow executes a workflow config: walks steps in order, builds prompts
// from declared context, and handles loop-backs via a program counter.
func RunWorkflow(wf *WorkflowConfig, opts PipelineOpts) (*PipelineResult, error) {
	if opts.StepTimeout == 0 {
		opts.StepTimeout = DefaultStepTimeout
	}
	if opts.MaxRetries == 0 {
		opts.MaxRetries = DefaultMaxRetries
	}

	// Clear stale artifacts from previous runs.
	artifactDir := fmt.Sprintf("%s/%s", opts.ProjectDir, ArtifactDir)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}
	for _, step := range wf.Steps {
		_ = os.Remove(fmt.Sprintf("%s/%s.md", artifactDir, step.Name))
		_ = os.Remove(fmt.Sprintf("%s/%s-raw.md", artifactDir, step.Name))
	}

	t := tmux.NewTmux()
	result := &PipelineResult{}

	model := opts.Model
	if model == "" {
		model = DefaultModel
	}

	// Verify required (non-optional) agents exist.
	for _, step := range wf.Steps {
		agent := step.AgentName()
		if step.Optional {
			continue
		}
		if !AgentExists(opts.ProjectDir, agent) {
			return nil, fmt.Errorf("agent %q not found (run dp create first)", agent)
		}
	}

	// Build step index for goto lookups.
	stepIndex := make(map[string]int, len(wf.Steps))
	for i, step := range wf.Steps {
		stepIndex[step.Name] = i
	}

	// Execution state.
	outputs := make(map[string]string)    // step name -> latest output
	loopCounts := make(map[string]int)    // loop-bearing step name -> iteration count

	stepSpawn := func(step StepConfig) SpawnOptions {
		agent := step.AgentName()
		effort := DefaultEffort[agent]
		if effort == "" {
			effort = "high"
		}
		so := SpawnOptions{
			AgentPreset: opts.AgentPreset,
			StepTimeout: opts.StepTimeout,
			Model:       model,
			Effort:      effort,
		}
		if opts.SessionMap != nil {
			if opts.IsFollowUp {
				so.ResumeID = opts.SessionMap.Agents[agent]
			} else {
				so.SessionID = opts.SessionMap.Agents[agent]
			}
		}
		return so
	}

	pc := 0
	for pc < len(wf.Steps) {
		step := wf.Steps[pc]
		agent := step.AgentName()

		// Handle optional steps.
		if step.Optional && !AgentExists(opts.ProjectDir, agent) {
			result.Steps = append(result.Steps, StepResult{Name: step.Name, Skipped: true})
			pc++
			continue
		}

		attempt := loopCounts[step.Name] + 1
		notifyStart(opts.OnStepStart, step.Name, attempt)

		// Build context from declared dependencies.
		priorContext := make(map[string]string)
		for _, ref := range step.Context {
			if val, ok := outputs[ref]; ok {
				priorContext[ref] = val
			}
		}

		// Build prompt.
		var prompt string
		if opts.IsFollowUp {
			prompt = BuildWorkflowFollowUpPrompt(step, opts.Task)
		} else {
			prompt = BuildWorkflowPrompt(step, opts.Task, priorContext)
		}

		// Execute the step.
		output, err := runStep(t, agent, opts, stepSpawn(step), func() string { return prompt })
		if err != nil {
			notifyDone(opts.OnStepDone, step.Name, false, "")
			return nil, fmt.Errorf("%s: %w", step.Name, err)
		}

		outputs[step.Name] = output

		// Loop logic.
		if step.Loop != nil {
			maxIter := step.Loop.Max
			if maxIter == 0 {
				maxIter = DefaultMaxRetries
			}

			passed := ParseResult(output, step.Loop.PassMarker, step.Loop.FailMarker)
			notifyDone(opts.OnStepDone, step.Name, passed, output)

			if !passed && loopCounts[step.Name] < maxIter-1 {
				// Save failed attempt artifact.
				_ = SaveArtifact(opts.ProjectDir, fmt.Sprintf("%s-attempt-%d", step.Name, attempt), output)
				loopCounts[step.Name]++

				// Jump back to goto target.
				pc = stepIndex[step.Loop.Goto]
				continue
			}

			// Passed or exhausted retries.
			result.Steps = append(result.Steps, StepResult{
				Name:   step.Name,
				Output: output,
				Passed: passed,
			})
		} else {
			notifyDone(opts.OnStepDone, step.Name, true, output)
			result.Steps = append(result.Steps, StepResult{
				Name:   step.Name,
				Output: output,
				Passed: true,
			})
		}

		pc++
	}

	return result, nil
}
