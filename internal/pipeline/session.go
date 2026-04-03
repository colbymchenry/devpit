package pipeline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// SessionMap tracks agent session IDs for resume capability.
// Generated on the first pipeline run and reused across follow-ups.
type SessionMap struct {
	RunID       string            `json:"run_id"`
	Task        string            `json:"task"`
	AgentPreset string            `json:"agent_preset,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	Agents      map[string]string `json:"agents"` // agent name -> session UUID
}

// GenerateSessionMap creates a new SessionMap with fresh UUIDs for each agent step.
func GenerateSessionMap(runID, task, agentPreset string, steps []string) *SessionMap {
	agents := make(map[string]string, len(steps))
	for _, step := range steps {
		agents[step] = uuid.New().String()
	}
	return &SessionMap{
		RunID:       runID,
		Task:        task,
		AgentPreset: agentPreset,
		CreatedAt:   time.Now(),
		Agents:      agents,
	}
}

// sessionsPath returns the path to .pipeline/sessions.json.
func sessionsPath(projectDir string) string {
	return filepath.Join(projectDir, ArtifactDir, SessionsFile)
}

// SaveSessionMap writes the session map to .pipeline/sessions.json atomically.
func SaveSessionMap(projectDir string, m *SessionMap) error {
	dir := filepath.Join(projectDir, ArtifactDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact dir: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session map: %w", err)
	}

	dest := sessionsPath(projectDir)
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// LoadSessionMap reads the session map from .pipeline/sessions.json.
// Returns nil, nil if the file does not exist.
func LoadSessionMap(projectDir string) (*SessionMap, error) {
	data, err := os.ReadFile(sessionsPath(projectDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m SessionMap
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal session map: %w", err)
	}
	return &m, nil
}

// DeleteSessionMap removes .pipeline/sessions.json.
func DeleteSessionMap(projectDir string) error {
	err := os.Remove(sessionsPath(projectDir))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
