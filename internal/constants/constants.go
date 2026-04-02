// Package constants defines shared constant values used throughout DevPit.
package constants

import "time"

// Timing constants for session management and tmux operations.
const (
	// DefaultDebounceMs is the default debounce for SendKeys operations.
	DefaultDebounceMs = 500

	// DefaultDisplayMs is the default duration for tmux display-message.
	DefaultDisplayMs = 5000

	// PollInterval is the default polling interval for wait loops.
	PollInterval = 100 * time.Millisecond

	// NudgeReadyTimeout is how long NudgeSession waits for the target pane to
	// accept input before giving up.
	NudgeReadyTimeout = 10 * time.Second

	// NudgeRetryInterval is the base interval between send-keys retry attempts.
	NudgeRetryInterval = 500 * time.Millisecond

	// DialogPollInterval is the interval between pane content checks when
	// polling for startup dialogs (workspace trust, bypass permissions).
	DialogPollInterval = 500 * time.Millisecond

	// DialogPollTimeout is how long to poll for startup dialogs before giving up.
	DialogPollTimeout = 8 * time.Second
)

// Directory names.
const (
	// DirRuntime is the runtime state directory (gitignored).
	DirRuntime = ".runtime"
)

// Agent role names (used by config/env.go for environment variable injection).
const (
	RoleMayor    = "mayor"
	RoleWitness  = "witness"
	RoleRefinery = "refinery"
	RolePolecat  = "polecat"
	RoleCrew     = "crew"
	RoleDeacon   = "deacon"
)

// SupportedShells lists shell binaries that can be detected in tmux panes.
var SupportedShells = []string{"bash", "zsh", "sh", "fish", "tcsh", "ksh", "pwsh", "powershell"}
