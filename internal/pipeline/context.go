package pipeline

import (
	"fmt"
	"strings"
)

// BuildPrompt constructs the user message for a pipeline step by combining
// the task description, context from prior steps, and output directives.
//
// The agent's system instructions come from .claude/agents/<name>.md loaded
// via --agent flag — they are NOT included in this user message.
func BuildPrompt(role, task string, priorContext map[string]string) string {
	var b strings.Builder

	// Task description
	b.WriteString("# Task\n\n")
	b.WriteString(task)
	b.WriteString("\n")

	// Prior context from completed steps
	if len(priorContext) > 0 {
		b.WriteString("\n# Context from Previous Steps\n")
		// Emit in a stable order
		for _, key := range contextOrder {
			if val, ok := priorContext[key]; ok {
				b.WriteString("\n## ")
				b.WriteString(contextLabels[key])
				b.WriteString("\n\n")
				b.WriteString(val)
				b.WriteString("\n")
			}
		}
	}

	// Role-specific output directives
	b.WriteString("\n# Output\n\n")
	if directive, ok := roleDirectives[role]; ok {
		b.WriteString(directive)
	}
	b.WriteString("Write your complete response to `.pipeline/<your-agent-name>.md` before finishing.\n")
	b.WriteString("End your response with a `## Result` section containing exactly one of: PASS, FAIL, ALL CLEAR, or ISSUES FOUND (whichever applies to your role).\n")

	return b.String()
}

// roleDirectives provide role-specific instructions appended before the
// generic output block. These make each agent's primary job unambiguous.
var roleDirectives = map[string]string{
	"architect": "Produce a detailed implementation plan. Do NOT modify any source files.\n\n",
	"coder":     "Implement the architect's plan by editing source files. You MUST make actual code changes — do not just describe what to do. Read the relevant files, edit them, and run the project's linter. If no source files are changed, this step has failed.\n\n",
	"tester":    "Run the project's test suite against the coder's changes. Report test results with specific pass/fail details.\n\n",
	"reviewer":  "Review the code changes for correctness, style, and potential issues. Do NOT modify source files.\n\n",
	"design-qa": "Visually verify the changes match the design intent. Take screenshots and compare against the reference.\n\n",
}

// contextOrder defines the order context sections appear in prompts.
var contextOrder = []string{"architect", "coder", "tester", "reviewer", "design-qa"}

// contextLabels maps agent names to human-readable context section headers.
var contextLabels = map[string]string{
	"architect": "Architect's Plan",
	"coder":     "Implementation Changes",
	"tester":    "Test Results",
	"reviewer":  "Review Feedback",
	"design-qa": "Visual QA Findings",
}

// BuildArchitectPrompt builds the prompt for the architect step.
func BuildArchitectPrompt(task string) string {
	return BuildPrompt("architect", task, nil)
}

// BuildCoderPrompt builds the prompt for the coder step.
func BuildCoderPrompt(task, architectPlan string) string {
	ctx := map[string]string{"architect": architectPlan}
	return BuildPrompt("coder", task, ctx)
}

// BuildTesterPrompt builds the prompt for the tester step.
func BuildTesterPrompt(task, coderOutput string) string {
	ctx := map[string]string{"coder": coderOutput}
	return BuildPrompt("tester", task, ctx)
}

// BuildCoderRetryPrompt builds the prompt for a coder retry after test failures.
func BuildCoderRetryPrompt(task, architectPlan, testFailures string, attempt int) string {
	ctx := map[string]string{
		"architect": architectPlan,
		"tester":    fmt.Sprintf("Test failures (attempt %d of %d):\n\n%s", attempt, DefaultMaxRetries, testFailures),
	}
	return BuildPrompt("coder", task, ctx)
}

// BuildCoderDesignFixPrompt builds the prompt for a coder retry after design-qa issues.
func BuildCoderDesignFixPrompt(task, architectPlan, designIssues string, attempt int) string {
	ctx := map[string]string{
		"architect": architectPlan,
		"design-qa": fmt.Sprintf("Visual issues found (attempt %d of %d):\n\n%s", attempt, DefaultMaxRetries, designIssues),
	}
	return BuildPrompt("coder", task, ctx)
}

// BuildReviewerPrompt builds the prompt for the reviewer step.
func BuildReviewerPrompt(task, architectPlan, coderOutput, testerOutput string) string {
	ctx := map[string]string{
		"architect": architectPlan,
		"coder":     coderOutput,
		"tester":    testerOutput,
	}
	return BuildPrompt("reviewer", task, ctx)
}

