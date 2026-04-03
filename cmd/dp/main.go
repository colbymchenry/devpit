// dp is the DevPit CLI — a sequential multi-agent pipeline for AI coding agents.
//
// It spawns specialized agents in tmux sessions, passing context between steps.
// Supports the default pipeline (architect → coder → tester → reviewer → design-qa)
// and custom workflows defined in YAML.
//
// Usage:
//
//	dp pipeline "Add a health check endpoint"
//	dp create --default
//	dp create "benchmark loop that tests and improves"
//	dp pipeline agent architect "Design a caching layer"
//	dp pipeline status
//	dp pipeline peek coder
package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/colbymchenry/devpit/internal/createpipeline"
	"github.com/colbymchenry/devpit/internal/pipeline"
	"github.com/colbymchenry/devpit/internal/tmux"
	"github.com/colbymchenry/devpit/internal/tui"
)

var version = "dev"

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// --- Root ---

var rootCmd = &cobra.Command{
	Use:   "dp",
	Short: "DevPit — sequential agent pipeline CLI",
	Long: `DevPit runs specialized AI agents in sequence on a task.

Supports the default workflow (architect → coder → tester → reviewer → design-qa)
and custom workflows with loops and exit conditions.

Create a workflow:  dp create
Run a pipeline:     dp pipeline "your task"
Custom workflow:    dp pipeline "your task" --workflow name`,
	Version: version,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip tmux check for commands that don't need it (help, version, completion, root TUI).
		if cmd.Name() == "help" || cmd.Name() == "version" || cmd.Name() == "completion" || cmd.Name() == "dp" {
			return nil
		}
		if _, err := exec.LookPath("tmux"); err != nil {
			fmt.Fprintln(os.Stderr, "tmux is required but not installed.")
			fmt.Fprintln(os.Stderr)
			switch runtime.GOOS {
			case "darwin":
				fmt.Fprintln(os.Stderr, "  brew install tmux")
			case "linux":
				fmt.Fprintln(os.Stderr, "  Ubuntu/Debian:  sudo apt install tmux")
				fmt.Fprintln(os.Stderr, "  Fedora/RHEL:    sudo dnf install tmux")
				fmt.Fprintln(os.Stderr, "  Arch:           sudo pacman -S tmux")
			default:
				fmt.Fprintln(os.Stderr, "  Install tmux for your platform: https://github.com/tmux/tmux")
			}
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Install tmux and try again.")
			os.Exit(1)
		}
		return nil
	},
}

// --- Pipeline flags ---

var (
	flagAgent      string
	flagModel      string
	flagTimeout    time.Duration
	flagMaxRetries int
	flagSkipReview bool
	flagSkipQA     bool
	flagWorkflow   string
	flagDefault    bool
	flagDetach     bool
	flagPeekLines  int
)

func init() {
	// dp pipeline
	rootCmd.AddCommand(pipelineCmd)
	pipelineCmd.Flags().StringVar(&flagAgent, "agent", "", "AI runtime preset (claude, gemini, codex, etc.)")
	pipelineCmd.Flags().StringVar(&flagModel, "model", "", "Model override (e.g., opus[1m], sonnet); defaults to opus[1m]")
	pipelineCmd.Flags().DurationVar(&flagTimeout, "timeout", pipeline.DefaultStepTimeout, "Max time per pipeline step")
	pipelineCmd.Flags().IntVar(&flagMaxRetries, "retries", pipeline.DefaultMaxRetries, "Max coder↔tester/design-qa retries")
	pipelineCmd.Flags().BoolVar(&flagSkipReview, "skip-review", false, "Skip the reviewer step")
	pipelineCmd.Flags().BoolVar(&flagSkipQA, "skip-qa", false, "Skip the design-qa step")
	pipelineCmd.Flags().StringVar(&flagWorkflow, "workflow", "", "Custom workflow file or name (from .claude/workflows/)")

	// dp pipeline agent
	pipelineCmd.AddCommand(agentCmd)
	agentCmd.Flags().BoolVar(&flagDetach, "detach", false, "Spawn without attaching")
	agentCmd.Flags().StringVar(&flagAgent, "agent", "", "AI runtime preset")
	agentCmd.Flags().StringVar(&flagModel, "model", "", "Model override (e.g., opus[1m], sonnet)")

	// dp pipeline status
	pipelineCmd.AddCommand(statusCmd)

	// dp pipeline stop
	pipelineCmd.AddCommand(stopCmd)

	// dp pipeline follow
	pipelineCmd.AddCommand(followCmd)

	// dp pipeline queue
	pipelineCmd.AddCommand(queueCmd)

	// dp pipeline peek
	pipelineCmd.AddCommand(peekCmd)
	peekCmd.Flags().IntVarP(&flagPeekLines, "lines", "n", 50, "Number of lines to capture")

	// dp create
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().BoolVar(&flagDefault, "default", false, "Use the standard workflow template (architect, coder, tester, reviewer)")
	createCmd.Flags().StringVar(&flagAgent, "agent", "", "AI runtime preset (default: claude)")
}

