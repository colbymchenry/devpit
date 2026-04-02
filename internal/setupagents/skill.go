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

//go:embed templates/agents/* templates/hooks/* templates/scripts/* templates/*.md
var agentTemplates embed.FS

// SkillPrompt returns the SKILL.md content with a reference to the temp
// directory containing agent templates.
func SkillPrompt(templateDir string) string {
	return fmt.Sprintf("%s\n\n---\n\nAll templates are available at: %s\nWhen the instructions say 'Read [templates/X](templates/X)', read the file at: %s/X\nProject directory: %s\n",
		skillMD, templateDir, templateDir, mustGetwd())
}

// WriteTemplatesToDir extracts all embedded templates to a temporary directory.
// Returns the directory path. Caller is responsible for cleanup.
func WriteTemplatesToDir() (string, error) {
	tmpDir, err := os.MkdirTemp("", "gt-setup-agents-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	if err := extractDir("templates", tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}

	return tmpDir, nil
}

// extractDir recursively extracts an embedded directory to destRoot,
// stripping the "templates/" prefix so files land at destRoot/<subpath>.
func extractDir(srcDir, destRoot string) error {
	entries, err := agentTemplates.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read embedded dir %q: %w", srcDir, err)
	}

	for _, entry := range entries {
		srcPath := srcDir + "/" + entry.Name()
		// Strip "templates/" prefix for the destination path
		relPath := srcPath[len("templates/"):]
		dstPath := filepath.Join(destRoot, relPath)

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				return err
			}
			if err := extractDir(srcPath, destRoot); err != nil {
				return err
			}
			continue
		}

		// Ensure parent dir exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}

		data, err := agentTemplates.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read template %q: %w", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return fmt.Errorf("write template %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