// BuildDesignQAPrompt builds the prompt for the design-qa step.
func BuildDesignQAPrompt(task, coderOutput, reviewerOutput string) string {
	ctx := map[string]string{
		"coder":    coderOutput,
		"reviewer": reviewerOutput,
	}
	return BuildPrompt("design-qa", task, ctx)
}

// BuildWorkflowPrompt constructs the user message for a workflow step.
// Unlike BuildPrompt, it handles arbitrary step names by using the step's
// Context field for ordering and falling back to title-cased labels.
func BuildWorkflowPrompt(step StepConfig, task string, priorContext map[string]string) string {
	var b strings.Builder

	b.WriteString("# Task\n\n")
	b.WriteString(task)
	b.WriteString("\n")

	// Prior context in the order declared by step.Context.
	if len(priorContext) > 0 {
		b.WriteString("\n# Context from Previous Steps\n")
		for _, key := range step.Context {
			val, ok := priorContext[key]
			if !ok {
				continue
			}
			b.WriteString("\n## ")
			b.WriteString(contextLabelFor(key))
			b.WriteString("\n\n")
			b.WriteString(val)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n# Output\n\n")
	if step.Directive != "" {
		b.WriteString(step.Directive)
		if !strings.HasSuffix(step.Directive, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	} else if directive, ok := roleDirectives[step.AgentName()]; ok {
		b.WriteString(directive)
	}

	b.WriteString(fmt.Sprintf("Write your complete response to `.pipeline/%s.md` before finishing.\n", step.Name))

	if step.Loop != nil {
		b.WriteString(fmt.Sprintf("End your response with a `## Result` section containing exactly one of: %s or %s.\n",
			step.Loop.PassMarker, step.Loop.FailMarker))
	} else {
		b.WriteString("End your response with a `## Result` section containing exactly one of: PASS, FAIL, ALL CLEAR, or ISSUES FOUND (whichever applies to your role).\n")
	}

	return b.String()
}

// BuildWorkflowFollowUpPrompt builds a follow-up prompt for a workflow step.
func BuildWorkflowFollowUpPrompt(step StepConfig, task string) string {
	var b strings.Builder

	b.WriteString("# Follow-Up Task\n\n")
	b.WriteString("This is a follow-up to your previous work. You already have the full context from the prior conversation.\n\n")
	b.WriteString(task)
	b.WriteString("\n")

	b.WriteString("\n# Output\n\n")
	if step.Directive != "" {
		b.WriteString(step.Directive)
		if !strings.HasSuffix(step.Directive, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	} else if directive, ok := roleDirectives[step.AgentName()]; ok {
		b.WriteString(directive)
	}

	b.WriteString(fmt.Sprintf("Write your complete response to `.pipeline/%s.md` before finishing.\n", step.Name))

	if step.Loop != nil {
		b.WriteString(fmt.Sprintf("End your response with a `## Result` section containing exactly one of: %s or %s.\n",
			step.Loop.PassMarker, step.Loop.FailMarker))
	} else {
		b.WriteString("End your response with a `## Result` section containing exactly one of: PASS, FAIL, ALL CLEAR, or ISSUES FOUND (whichever applies to your role).\n")
	}

	return b.String()
}

// contextLabelFor returns a human-readable label for a step name.
// Uses the built-in contextLabels if available, otherwise title-cases the name.
func contextLabelFor(name string) string {
	if label, ok := contextLabels[name]; ok {
		return label
	}
	// Title-case: "baseline-test" → "Baseline Test's Output"
	words := strings.Split(strings.ReplaceAll(name, "-", " "), " ")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ") + "'s Output"
}

// BuildFollowUpPrompt builds a minimal prompt for follow-up runs.
// The agent already has full conversation history via --resume, so we
// only send the new task without prior context sections.
func BuildFollowUpPrompt(role, task string) string {
	var b strings.Builder

	b.WriteString("# Follow-Up Task\n\n")
	b.WriteString("This is a follow-up to your previous work. You already have the full context from the prior conversation.\n\n")
	b.WriteString(task)
	b.WriteString("\n")

	// Role-specific output directives (same as initial run)
	b.WriteString("\n# Output\n\n")
	if directive, ok := roleDirectives[role]; ok {
		b.WriteString(directive)
	}
	b.WriteString("Write your complete response to `.pipeline/<your-agent-name>.md` before finishing.\n")
	b.WriteString("End your response with a `## Result` section containing exactly one of: PASS, FAIL, ALL CLEAR, or ISSUES FOUND (whichever applies to your role).\n")

	return b.String()
}
