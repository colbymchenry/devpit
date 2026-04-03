package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RunStatus represents the state of a pipeline run or step.
type RunStatus string

const (
	StatusPending   RunStatus = "pending"
	StatusRunning   RunStatus = "running"
	StatusPassed    RunStatus = "passed"
	StatusFailed    RunStatus = "failed"
	StatusSkipped   RunStatus = "skipped"
	StatusCancelled RunStatus = "cancelled"
)

// RunRecord represents a complete pipeline run, persisted as JSON.
type RunRecord struct {
	ID        string       `json:"id"`
	Task      string       `json:"task"`
	Agent     string       `json:"agent"`
	Status    RunStatus    `json:"status"`
	StartedAt time.Time    `json:"started_at"`
	EndedAt   *time.Time   `json:"ended_at,omitempty"`
	Steps     []StepRecord `json:"steps"`
}

// StepRecord represents a single pipeline step within a run.
type StepRecord struct {
	Name      string     `json:"name"`
	Status    RunStatus  `json:"status"`
	Attempt   int        `json:"attempt"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Output    string     `json:"output,omitempty"`
}

// HistoryDir returns the path to the pipeline history directory.
func HistoryDir(projectDir string) string {
	return filepath.Join(projectDir, ArtifactDir, "history")
}

// NewRunRecord creates a new run record with a timestamp-based ID.
func NewRunRecord(task, agent string) *RunRecord {
	now := time.Now()
	return &RunRecord{
		ID:        now.Format("20060102T150405"),
		Task:      task,
		Agent:     agent,
		Status:    StatusRunning,
		StartedAt: now,
	}
}

// SaveRunRecord writes a run record to disk atomically.
func SaveRunRecord(projectDir string, r *RunRecord) error {
	dir := HistoryDir(projectDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run record: %w", err)
	}

	dest := filepath.Join(dir, r.ID+".json")
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// LoadRunRecord reads a single run record by ID.
func LoadRunRecord(projectDir, id string) (*RunRecord, error) {
	path := filepath.Join(HistoryDir(projectDir), id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r RunRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("unmarshal run record: %w", err)
	}
	return &r, nil
}

// ListRunRecords returns all run records sorted newest first.
func ListRunRecords(projectDir string) ([]*RunRecord, error) {
	dir := HistoryDir(projectDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var records []*RunRecord
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-len(".json")]
		r, err := LoadRunRecord(projectDir, id)
		if err != nil {
			continue // skip corrupt files
		}
		records = append(records, r)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].StartedAt.After(records[j].StartedAt)
	})
	return records, nil
}

// DeleteRunRecord removes a run record from disk by ID.
func DeleteRunRecord(projectDir, id string) error {
	path := filepath.Join(HistoryDir(projectDir), id+".json")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete run record %q: %w", id, err)
	}
	return nil
}

// ActiveRunRecords returns only records with status "running".
func ActiveRunRecords(projectDir string) ([]*RunRecord, error) {
	all, err := ListRunRecords(projectDir)
	if err != nil {
		return nil, err
	}
	var active []*RunRecord
	for _, r := range all {
		if r.Status == StatusRunning {
			active = append(active, r)
		}
	}
	return active, nil
}
