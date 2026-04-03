package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/colbymchenry/devpit/internal/flock"
)

// QueueItem represents a pending follow-up task.
type QueueItem struct {
	ID       string    `json:"id"`
	Task     string    `json:"task"`
	QueuedAt time.Time `json:"queued_at"`
	Status   string    `json:"status"` // pending | running | done | failed
}

// Queue represents the follow-up queue persisted to disk.
type Queue struct {
	Items []QueueItem `json:"items"`
}

const queueLockTimeout = 5 * time.Second

// queueFilePath returns the path to .pipeline/queue.json.
func queueFilePath(projectDir string) string {
	return filepath.Join(projectDir, ArtifactDir, QueueFile)
}

// queueLockPath returns the path to .pipeline/queue.lock.
func queueLockPath(projectDir string) string {
	return filepath.Join(projectDir, ArtifactDir, QueueLockFile)
}

// WatcherLockPath returns the path to .pipeline/watcher.lock.
func WatcherLockPath(projectDir string) string {
	return filepath.Join(projectDir, ArtifactDir, WatcherLockFile)
}

// EnqueueFollowUp appends a follow-up task to the queue file.
// Uses flock for concurrent safety.
func EnqueueFollowUp(projectDir, task string) (*QueueItem, error) {
	unlock, err := flock.AcquireLock(queueLockPath(projectDir), queueLockTimeout)
	if err != nil {
		return nil, fmt.Errorf("acquire queue lock: %w", err)
	}
	defer unlock()

	q, err := loadQueueUnlocked(projectDir)
	if err != nil {
		return nil, err
	}

	item := QueueItem{
		ID:       fmt.Sprintf("q-%d", time.Now().UnixMilli()),
		Task:     task,
		QueuedAt: time.Now(),
		Status:   "pending",
	}
	q.Items = append(q.Items, item)

	if err := saveQueueUnlocked(projectDir, q); err != nil {
		return nil, err
	}
	return &item, nil
}

// DequeueNext returns the next pending item and marks it "running".
// Returns nil, nil if no pending items exist.
func DequeueNext(projectDir string) (*QueueItem, error) {
	unlock, err := flock.AcquireLock(queueLockPath(projectDir), queueLockTimeout)
	if err != nil {
		return nil, fmt.Errorf("acquire queue lock: %w", err)
	}
	defer unlock()

	q, err := loadQueueUnlocked(projectDir)
	if err != nil {
		return nil, err
	}

	for i := range q.Items {
		if q.Items[i].Status == "pending" {
			q.Items[i].Status = "running"
			if err := saveQueueUnlocked(projectDir, q); err != nil {
				return nil, err
			}
			return &q.Items[i], nil
		}
	}
	return nil, nil
}

// UpdateItemStatus updates a queue item's status by ID.
func UpdateItemStatus(projectDir, itemID, status string) error {
	unlock, err := flock.AcquireLock(queueLockPath(projectDir), queueLockTimeout)
	if err != nil {
		return fmt.Errorf("acquire queue lock: %w", err)
	}
	defer unlock()

	q, err := loadQueueUnlocked(projectDir)
	if err != nil {
		return err
	}

	for i := range q.Items {
		if q.Items[i].ID == itemID {
			q.Items[i].Status = status
			return saveQueueUnlocked(projectDir, q)
		}
	}
	return fmt.Errorf("queue item %q not found", itemID)
}

// LoadQueue reads the queue from disk (acquires lock).
func LoadQueue(projectDir string) (*Queue, error) {
	unlock, err := flock.AcquireLock(queueLockPath(projectDir), queueLockTimeout)
	if err != nil {
		return nil, fmt.Errorf("acquire queue lock: %w", err)
	}
	defer unlock()

	return loadQueueUnlocked(projectDir)
}

// ClearQueue removes all items from the queue file.
func ClearQueue(projectDir string) error {
	unlock, err := flock.AcquireLock(queueLockPath(projectDir), queueLockTimeout)
	if err != nil {
		return fmt.Errorf("acquire queue lock: %w", err)
	}
	defer unlock()

	return saveQueueUnlocked(projectDir, &Queue{})
}

// PendingCount returns the number of pending items in the queue.
func PendingCount(projectDir string) (int, error) {
	q, err := LoadQueue(projectDir)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range q.Items {
		if item.Status == "pending" {
			count++
		}
	}
	return count, nil
}

// loadQueueUnlocked reads the queue from disk without acquiring a lock.
// Caller must hold the queue lock.
func loadQueueUnlocked(projectDir string) (*Queue, error) {
	data, err := os.ReadFile(queueFilePath(projectDir))
	if err != nil {
		if os.IsNotExist(err) {
			return &Queue{}, nil
		}
		return nil, fmt.Errorf("read queue file: %w", err)
	}
	var q Queue
	if err := json.Unmarshal(data, &q); err != nil {
		return nil, fmt.Errorf("unmarshal queue: %w", err)
	}
	return &q, nil
}

// saveQueueUnlocked writes the queue to disk atomically without acquiring a lock.
// Caller must hold the queue lock.
func saveQueueUnlocked(projectDir string, q *Queue) error {
	dir := filepath.Join(projectDir, ArtifactDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}

	data, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal queue: %w", err)
	}

	dest := queueFilePath(projectDir)
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
