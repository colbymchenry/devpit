package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRunRecord(t *testing.T) {
	r := NewRunRecord("Add health check", "claude")
	if r.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if r.Task != "Add health check" {
		t.Fatalf("expected task 'Add health check', got %q", r.Task)
	}
	if r.Agent != "claude" {
		t.Fatalf("expected agent 'claude', got %q", r.Agent)
	}
	if r.Status != StatusRunning {
		t.Fatalf("expected status running, got %q", r.Status)
	}
}

func TestSaveAndLoadRunRecord(t *testing.T) {
	dir := t.TempDir()

	r := NewRunRecord("Fix login bug", "gemini")
	now := time.Now()
	r.Steps = []StepRecord{
		{Name: "architect", Status: StatusPassed, Attempt: 1, StartedAt: now, Output: "plan here"},
	}

	if err := SaveRunRecord(dir, r); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify file exists
	path := filepath.Join(HistoryDir(dir), r.ID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}

	// Load and verify
	loaded, err := LoadRunRecord(dir, r.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Task != "Fix login bug" {
		t.Fatalf("expected task 'Fix login bug', got %q", loaded.Task)
	}
	if len(loaded.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(loaded.Steps))
	}
	if loaded.Steps[0].Name != "architect" {
		t.Fatalf("expected step 'architect', got %q", loaded.Steps[0].Name)
	}
}

func TestListRunRecords(t *testing.T) {
	dir := t.TempDir()

	// Create two records with different timestamps
	r1 := NewRunRecord("Task 1", "claude")
	r1.ID = "20260401T100000"
	r1.StartedAt = time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)

	r2 := NewRunRecord("Task 2", "claude")
	r2.ID = "20260402T100000"
	r2.StartedAt = time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)

	if err := SaveRunRecord(dir, r1); err != nil {
		t.Fatalf("save r1: %v", err)
	}
	if err := SaveRunRecord(dir, r2); err != nil {
		t.Fatalf("save r2: %v", err)
	}

	records, err := ListRunRecords(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	// Newest first
	if records[0].ID != "20260402T100000" {
		t.Fatalf("expected newest first, got %q", records[0].ID)
	}
}

func TestActiveRunRecords(t *testing.T) {
	dir := t.TempDir()

	r1 := NewRunRecord("Running task", "claude")
	r1.Status = StatusRunning

	r2 := NewRunRecord("Done task", "claude")
	r2.ID = "20260401T090000"
	r2.Status = StatusPassed

	if err := SaveRunRecord(dir, r1); err != nil {
		t.Fatalf("save r1: %v", err)
	}
	if err := SaveRunRecord(dir, r2); err != nil {
		t.Fatalf("save r2: %v", err)
	}

	active, err := ActiveRunRecords(dir)
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active, got %d", len(active))
	}
	if active[0].Task != "Running task" {
		t.Fatalf("expected 'Running task', got %q", active[0].Task)
	}
}

func TestListRunRecords_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	records, err := ListRunRecords(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 records, got %d", len(records))
	}
}
