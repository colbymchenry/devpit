# devpit

Sequential agent pipeline CLI for AI coding agents.

## Installation

```bash
npm install -g devpit
```

This downloads the `dp` binary for your platform.

**Prerequisites:** [tmux](https://github.com/tmux/tmux) 3.0+ and an AI CLI ([Claude Code](https://claude.ai/code), [Gemini CLI](https://github.com/google-gemini/gemini-cli), [Codex](https://github.com/openai/codex), etc.)

## Quick Start

```bash
# 1. Generate agents for your project (one-time)
dp setup-agents

# 2. Run the full pipeline
dp pipeline "Add a health check endpoint"
```

## How It Works

`dp pipeline` runs agents **sequentially**, one at a time:

```
Architect → Coder → Tester ↔ Coder (retry) → Reviewer → Design QA ↔ Coder (retry)
```

Each step spawns a real AI agent in a tmux session, waits for it to finish, captures its output, and passes context to the next step. You can attach to any session and watch in real-time.

## Commands

```bash
dp pipeline "task"                      # Run full pipeline
dp pipeline agent architect "prompt"    # Run single agent interactively
dp pipeline status                      # Show running sessions
dp pipeline peek coder                  # Read agent's recent output
dp setup-agents                         # Generate agent files
```

## Agent Files

Agents live in `.claude/agents/*.md` with YAML frontmatter:

```yaml
---
name: architect
description: Plans implementation before code gets written
model: opus
tools: Read, Glob, Grep, Bash
---
Your system prompt here...
```

`dp setup-agents` detects your stack and generates these tailored to your project.

## Supported Platforms

- macOS (Intel and Apple Silicon)
- Linux (x64 and ARM64)
- Windows (x64)

## License

MIT
