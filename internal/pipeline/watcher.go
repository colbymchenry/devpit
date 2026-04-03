package pipeline

import (
	"fmt"
	"time"

	"github.com/colbymchenry/devpit/internal/flock"
)

// WatcherOpts configures the queue watcher.
type WatcherOpts struct {
	// ProjectDir is the root directory of the project.
	ProjectDir string

	// AgentPreset overrides the AI runtime (e.g., "claude", "gemini", "codex").
	AgentPreset string

	// Model overrides the model for all steps.
	Model string

	// StepTimeout is the max time per pipeline step.
	StepTimeout time.Duration

	// MaxRetries is the max coder↔tester or coder↔design-qa retry loops.
	MaxRetries int

	// SkipReview skips the reviewer step.
	SkipReview bool

	// SkipQA skips the design-qa step.
	SkipQA bool

	// IdleTimeout is how long to wait with no new items before exiting.
	// Defaults to DefaultWatcherIdleTimeout.
	IdleTimeout time.Duration

	// OnStepStart is called when a pipeline step begins.
	OnStepStart func(step string, attempt int)

	// OnStepDone is called when a pipeline step completes.
	OnStepDone func(step string, passed bool, output string)

	// OnItemStart is called when a queue item begins processing.
	OnItemStart func(item *QueueItem)

	// OnItemDone is called when a queue item finishes processing.
	OnItemDone func(item *QueueItem, err error)
}

// WatchAndProcess acquires the watcher lock (non-blocking) and polls the queue
// for pending follow-up items. Each item runs through the full pipeline with
// IsFollowUp=true so agents are resumed via --resume.
//
// Returns true if this process became the watcher (and ran to completion).
// Returns false immediately if another watcher already holds the lock.
func WatchAndProcess(opts WatcherOpts) bool {
	lockPath := WatcherLockPath(opts.ProjectDir)
	unlock, acquired := flock.TryLock(lockPath)
	if !acquired {
		return false
	}
	defer unlock()

	idleTimeout := opts.IdleTimeout
	if idleTimeout == 0 {
		idleTimeout = DefaultWatcherIdleTimeout
	}

	idleSince := time.Now()

	for {
		item, err := DequeueNext(opts.ProjectDir)
		if err != nil {
			// Queue read error — wait and retry
			time.Sleep(2 * time.Second)
			continue
		}

		if item == nil {
			// No pending items — check idle timeout
			if time.Since(idleSince) > idleTimeout {
				return true
			}
			time.Sleep(2 * time.Second)
			continue
		}

		// Reset idle timer — we have work
		idleSince = time.Now()

		if opts.OnItemStart != nil {
			opts.OnItemStart(item)
		}

		runErr := runFollowUp(opts, item)

		status := "done"
		if runErr != nil {
			status = "failed"
		}
		_ = UpdateItemStatus(opts.ProjectDir, item.ID, status)

		if opts.OnItemDone != nil {
			opts.OnItemDone(item, runErr)
		}

		// Reset idle timer after completing work
		idleSince = time.Now()
	}
}

// runFollowUp executes a single follow-up item through the pipeline.
func runFollowUp(opts WatcherOpts, item *QueueItem) error {
	sessions, err := LoadSessionMap(opts.ProjectDir)
	if err != nil {
		return fmt.Errorf("load sessions: %w", err)
	}
	if sessions == nil {
		return fmt.Errorf("no session map found — run 'dp pipeline' first")
	}

	_, err = Run(PipelineOpts{
		Task:        item.Task,
		ProjectDir:  opts.ProjectDir,
		AgentPreset: opts.AgentPreset,
		Model:       opts.Model,
		StepTimeout: opts.StepTimeout,
		MaxRetries:  opts.MaxRetries,
		SkipReview:  opts.SkipReview,
		SkipQA:      opts.SkipQA,
		OnStepStart: opts.OnStepStart,
		OnStepDone:  opts.OnStepDone,
		SessionMap:  sessions,
		IsFollowUp:  true,
	})
	return err
}
