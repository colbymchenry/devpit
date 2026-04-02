package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
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
func WaitForCompletion(t *tmux.Tmux, session string, timeout time.Duration) error {
	// Phase A: Wait for busy signal or early idle
	busyDeadline := time.Now().Add(BusyWaitTimeout)
	for time.Now().Before(busyDeadline) {
		lines, err := t.CapturePaneLines(session, 5)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		for _, line := range lines {
			if strings.Contains(strings.TrimSpace(line), "esc to interrupt") {
				// Agent is working — move to Phase B
				goto phaseB
			}
		}
		// Check if already idle (very fast task)
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			normalized := strings.ReplaceAll(trimmed, "\u00a0", " ")
			if strings.HasPrefix(normalized, "❯ ") || normalized == "❯" {
				return nil // Already done
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

phaseB:
	// Phase B: Standard WaitForIdle — 2 consecutive idle polls
	return t.WaitForIdle(session, timeout)
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

// SaveArtifact writes agent output to .pipeline/<name>.md
func SaveArtifact(projectDir, name, content string) error {
	dir := filepath.Join(projectDir, ArtifactDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, name+".md")
	return os.WriteFile(path, []byte(content), 0o644)
}
