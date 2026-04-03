package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkflowConfig defines a custom pipeline as an ordered list of steps.
type WorkflowConfig struct {
	Name        string       `yaml:"name"`
	Description string       `yaml:"description,omitempty"`
	Steps       []StepConfig `yaml:"steps"`
}

// StepConfig defines a single step in a workflow.
type StepConfig struct {
	// Name is the unique identifier for this step.
	Name string `yaml:"name"`

	// Agent is the agent file name (without .md). Defaults to Name.
	Agent string `yaml:"agent,omitempty"`

	// Context lists prior step names whose outputs are included in the prompt.
	Context []string `yaml:"context,omitempty"`

	// Directive is a per-step instruction added to the prompt's Output section.
	// If empty, falls back to the built-in roleDirectives for known agents.
	Directive string `yaml:"directive,omitempty"`

	// Optional means the step is skipped if its agent file doesn't exist.
	Optional bool `yaml:"optional,omitempty"`

	// Loop defines a loop-back condition on failure.
	Loop *LoopConfig `yaml:"loop,omitempty"`
}

// LoopConfig defines loop-back behavior for a step.
type LoopConfig struct {
	// Goto is the step name to jump back to on failure.
	Goto string `yaml:"goto"`

	// Max is the maximum number of loop iterations (default: DefaultMaxRetries).
	Max int `yaml:"max,omitempty"`

	// PassMarker is the output marker that signals success (e.g., "PASS").
	PassMarker string `yaml:"pass"`

	// FailMarker is the output marker that signals failure (e.g., "FAIL").
	FailMarker string `yaml:"fail"`
}

// AgentName returns the agent file name for this step, defaulting to Name.
func (s StepConfig) AgentName() string {
	if s.Agent != "" {
		return s.Agent
	}
	return s.Name
}

// LoadWorkflow reads and validates a workflow YAML file.
func LoadWorkflow(path string) (*WorkflowConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow %q: %w", path, err)
	}

	var wf WorkflowConfig
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse workflow %q: %w", path, err)
	}

	if err := ValidateWorkflow(&wf); err != nil {
		return nil, fmt.Errorf("invalid workflow %q: %w", path, err)
	}

	return &wf, nil
}

// FindWorkflow resolves a --workflow argument to a file path.
// It checks: exact path, then .claude/workflows/<name>.yaml, then .yml.
func FindWorkflow(projectDir, name string) (string, error) {
	// If it looks like a path, use as-is.
	if strings.Contains(name, "/") || strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
		if _, err := os.Stat(name); err != nil {
			return "", fmt.Errorf("workflow file not found: %s", name)
		}
		return name, nil
	}

	// Try .claude/workflows/<name>.yaml then .yml
	dir := filepath.Join(projectDir, ".claude", "workflows")
	for _, ext := range []string{".yaml", ".yml"} {
		path := filepath.Join(dir, name+ext)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("workflow %q not found (looked in .claude/workflows/)", name)
}

// ValidateWorkflow checks a workflow config for structural errors.
func ValidateWorkflow(wf *WorkflowConfig) error {
	if len(wf.Steps) == 0 {
		return fmt.Errorf("workflow has no steps")
	}

	seen := make(map[string]int, len(wf.Steps)) // name -> index
	for i, step := range wf.Steps {
		if step.Name == "" {
			return fmt.Errorf("step %d has no name", i)
		}

		if _, dup := seen[step.Name]; dup {
			return fmt.Errorf("duplicate step name %q", step.Name)
		}
		seen[step.Name] = i

		// Context refs must point to earlier steps.
		for _, ref := range step.Context {
			refIdx, ok := seen[ref]
			if !ok {
				return fmt.Errorf("step %q: context references unknown step %q", step.Name, ref)
			}
			if refIdx >= i {
				return fmt.Errorf("step %q: context references later step %q", step.Name, ref)
			}
		}

		// Loop validation.
		if step.Loop != nil {
			if step.Loop.Goto == "" {
				return fmt.Errorf("step %q: loop has no goto target", step.Name)
			}
			if step.Loop.Goto == step.Name {
				return fmt.Errorf("step %q: loop goto cannot target itself", step.Name)
			}
			gotoIdx, ok := seen[step.Loop.Goto]
			if !ok {
				return fmt.Errorf("step %q: loop goto references unknown step %q", step.Name, step.Loop.Goto)
			}
			if gotoIdx >= i {
				return fmt.Errorf("step %q: loop goto must point to an earlier step", step.Name)
			}
			if step.Loop.PassMarker == "" || step.Loop.FailMarker == "" {
				return fmt.Errorf("step %q: loop requires both pass and fail markers", step.Name)
			}
		}
	}

	return nil
}

// LoadDefaultWorkflow loads the default workflow YAML from .claude/workflows/.
// Returns an error if no default workflow exists — run 'dp create' first.
func LoadDefaultWorkflow(projectDir string) (*WorkflowConfig, error) {
	wfPath, err := FindWorkflow(projectDir, "default")
	if err != nil {
		return nil, fmt.Errorf("no default workflow found in .claude/workflows/\nRun 'dp create' first")
	}
	return LoadWorkflow(wfPath)
}

// DiscoverWorkflows returns the names of available workflow files in .claude/workflows/.
func DiscoverWorkflows(projectDir string) []string {
	dir := filepath.Join(projectDir, ".claude", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") {
			names = append(names, strings.TrimSuffix(name, ".yaml"))
		} else if strings.HasSuffix(name, ".yml") {
			names = append(names, strings.TrimSuffix(name, ".yml"))
		}
	}
	return names
}


// StepNames returns the step names from a workflow config.
func (wf *WorkflowConfig) StepNames() []string {
	names := make([]string, len(wf.Steps))
	for i, s := range wf.Steps {
		names[i] = s.Name
	}
	return names
}

// AgentNames returns the unique agent names from a workflow config.
func (wf *WorkflowConfig) AgentNames() []string {
	seen := make(map[string]bool)
	var names []string
	for _, s := range wf.Steps {
		agent := s.AgentName()
		if !seen[agent] {
			seen[agent] = true
			names = append(names, agent)
		}
	}
	return names
}
