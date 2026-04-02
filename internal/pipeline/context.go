package pipeline

import (
	"fmt"
	"strings"
)

// BuildPrompt constructs the full prompt for a pipeline step by combining
// the agent's system instructions (from its .md file body), the user's task
// description, and context from prior pipeline steps.
func BuildPrompt(agentBody, task string, priorContext map[string]string) string {
	var b strings.Builder

	// Agent instructions (system prompt from .claude/agents/<name>.md)
	if agentBody != "" {
		b.WriteString(agentBody)
		b.WriteString("\n\n")
	}

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

	// Output instruction — ask agent to write results to file as fallback
	b.WriteString("\n# Output\n\n")
	b.WriteString("Write your complete response to `.pipeline/<your-agent-name>.md` before finishing.\n")
	b.WriteString("End your response with a `## Result` section containing exactly one of: PASS, FAIL, ALL CLEAR, or ISSUES FOUND (whichever applies to your role).\n")

	return b.String()
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
func BuildArchitectPrompt(agentBody, task string) string {
	return BuildPrompt(agentBody, task, nil)
}

// BuildCoderPrompt builds the prompt for the coder step.
func BuildCoderPrompt(agentBody, task, architectPlan string) string {
	ctx := map[string]string{"architect": architectPlan}
	return BuildPrompt(agentBody, task, ctx)
}

// BuildTesterPrompt builds the prompt for the tester step.
func BuildTesterPrompt(agentBody, task, coderOutput string) string {
	ctx := map[string]string{"coder": coderOutput}
	return BuildPrompt(agentBody, task, ctx)
}

// BuildCoderRetryPrompt builds the prompt for a coder retry after test failures.
func BuildCoderRetryPrompt(agentBody, task, architectPlan, testFailures string, attempt int) string {
	ctx := map[string]string{
		"architect": architectPlan,
		"tester":    fmt.Sprintf("Test failures (attempt %d of %d):\n\n%s", attempt, DefaultMaxRetries, testFailures),
	}
	return BuildPrompt(agentBody, task, ctx)
}

// BuildCoderDesignFixPrompt builds the prompt for a coder retry after design-qa issues.
func BuildCoderDesignFixPrompt(agentBody, task, architectPlan, designIssues string, attempt int) string {
	ctx := map[string]string{
		"architect": architectPlan,
		"design-qa": fmt.Sprintf("Visual issues found (attempt %d of %d):\n\n%s", attempt, DefaultMaxRetries, designIssues),
	}
	return BuildPrompt(agentBody, task, ctx)
}

// BuildReviewerPrompt builds the prompt for the reviewer step.
func BuildReviewerPrompt(agentBody, task, architectPlan, coderOutput, testerOutput string) string {
	ctx := map[string]string{
		"architect": architectPlan,
		"coder":     coderOutput,
		"tester":    testerOutput,
	}
	return BuildPrompt(agentBody, task, ctx)
}

// BuildDesignQAPrompt builds the prompt for the design-qa step.
func BuildDesignQAPrompt(agentBody, task, coderOutput, reviewerOutput string) string {
	ctx := map[string]string{
		"coder":    coderOutput,
		"reviewer": reviewerOutput,
	}
	return BuildPrompt(agentBody, task, ctx)
}
