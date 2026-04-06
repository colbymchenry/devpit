# DevPit

**Sequential agent pipeline for AI coding agents**

DevPit runs specialized AI agents one at a time on a task. Each agent runs in its own tmux session with full visibility. You can attach and watch any agent work in real-time.

## How It Works

```
dp pipeline "Add a health check endpoint"
```

DevPit executes a workflow — an ordered list of steps. Each step:
1. Spawns an AI agent in a tmux session
2. Sends the task + context from previous steps
3. Waits for the agent to finish
4. Captures output and passes it to the next step

Steps can have loop-back conditions (e.g., tester fails → jump back to coder, retry up to 3 times).

The default workflow runs: **architect → coder → tester ↔ coder (retry) → reviewer → design-qa ↔ coder (retry)**

Custom workflows support arbitrary step sequences, context dependencies, and configurable pass/fail markers.

## Installation

### Prerequisites

- **tmux 3.0+** — agents run in tmux sessions
- **An AI CLI** — [Claude Code](https://claude.ai/code) (default), [Gemini CLI](https://github.com/google-gemini/gemini-cli), [Codex](https://github.com/openai/codex), or others

### From source

```bash
git clone https://github.com/colbymchenry/devpit.git
cd devpit
make install  # builds and installs to ~/.local/bin/dp
```

### From npm

```bash
npm install -g devpit
```

## Quick Start

```bash
# 1. Create a workflow (one-time)
dp create --default

# 2. Run the pipeline
dp pipeline "Add a health check endpoint"
```

`dp create` spawns Claude to interview you about your project, then generates agent files (`.claude/agents/*.md`) and a workflow (`.claude/workflows/default.yaml`). Use `--default` for the standard template or describe a custom workflow.

## Commands

### `dp` (no args)

Launch the interactive TUI dashboard. View running and past pipelines, start new runs, create workflows, and edit workflow configs — all from one interface.

### `dp pipeline "task"`

Run a workflow pipeline. Loads the default workflow from `.claude/workflows/default.yaml`, or specify a custom one with `--workflow`.

```bash
dp pipeline "Fix the login form validation"
dp pipeline "Refactor auth module" --agent gemini
dp pipeline "Optimize performance" --workflow optimize
```

| Flag | Default | Description |
|------|---------|-------------|
| `--agent` | claude | AI runtime (claude, gemini, codex, etc.) |
| `--model` | opus[1m] | Model override |
| `--timeout` | 10m | Max time per step |
| `--retries` | 3 | Max loop-back retries |
| `--workflow` | default | Custom workflow name (from `.claude/workflows/`) |

### `dp create [prompt]`

Create a new workflow interactively. Claude scans your project, interviews you, and generates agent files and a workflow YAML.

```bash
dp create                                           # TUI create form
dp create --default                                 # Standard template
dp create "benchmark loop that tests and improves"  # Custom workflow
```

### `dp pipeline agent <name> "prompt"`

Run a single agent interactively — spawns a tmux session and attaches your terminal.

```bash
dp pipeline agent architect "Design a caching layer"
dp pipeline agent coder "Implement the plan" --detach
```

### `dp pipeline follow "task"`

Queue a follow-up task that reuses the same agent sessions with full context.

```bash
dp pipeline follow "Make the button blue instead of green"
```

### `dp pipeline status`

Show running pipeline sessions with working/idle state.

### `dp pipeline peek <name>`

Read an agent's recent terminal output.

```bash
dp pipeline peek coder
dp pipeline peek tester -n 200
```

### `dp pipeline stop`

Stop all running pipeline agent sessions.

## TUI Dashboard

Run `dp` with no arguments to launch the interactive dashboard:

- **Dashboard** — view running and past pipeline runs, retry failed ones, kill active sessions
- **New run** (`n`) — launch a pipeline with a task, workflow, and agent selection
- **Create workflow** (`c`) — generate a new workflow with Claude
- **Edit workflow** (`e`) — modify workflow configs: reorder steps, edit fields, add/remove steps
- **History** (`h`) — browse past runs with status and details

## Custom Workflows

Workflows are YAML files in `.claude/workflows/`:

```yaml
name: optimize
description: Iterative benchmark-and-improve loop
steps:
  - name: baseline
    agent: benchmarker
  - name: analyst
    context: [baseline]
  - name: improver
    agent: coder
    context: [analyst]
    directive: "Implement the improvements proposed by the analyst"
  - name: verifier
    agent: benchmarker
    context: [improver]
    loop:
      goto: analyst
      max: 3
      pass: "PIPELINE_RESULT:PASS"
      fail: "PIPELINE_RESULT:FAIL"
```

Run with `dp pipeline "your task" --workflow optimize`.

Edit workflows in the TUI with `e` from the dashboard, or directly in YAML.

## Agent Files

Agents are markdown files in `.claude/agents/` with YAML frontmatter:

```yaml
---
name: architect
description: Plans implementation before code gets written
model: opus
tools: Read, Glob, Grep, Bash
effort: high
---

You are the architect. Analyze the task, identify affected files,
plan the implementation, and flag risks...
```

`dp create` generates these based on your project type and preferences.

## Multi-Runtime Support

DevPit works with multiple AI CLIs. The `--agent` flag selects the runtime:

```bash
dp pipeline "task" --agent claude    # Claude Code (default)
dp pipeline "task" --agent gemini    # Gemini CLI
dp pipeline "task" --agent codex     # OpenAI Codex
dp pipeline "task" --agent copilot   # GitHub Copilot
```

Each runtime has its own readiness detection, prompt delivery, and startup dialog handling built into the tmux layer.

## License

MIT