// --- dp pipeline ---

var pipelineCmd = &cobra.Command{
	Use:   "pipeline <task>",
	Short: "Run sequential agent pipeline on a task",
	Long: `Run a sequential agent pipeline on a task.

By default, runs: architect → coder → tester → reviewer → design-qa.
Use --workflow to run a custom workflow defined in .claude/workflows/.

Custom workflows support arbitrary step sequences, context passing between
steps, and loop-back conditions with configurable pass/fail markers.

Examples:
  dp pipeline "Add a health check endpoint"
  dp pipeline "Fix the login form validation" --retries 2
  dp pipeline "Refactor auth module" --skip-qa --agent gemini
  dp pipeline "Optimize performance" --workflow optimize`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := args[0]
		projectDir, err := os.Getwd()
		if err != nil {
			return err
		}

		// Resolve workflow.
		var wf *pipeline.WorkflowConfig
		if flagWorkflow != "" {
			wfPath, err := pipeline.FindWorkflow(projectDir, flagWorkflow)
			if err != nil {
				return err
			}
			wf, err = pipeline.LoadWorkflow(wfPath)
			if err != nil {
				return err
			}
		} else {
			// Default pipeline — load from .claude/workflows/default.yaml.
			wf, err = pipeline.LoadDefaultWorkflow(projectDir)
			if err != nil {
				return err
			}
		}

		fmt.Fprintf(os.Stderr, "Pipeline: %s\n", strings.Join(wf.StepNames(), " → "))
		fmt.Fprintf(os.Stderr, "Task: %s\n\n", task)

		// Generate and save session IDs for resume capability.
		sessions := pipeline.GenerateSessionMap(
			time.Now().Format("20060102T150405"),
			task, flagAgent, wf.AgentNames(),
		)
		if err := pipeline.SaveSessionMap(projectDir, sessions); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save session map: %v\n", err)
		}

		opts := pipeline.PipelineOpts{
			Task:        task,
			ProjectDir:  projectDir,
			AgentPreset: flagAgent,
			Model:       flagModel,
			StepTimeout: flagTimeout,
			MaxRetries:  flagMaxRetries,
			SkipReview:  flagSkipReview,
			SkipQA:      flagSkipQA,
			SessionMap:  sessions,
			OnStepStart: func(step string, attempt int) {
				if attempt > 1 {
					fmt.Fprintf(os.Stderr, "→ %s (attempt %d)\n", step, attempt)
				} else {
					fmt.Fprintf(os.Stderr, "→ %s\n", step)
				}
			},
			OnStepDone: func(step string, passed bool, output string) {
				if passed {
					fmt.Fprintf(os.Stderr, "  done\n")
				} else {
					fmt.Fprintf(os.Stderr, "  failed\n")
				}
			},
		}

		result, err := pipeline.RunWorkflow(wf, opts)
		if err != nil {
			return err
		}

		fmt.Print(pipeline.FormatSummary(result))
		return nil
	},
}

// --- dp pipeline agent ---

var agentCmd = &cobra.Command{
	Use:   "agent <name> <prompt>",
	Short: "Run a single agent interactively in tmux",
	Long: `Spawn a single pipeline agent and attach to it.

Examples:
  dp pipeline agent architect "Design a caching layer"
  dp pipeline agent coder "Implement the plan" --detach`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, prompt := args[0], args[1]
		projectDir, err := os.Getwd()
		if err != nil {
			return err
		}

		if !pipeline.AgentExists(projectDir, name) {
			return fmt.Errorf("agent %q not found (run dp create first)", name)
		}

		fullPrompt := pipeline.BuildPrompt(name, prompt, nil)
		t := tmux.NewTmux()
		model := flagModel
		if model == "" {
			model = pipeline.DefaultModel
		}
		session, err := pipeline.SpawnAgent(t, name, projectDir, fullPrompt, pipeline.SpawnOptions{
			AgentPreset: flagAgent,
			Model:       model,
			Effort:      pipeline.DefaultEffort[name],
		})
		if err != nil {
			return err
		}

		if flagDetach {
			fmt.Fprintf(os.Stderr, "Agent %q running in session %q\n", name, session)
			fmt.Fprintf(os.Stderr, "  peek:   dp pipeline peek %s\n", name)
			fmt.Fprintf(os.Stderr, "  attach: tmux attach -t %s\n", session)
			return nil
		}

		tmuxPath, err := exec.LookPath("tmux")
		if err != nil {
			return fmt.Errorf("tmux not found: %w", err)
		}
		return syscall.Exec(tmuxPath, []string{"tmux", "-u", "attach-session", "-t", session}, os.Environ()) //nolint:gosec // tmuxPath is from exec.LookPath
	},
}

