package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/colbymchenry/devpit/internal/tmux"
)

// WaitForCompletion waits for an agent to finish processing.
//
// Phase A: Wait up to BusyWaitTimeout for the agent to become busy (shows
// "esc to interrupt"). This prevents false idle detection before the agent
// starts processing its prompt.
//
// Phase B: Use tmux.WaitForIdle with 2-consecutive-idle-poll algorithm to
// detect genuine completion.
// crashIndicators are strings that signal the agent runtime has crashed.
// When found in the pane during polling, WaitForCompletion returns immediately
// with an error instead of waiting for idle detection.
var crashIndicators = []string{
	"Resume this session with",
	"Session expired",
	"SIGTERM",
	"panic:",
}

// ErrAgentCrashed indicates the agent runtime exited unexpectedly.
var ErrAgentCrashed = fmt.Errorf("agent crashed")

func WaitForCompletion(t *tmux.Tmux, session string, timeout time.Duration) error {
	// Phase A: Wait for the agent to become busy.
	// We MUST see a busy signal before treating an idle prompt as "done,"
	// because the ❯ prompt is visible in the pane before the agent starts
	// processing — returning early on that initial ❯ is a race condition.
	//
	// Busy signals:
	//   - "esc to interrupt" in status bar (tool execution, response streaming)
	//   - Extended thinking indicators (e.g., "thinking with high effort")
	//     Claude Code shows ❯ even during extended thinking, but does NOT
	//     show "esc to interrupt" — only a thinking spinner is visible.
	busyDeadline := time.Now().Add(BusyWaitTimeout)
	for time.Now().Before(busyDeadline) {
		lines, err := t.CapturePaneLines(session, 30)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		// Check busy indicators only in the status area (last ~8 lines)
		// to avoid false positives from spinner text in scrollback.
		busyWindow := lines
		if len(busyWindow) > 8 {
			busyWindow = busyWindow[len(busyWindow)-8:]
		}
		if isBusy(busyWindow) {
			goto phaseB
		}
		// Check for crash even before busy signal appears
		if msg := checkCrashIndicators(lines); msg != "" {
			return fmt.Errorf("%w: %s", ErrAgentCrashed, msg)
		}
		time.Sleep(200 * time.Millisecond)
	}

phaseB:
	// Phase B: Poll for idle with crash detection.
	// We inline the idle-check logic (rather than delegating to WaitForIdle)
	// so we can also detect crashes immediately instead of waiting for timeout.
	deadline := time.Now().Add(timeout)
	consecutiveIdle := 0
	const requiredConsecutive = 2

	for time.Now().Before(deadline) {
		lines, err := t.CapturePaneLines(session, 30)
		if err != nil {
			consecutiveIdle = 0
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Crash detection — scan all 30 lines
		if msg := checkCrashIndicators(lines); msg != "" {
			return fmt.Errorf("%w: %s", ErrAgentCrashed, msg)
		}

		// Busy detection — only check the last ~8 lines (status area).
		// Checking all 30 lines causes false positives: spinner lines from
		// completed tool calls (e.g., "✱ Booping…") persist in scrollback
		// and isBusy would see them as active thinking, blocking forever.
		// This matches tmux.IsIdle which only checks 5 lines.
		busyWindow := lines
		if len(busyWindow) > 8 {
			busyWindow = busyWindow[len(busyWindow)-8:]
		}
		if isBusy(busyWindow) {
			consecutiveIdle = 0
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Prompt detection — agent may be idle
		promptFound := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			normalized := strings.ReplaceAll(trimmed, "\u00a0", " ")
			if strings.HasPrefix(normalized, "❯ ") || normalized == "❯" {
				promptFound = true
				break
			}
		}

		if promptFound {
			consecutiveIdle++
			if consecutiveIdle >= requiredConsecutive {
				return nil
			}
		} else {
			consecutiveIdle = 0
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for agent to finish")
}

// spinnerLineRe matches Claude Code's thinking spinner format:
//
//	<glyph> <CapitalizedVerb>…
//
// Examples: "✱ Cooking…", "+ Frosting…", "· Thinking… (42s · thinking)"
//
// This matches the structural pattern rather than specific glyph characters,
// which vary across terminals (Ghostty uses *, some render +) and Claude Code
// versions. The verb is always a single capitalized word from spinnerVerbs.ts
// followed by a horizontal ellipsis (U+2026).
var spinnerLineRe = regexp.MustCompile(`^\S\s[A-Z][a-zA-Z]+\x{2026}`)

// isSpinnerLine returns true if the line matches Claude Code's thinking spinner format.
func isSpinnerLine(line string) bool {
	if strings.HasPrefix(line, "❯") {
		return false
	}
	return spinnerLineRe.MatchString(line)
}

// isBusy checks if the agent is actively working. Returns true if the pane
// shows any busy signal:
//   - "esc to interrupt" — visible during tool execution and response streaming
//   - Spinner line — Claude Code is thinking/processing. The format is always
//     "<glyph> <Verb>…" (e.g., "+ Frosting…", "✳ Perambulating…").
//     During thinking, "esc to interrupt" is NOT shown but ❯ IS visible,
//     which would cause false idle detection without this check.
func isBusy(lines []string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "esc to interrupt") {
			return true
		}
		if isSpinnerLine(trimmed) {
			return true
		}
	}
	return false
}

// checkCrashIndicators scans pane lines for crash signals.
// Returns the matched indicator message, or empty string if none found.
func checkCrashIndicators(lines []string) string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, indicator := range crashIndicators {
			if strings.Contains(trimmed, indicator) {
				return indicator
			}
		}
	}
	return ""
}

