// Package createpipeline provides the dp create command that interactively
// creates pipeline workflows and agent files using a Claude session.
package createpipeline

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/colbymchenry/devpit/internal/config"
	"github.com/colbymchenry/devpit/internal/pipeline"
	"github.com/colbymchenry/devpit/internal/tmux"
)

//go:embed skill.md
var skillMD string

//go:embed templates/agents/* templates/scripts/* templates/*.md templates/*.yaml
var templates embed.FS

// SessionName is the tmux session name for the create pipeline session.
const SessionName = pipeline.SessionPrefix + "create"

// SkillPrompt returns the skill.md content with references to the template
// directory and project directory.
func SkillPrompt(templateDir string) string {
	return fmt.Sprintf("%s\n\n---\n\nAll templates are available at: %s\nWhen the instructions say 'Read [templates/X](templates/X)', read the file at: %s/X\nProject directory: %s\n",
		skillMD, templateDir, templateDir, mustGetwd())
}

// WriteTemplatesToDir extracts all embedded templates to a temporary directory.
// Caller is responsible for cleanup.
func WriteTemplatesToDir() (string, error) {
	tmpDir, err := os.MkdirTemp("", "dp-create-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	if err := extractDir("templates", tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}

	return tmpDir, nil
}

// SpawnSession creates a tmux session with a Claude agent for pipeline creation.
// Returns the tmux session name. The caller should attach to it.
func SpawnSession(projectDir, prompt, presetName string, isDefault bool) (string, error) {
	tmpDir, err := WriteTemplatesToDir()
	if err != nil {
		return "", err
	}
	// Don't defer cleanup — tmux session needs the files.

	skillPrompt := SkillPrompt(tmpDir)

	// Write prompt to a temp file to avoid tmux command length limits.
	promptFile := filepath.Join(tmpDir, "prompt.txt")
	if err := os.WriteFile(promptFile, []byte(skillPrompt), 0o600); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("write prompt file: %w", err)
	}

	if presetName == "" {
		presetName = "claude"
	}
	rc := config.RuntimeConfigFromPreset(config.AgentPreset(presetName))
	baseCommand := rc.BuildCommand()

	// Set PIPELINE_MODE so the skill prompt knows which flow to run.
	mode := "custom"
	if isDefault {
		mode = "default"
	}

	// Build the initial message for Claude.
	var initialMsg string
	if isDefault {
		initialMsg = "Your create-pipeline instructions are already loaded in the system prompt. Follow them now in default mode. Analyze this project and interview me to generate agent files."
		if prompt != "" {
			initialMsg += "\n\nAdditional context from the user: " + prompt
		}
	} else {
		if prompt != "" {
			initialMsg = "Your create-pipeline instructions are already loaded in the system prompt. Follow them now in custom mode. The user wants: " + prompt
		} else {
			initialMsg = "Your create-pipeline instructions are already loaded in the system prompt. Follow them now in custom mode. Ask me what kind of pipeline I want to create."
		}
	}

	command := fmt.Sprintf("exec env PIPELINE_MODE=%s PIPELINE_AGENT=create %s --append-system-prompt-file %s %s",
		mode, baseCommand, quoteForShell(promptFile), quoteForShell(initialMsg))

	t := tmux.NewTmux()
	_ = t.KillSession(SessionName)

	if err := t.NewSessionWithCommand(SessionName, projectDir, command); err != nil {
		return "", err
	}

	if err := t.AcceptStartupDialogs(SessionName); err != nil {
		_ = err
	}

	return SessionName, nil
}

// extractDir recursively extracts an embedded directory to destRoot,
// stripping the "templates/" prefix so files land at destRoot/<subpath>.
func extractDir(srcDir, destRoot string) error {
	entries, err := templates.ReadDir(srcDir)
	if err != nil {
		return fmt.Errorf("read embedded dir %q: %w", srcDir, err)
	}

	for _, entry := range entries {
		srcPath := srcDir + "/" + entry.Name()
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

		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}

		data, err := templates.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read template %q: %w", srcPath, err)
		}
		if err := os.WriteFile(dstPath, data, 0o644); err != nil {
			return fmt.Errorf("write template %q: %w", entry.Name(), err)
		}
	}
	return nil
}

// TmuxAttachArgs returns the args for attaching to the create session.
// Used by the TUI to build an exec.Cmd for tea.Exec.
func TmuxAttachArgs() []string {
	return []string{"tmux", "-u", "attach-session", "-t", SessionName}
}

// TmuxAttachCmd returns an *exec.Cmd that attaches to the create session.
func TmuxAttachCmd() *exec.Cmd {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return nil
	}
	cmd := exec.Command(tmuxPath, "-u", "attach-session", "-t", SessionName) //nolint:gosec // tmuxPath is from LookPath
	return cmd
}

func quoteForShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