// --- dp pipeline status ---

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show running pipeline agent sessions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		t := tmux.NewTmux()
		sessions, err := t.ListSessions()
		if err != nil {
			return err
		}

		var found bool
		for _, session := range sessions {
			if !strings.HasPrefix(session, pipeline.SessionPrefix) {
				continue
			}
			found = true
			agentName := strings.TrimPrefix(session, pipeline.SessionPrefix)

			state := "working"
			if t.IsIdle(session) {
				state = "idle"
			}

			lines, _ := t.CapturePaneLines(session, 5)
			lastLine := ""
			for i := len(lines) - 1; i >= 0; i-- {
				if trimmed := strings.TrimSpace(lines[i]); trimmed != "" {
					lastLine = trimmed
					break
				}
			}
			if len(lastLine) > 80 {
				lastLine = lastLine[:77] + "..."
			}

			fmt.Printf("  %-12s  %-8s  %s\n", agentName, state, lastLine)
		}

		if !found {
			fmt.Println("No pipeline sessions running.")
		}
		return nil
	},
}

// --- dp pipeline stop ---

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop all running pipeline agent sessions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		t := tmux.NewTmux()
		sessions, err := t.ListSessions()
		if err != nil {
			fmt.Println("No pipeline sessions running.")
			return nil
		}

		var killed int
		for _, session := range sessions {
			if !strings.HasPrefix(session, pipeline.SessionPrefix) {
				continue
			}
			agentName := strings.TrimPrefix(session, pipeline.SessionPrefix)
			if err := t.KillSessionWithProcesses(session); err != nil {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", agentName, err)
			} else {
				fmt.Printf("  ✓ stopped %s\n", agentName)
				killed++
			}
		}

		if killed == 0 {
			fmt.Println("No pipeline sessions running.")
		} else {
			fmt.Printf("\nStopped %d session(s).\n", killed)
		}

		// Clear queue and session tracking
		projectDir, _ := os.Getwd()
		if projectDir != "" {
			if err := pipeline.ClearQueue(projectDir); err == nil {
				fmt.Println("  Queue cleared.")
			}
			if err := pipeline.DeleteSessionMap(projectDir); err == nil {
				fmt.Println("  Session map cleared.")
			}
		}
		return nil
	},
}

// --- dp pipeline follow ---