// ExtractOutput captures the full tmux scrollback and extracts the agent's
// last response. Returns the raw captured text.
//
// Falls back to reading .pipeline/<agent>.md if the file exists (agents are
// instructed to write their output there).
func ExtractOutput(t *tmux.Tmux, session, projectDir, agentName string) (string, error) {
	// Check file-based fallback first
	artifactPath := filepath.Join(projectDir, ArtifactDir, agentName+".md")
	if data, err := os.ReadFile(artifactPath); err == nil && len(data) > 0 {
		return string(data), nil
	}

	// Capture full scrollback
	raw, err := t.CapturePaneAll(session)
	if err != nil {
		return "", fmt.Errorf("capture pane for %q: %w", session, err)
	}

	// Always save the raw pane capture for debugging — if something goes
	// wrong, the user can inspect .pipeline/<agent>-raw.md for the full
	// tmux session output including startup errors, dialogs, etc.
	_ = SaveArtifact(projectDir, agentName+"-raw", raw)

	return parseLastResponse(raw), nil
}

// parseLastResponse extracts the agent's final response from raw tmux scrollback.
// Scans backward from the end to find the content between the last two prompt
// markers (❯), which brackets the agent's response.
func parseLastResponse(raw string) string {
	lines := strings.Split(raw, "\n")

	// Find the idle prompt at the end (scanning backward)
	endIdx := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if isPromptLine(lines[i]) {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		// No prompt found — return everything
		return strings.TrimSpace(raw)
	}

	// Scan backward from there to find the previous prompt (start of this response)
	startIdx := 0
	for i := endIdx - 1; i >= 0; i-- {
		if isPromptLine(lines[i]) {
			startIdx = i + 1
			break
		}
	}

	// Extract the response block, skip empty leading/trailing lines
	response := strings.Join(lines[startIdx:endIdx], "\n")
	return strings.TrimSpace(response)
}

// isPromptLine checks if a line looks like a Claude Code / agent prompt.
func isPromptLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	normalized := strings.ReplaceAll(trimmed, "\u00a0", " ")
	return strings.HasPrefix(normalized, "❯ ") || normalized == "❯"
}

// ParseResult checks agent output for pass/fail markers.
// Returns true if the output contains a passing result.
func ParseResult(output string, passMarker, failMarker string) bool {
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if strings.EqualFold(trimmed, passMarker) {
			return true
		}
		if strings.EqualFold(trimmed, failMarker) {
			return false
		}
		// Also check "## Result" section format
		if strings.HasPrefix(trimmed, "## Result") && i+1 < len(lines) {
			nextLine := strings.TrimSpace(lines[i+1])
			if strings.EqualFold(nextLine, passMarker) {
				return true
			}
			return false
		}
	}
	// No explicit marker found — assume pass (agent didn't report failure)
	return true
}

// agentErrorPatterns lists patterns in agent output that indicate the runtime
// environment is broken. When detected, the pipeline step fails immediately
// instead of passing garbage to the next step.
var agentErrorPatterns = []struct {
	marker  string
	message string
}{
	{"Settings Error", "Claude Code settings error detected in agent session"},
	{"Resume this session with", "agent crashed — Claude Code exited unexpectedly"},
	{"API Error", "agent encountered an API error"},
	{"Could not connect to MCP server", "MCP server connection failed"},
	{"Authentication required", "agent requires authentication"},
	{"ECONNREFUSED", "agent could not connect to a required service"},
}

// ValidateAgentOutput checks extracted output for known error patterns that
// indicate the agent couldn't run properly. Returns nil if the output looks
// like a real agent response.
//
// When an error is detected, the pipeline should stop — the artifact has
// already been saved so the user can inspect what happened.
func ValidateAgentOutput(agentName, output string) error {
	trimmed := strings.TrimSpace(output)

	if len(trimmed) == 0 {
		return fmt.Errorf("agent %q produced no output — session may have crashed (see %s/%s-raw.md)",
			agentName, ArtifactDir, agentName)
	}

	for _, p := range agentErrorPatterns {
		if strings.Contains(output, p.marker) {
			// Include a preview so the error message is self-contained
			preview := trimmed
			if len(preview) > 500 {
				preview = preview[:500] + "\n..."
			}
			return fmt.Errorf("agent %q: %s — full output in %s/%s.md:\n\n%s",
				agentName, p.message, ArtifactDir, agentName, preview)
		}
	}

	return nil
}

// SaveArtifact writes agent output to .pipeline/<name>.md
func SaveArtifact(projectDir, name, content string) error {
	dir := filepath.Join(projectDir, ArtifactDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, name+".md")
	return os.WriteFile(path, []byte(content), 0o644)
}

// LoadArtifact reads a saved artifact from .pipeline/<name>.md.
// Returns empty string and nil error if the file does not exist.
func LoadArtifact(projectDir, name string) (string, error) {
	path := filepath.Join(projectDir, ArtifactDir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
