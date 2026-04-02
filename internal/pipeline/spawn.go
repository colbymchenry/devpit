package pipeline

import (
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/colbymchenry/devpit/internal/config"
	"github.com/colbymchenry/devpit/internal/tmux"
)

// SpawnOptions controls how an agent session is created.
type SpawnOptions struct {
	// AgentPreset overrides the default AI runtime (e.g., "claude", "gemini", "codex").
	// Empty string uses the default agent.
	AgentPreset string

	// StepTimeout is how long to wait for the agent to finish.
	StepTimeout time.Duration
}

// SpawnAgent creates a tmux session for a pipeline agent, accepts startup dialogs,
// and waits for the runtime to be ready. Returns the session name.
//
// The session is created with the agent's preset command (claude, gemini, codex, etc.)
// and the prompt is passed according to the preset's PromptMode.
func SpawnAgent(t *tmux.Tmux, name, workDir, prompt string, opts SpawnOptions) (string, error) {
	session := SessionPrefix + name

	// Kill any existing session with this name
	_ = t.KillSession(session)

	// Resolve runtime from gastown's agent preset system
	presetName := opts.AgentPreset
	if presetName == "" {
		presetName = "claude" // Default
	}
	rc := config.RuntimeConfigFromPreset(config.AgentPreset(presetName))

	// Build command from preset — handles per-runtime differences:
	//   claude: "claude --dangerously-skip-permissions <prompt>"
	//   gemini: "gemini --approval-mode yolo -i <prompt>"
	//   codex:  "codex --dangerously-bypass-approvals-and-sandbox <prompt>"
	cmd := rc.BuildCommandWithPrompt(prompt)
	command := fmt.Sprintf("exec env PIPELINE_AGENT=%s %s", name, cmd)

	// Create tmux session
	if err := t.NewSessionWithCommand(session, workDir, command); err != nil {
		return "", fmt.Errorf("create session %q: %w", session, err)
	}

	// Set large scrollback for full output capture.
	// tmux.Tmux doesn't expose a SetOption method, so we call tmux directly.
	_ = exec.Command("tmux", "-u", "set-option", "-t", session, "history-limit", strconv.Itoa(ScrollbackLimit)).Run()

	// Accept startup dialogs (workspace trust, bypass permissions, etc.)
	// This handles Claude, Codex, and other runtimes that show trust dialogs.
	if err := t.AcceptStartupDialogs(session); err != nil {
		// Non-fatal — dialog may not appear
		_ = err
	}

	// Wait for runtime to be ready (prompt visible or delay elapsed)
	timeout := opts.StepTimeout
	if timeout == 0 {
		timeout = DefaultStepTimeout
	}
	readyTimeout := 30 * time.Second
	if readyTimeout > timeout {
		readyTimeout = timeout
	}
	if err := t.WaitForRuntimeReady(session, rc, readyTimeout); err != nil {
		// Non-fatal — agent may already be processing
		_ = err
	}

	return session, nil
}

// KillAgent terminates a pipeline agent session and its processes.
func KillAgent(t *tmux.Tmux, session string) {
	_ = t.KillSessionWithProcesses(session)
}