var followCmd = &cobra.Command{
	Use:   "follow <task>",
	Short: "Queue a follow-up task for the current pipeline",
	Long: `Queue a follow-up prompt that runs through the same pipeline agents,
resuming their conversations with --resume to preserve full context.

The follow-up waits in a queue and runs when the pipeline is idle.

Examples:
  dp pipeline follow "Make the button blue instead of green"
  dp pipeline follow "Also add error handling for edge cases"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		task := args[0]
		projectDir, err := os.Getwd()
		if err != nil {
			return err
		}

		// Verify sessions.json exists (pipeline must have run at least once)
		sessions, err := pipeline.LoadSessionMap(projectDir)
		if err != nil {
			return fmt.Errorf("load session map: %w", err)
		}
		if sessions == nil {
			return fmt.Errorf("no pipeline session found — run 'dp pipeline' first")
		}

		// Enqueue the follow-up
		item, err := pipeline.EnqueueFollowUp(projectDir, task)
		if err != nil {
			return fmt.Errorf("enqueue: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Queued follow-up: %s\n", item.Task)

		count, _ := pipeline.PendingCount(projectDir)
		fmt.Fprintf(os.Stderr, "Queue depth: %d\n", count)

		// Try to become the watcher (non-blocking flock).
		// If another process holds the lock, we're done — it will pick up our item.
		became := pipeline.WatchAndProcess(pipeline.WatcherOpts{
			ProjectDir:  projectDir,
			AgentPreset: sessions.AgentPreset,
			Model:       flagModel,
			StepTimeout: flagTimeout,
			MaxRetries:  flagMaxRetries,
			SkipReview:  flagSkipReview,
			SkipQA:      flagSkipQA,
			OnStepStart: func(step string, attempt int) {
				if attempt > 1 {
					fmt.Fprintf(os.Stderr, "  → %s (attempt %d)\n", step, attempt)
				} else {
					fmt.Fprintf(os.Stderr, "  → %s\n", step)
				}
			},
			OnStepDone: func(step string, passed bool, _ string) {
				if passed {
					fmt.Fprintf(os.Stderr, "    done\n")
				} else {
					fmt.Fprintf(os.Stderr, "    failed\n")
				}
			},
			OnItemStart: func(item *pipeline.QueueItem) {
				fmt.Fprintf(os.Stderr, "\n═══ Follow-up: %s ═══\n\n", item.Task)
			},
			OnItemDone: func(item *pipeline.QueueItem, err error) {
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Follow-up failed: %v\n", err)
				} else {
					fmt.Fprintf(os.Stderr, "  Follow-up complete.\n")
				}
			},
		})

		if became {
			fmt.Fprintf(os.Stderr, "Watcher finished — queue empty.\n")
		} else {
			fmt.Fprintf(os.Stderr, "Watcher already running — it will pick up this item.\n")
		}

		return nil
	},
}

// --- dp pipeline queue ---

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Show the follow-up queue",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir, err := os.Getwd()
		if err != nil {
			return err
		}

		q, err := pipeline.LoadQueue(projectDir)
		if err != nil {
			return err
		}

		if len(q.Items) == 0 {
			fmt.Println("Queue is empty.")
			return nil
		}

		fmt.Printf("  %-16s  %-8s  %s\n", "ID", "STATUS", "TASK")
		fmt.Printf("  %-16s  %-8s  %s\n", "────────────────", "────────", "────────────────────────────────")
		for _, item := range q.Items {
			task := item.Task
			if len(task) > 50 {
				task = task[:47] + "..."
			}
			fmt.Printf("  %-16s  %-8s  %s\n", item.ID, item.Status, task)
		}
		return nil
	},
}

// --- dp pipeline peek ---

var peekCmd = &cobra.Command{
	Use:   "peek <agent> [count]",
	Short: "View recent output from a pipeline agent",
	Long: `Examples:
  dp pipeline peek architect
  dp pipeline peek coder 100
  dp pipeline peek tester -n 200`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		session := pipeline.SessionPrefix + agentName

		lines := flagPeekLines
		if len(args) > 1 {
			n, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid line count: %s", args[1])
			}
			lines = n
		}

		t := tmux.NewTmux()
		has, err := t.HasSession(session)
		if err != nil || !has {
			return fmt.Errorf("no active session for agent %q", agentName)
		}

		output, err := t.CapturePane(session, lines)
		if err != nil {
			return err
		}

		fmt.Print(output)
		return nil
	},
}

// --- dp create ---

var createCmd = &cobra.Command{
	Use:   "create [prompt]",
	Short: "Create a new workflow interactively with Claude",
	Long: `Create a new workflow by describing what you want. Claude interviews you
to design the workflow and generates all necessary files.

With no arguments, opens the TUI create form.
With a prompt, spawns a Claude session directly.
With --default, creates the standard workflow (architect, coder, tester, reviewer).

Examples:
  dp create                                           # TUI create form
  dp create "benchmark loop that tests and improves"  # Custom workflow
  dp create --default                                 # Standard workflow`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir, err := os.Getwd()
		if err != nil {
			return err
		}

		// No args and no --default: launch TUI at create view.
		if len(args) == 0 && !flagDefault {
			return tui.RunAtView("create")
		}

		// Direct mode: spawn Claude session.
		prompt := ""
		if len(args) > 0 {
			prompt = args[0]
		}

		session, err := createpipeline.SpawnSession(projectDir, prompt, flagAgent, flagDefault)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stderr, "Creating workflow in session %q...\n", session)
		fmt.Fprintf(os.Stderr, "Claude will interview you to design your workflow.\n\n")

		tmuxPath, err := exec.LookPath("tmux")
		if err != nil {
			return fmt.Errorf("tmux not found: %w", err)
		}
		return syscall.Exec(tmuxPath, []string{"tmux", "-u", "attach-session", "-t", session}, os.Environ()) //nolint:gosec // tmuxPath is from exec.LookPath
	},
}
