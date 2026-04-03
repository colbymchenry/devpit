package pipeline

import (
	"fmt"
	"os/exec"
	"strconv"
	"time"

	"github.com/colbymchenry/devpit/internal/config"
	"github.com/colbymchenry/devpit/internal/tmux"
	"github.com/google/uuid"
)

// SpawnOptions controls how an agent session is created.
type SpawnOptions struct {
	// AgentPreset overrides the default AI runtime (e.g., "claude", "gemini", "codex").
	// Empty string uses the default agent.
	AgentPreset string

	// StepTimeout is how long to wait for the agent to finish.
	StepTimeout time.Duration

	// Model overrides the model for this step (e.g., "opus[1m]", "sonnet").
	// Empty string uses the preset default.
	Model string

	// Effort sets the effort level for this step (e.g., "low", "medium", "high", "max").
	// Empty string uses the preset default.
	Effort string

	// SessionID passes --session-id <uuid> on the first pipeline run so we
	// control the Claude Code session UUID for later resume.
	SessionID string

	// ResumeID passes --resume <uuid> on follow-up runs to load the agent's
	// prior conversation history from disk.
	ResumeID string
}

// SpawnAgent creates a tmux session for a pipeline agent, accepts startup dialogs,
// and waits for the runtime to be ready. Returns the session name.
//
// The session is created with the agent's preset command (claude, gemini, codex, etc.)
// and the prompt is passed according to the preset's PromptMode.
//
// For Claude, the --agent flag loads .claude/agents/<name>.md as the system prompt.
// This gives the agent proper system-level authority, its own memory, and the full
// agent lifecycle (tools, permissions, etc.) defined in the agent file.
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
	//   claude: "claude --dangerously-skip-permissions --agent <name> <prompt>"
	//   gemini: "gemini --approval-mode yolo -i <prompt>"
	//   codex:  "codex --dangerously-bypass-approvals-and-sandbox <prompt>"
	cmd := rc.BuildCommandWithPrompt(prompt)

	// For Claude, load the agent's .md file via --agent so it becomes the
	// system prompt with proper authority, memory, tools, and permissions.
	if rc.Command == "claude" && AgentExists(workDir, name) {
		cmd += " --agent " + name
	}

	// For Claude, append --model and --effort when specified.
	// These act as defaults — the agent .md frontmatter can still override.
	if rc.Command == "claude" {
		if opts.Model != "" {
			cmd += " --model " + opts.Model
		}
		if opts.Effort != "" {
			cmd += " --effort " + opts.Effort
		}
	}

	// For Claude, pass --session-id on first run (deterministic session tracking)
	// or --resume on follow-up runs (load prior conversation history from disk).
	if rc.Command == "claude" {
		if opts.ResumeID != "" {
			cmd += " --resume " + opts.ResumeID
		} else if opts.SessionID != "" {
			cmd += " --session-id " + opts.SessionID
		}
	}

	command := fmt.Sprintf("exec env PIPELINE_AGENT=%s %s", name, cmd)

	// Create tmux session
	if err := t.NewSessionWithCommand(session, workDir, command); err != nil {
		return "", fmt.Errorf("create session %q: %w", session, err)
	}

	// Set large scrollback for full output capture.
	// tmux.Tmux doesn't expose a SetOption method, so we call tmux directly.
	_ = exec.Command("tmux", "-u", "set-option", "-t", session, "history-limit", strconv.Itoa(ScrollbackLimit)).Run()

	// Set session-level environment variables via tmux set-environment.
	// These persist for the entire tmux session lifetime and are inherited
	// by child processes — critical for agent runtimes (like Claude Code)
	// that restart internally during extended thinking. Without these,
	// the restarted process loses its session identity and crashes.
	runID := uuid.New().String()
	_ = t.SetEnvironment(session, "PIPELINE_AGENT", name)
	_ = t.SetEnvironment(session, "PIPELINE_RUN", runID)
	if rc.Session != nil && rc.Session.SessionIDEnv != "" {
		_ = t.SetEnvironment(session, rc.Session.SessionIDEnv, session)
	}
	if rc.Session != nil && rc.Session.ConfigDirEnv != "" {
		_ = t.SetEnvironment(session, rc.Session.ConfigDirEnv, workDir+"/.claude")
	}

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
