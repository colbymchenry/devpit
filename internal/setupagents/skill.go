// Package setupagents embeds the setup-agents SKILL.md and agent templates,
// providing the gt setup-agents command that bootstraps specialized AI
// subagents for a project.
package setupagents

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed skill.md
var skillMD string

//go:embed templates/agents/*
var agentTemplates embed.FS

// SkillPrompt returns the SKILL.md content with a reference to the temp
// directory containing agent templates.
func SkillPrompt(templateDir string) string {
	return fmt.Sprintf("%s\n\n---\n\nAgent templates are available at: %s\nProject directory: %s\n",
		skillMD, templateDir, mustGetwd())
}

// WriteTemplatesToDir extracts embedded agent templates to a temporary directory.
// Returns the directory path. Caller is responsible for cleanup.
func WriteTemplatesToDir() (string, error) {
	tmpDir, err := os.MkdirTemp("", "gt-setup-agents-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	entries, err := agentTemplates.ReadDir("templates/agents")
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("read embedded templates: %w", err)
	}

	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := agentTemplates.ReadFile("templates/agents/" + entry.Name())
		if err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("read template %q: %w", entry.Name(), err)
		}
		dst := filepath.Join(agentsDir, entry.Name())
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("write template %q: %w", entry.Name(), err)
		}
	}

	return tmpDir, nil
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
