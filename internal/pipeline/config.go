package pipeline

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentConfig represents a parsed .claude/agents/<name>.md file.
type AgentConfig struct {
	// Fields from YAML frontmatter
	Name            string `yaml:"name"`
	Description     string `yaml:"description"`
	Model           string `yaml:"model"`
	Tools           string `yaml:"tools"`
	DisallowedTools string `yaml:"disallowedTools"`
	PermissionMode  string `yaml:"permissionMode"`
	MaxTurns        int    `yaml:"maxTurns"`
	Effort          string `yaml:"effort"`

	// Body is the system prompt content after the frontmatter.
	Body string `yaml:"-"`
}

// LoadAgentConfig reads and parses a .claude/agents/<name>.md file.
func LoadAgentConfig(projectDir, name string) (*AgentConfig, error) {
	path := filepath.Join(projectDir, ".claude", "agents", name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent config %q: %w", name, err)
	}
	return parseAgentMd(data)
}

// AgentExists checks if a .claude/agents/<name>.md file exists.
func AgentExists(projectDir, name string) bool {
	path := filepath.Join(projectDir, ".claude", "agents", name+".md")
	_, err := os.Stat(path)
	return err == nil
}

// DiscoverPipeline returns the ordered list of pipeline steps based on
// which agent .md files exist in the project.
func DiscoverPipeline(projectDir string) []string {
	steps := make([]string, 0, len(CoreAgents)+1)
	for _, name := range CoreAgents {
		if AgentExists(projectDir, name) {
			steps = append(steps, name)
		}
	}
	if AgentExists(projectDir, VisualQAAgent) {
		steps = append(steps, VisualQAAgent)
	}
	return steps
}

// parseAgentMd splits a markdown file with YAML frontmatter into config + body.
// Expects the format:
//
//	---
//	name: architect
//	...
//	---
//	<system prompt body>
func parseAgentMd(data []byte) (*AgentConfig, error) {
	content := string(data)
	scanner := bufio.NewScanner(strings.NewReader(content))

	// Find opening ---
	var foundOpen bool
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" {
			foundOpen = true
			break
		}
		// Skip blank lines before frontmatter
		if line != "" {
			return nil, fmt.Errorf("expected YAML frontmatter (---), got: %q", line)
		}
	}
	if !foundOpen {
		return nil, fmt.Errorf("no YAML frontmatter found")
	}

	// Collect YAML lines until closing ---
	var yamlLines []string
	var foundClose bool
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			foundClose = true
			break
		}
		yamlLines = append(yamlLines, line)
	}
	if !foundClose {
		return nil, fmt.Errorf("unclosed YAML frontmatter")
	}

	// Parse YAML
	cfg := &AgentConfig{}
	yamlData := strings.Join(yamlLines, "\n")
	if err := yaml.Unmarshal([]byte(yamlData), cfg); err != nil {
		return nil, fmt.Errorf("parse YAML frontmatter: %w", err)
	}

	// Everything after closing --- is the body
	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}
	cfg.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	return cfg, nil
}
