# DevPit

**Sequential agent pipeline for AI coding agents**

DevPit runs specialized AI agents one at a time on a task — architect, coder, tester, reviewer, and (for visual projects) design-qa. Each agent runs in its own tmux session with full visibility. You can attach and watch any agent work in real-time.

## How It Works

```
dp pipeline "Add a health check endpoint"
```

```
Architect → Coder → Tester ↔ Coder (retry) → Reviewer → Design QA ↔ Coder (retry) → Done
```

Each step:
1. Spawns an AI agent in a tmux session
2. Sends the task + context from previous steps
3. Waits for the agent to finish
4. Captures output and passes it to the next step

Two retry loops keep quality high:
- **Coder ↔ Tester** — if tests fail, coder fixes and tester re-runs (max 3)
- **Coder ↔ Design QA** — if visual issues found, coder fixes and QA re-checks (max 3)

## Installation

### Prerequisites

- **tmux 3.0+** — agents run in tmux sessions
- **An AI CLI** — [Claude Code](https://claude.ai/code) (default), [Gemini CLI](https://github.com/google-gemini/gemini-cli), [Codex](https://github.com/openai/codex), or others

### From source

```bash
git clone https://github.com/colbymchenry/devpit.git
cd devpit
go build -o dp ./cmd/dp
cp dp /usr/local/bin/dp  # or ~/.local/bin/dp
```

### From npm

```bash
npm install -g devpit
```

## Quick Start

```bash
# 1. Generate agents for your project (one-time)
dp setup-agents

# 2. Run the pipeline
dp pipeline "Add a health check endpoint"
```

`dp setup-agents` detects your stack, asks about your preferences, and generates tailored agent files in `.claude/agents/`.

## Commands

### `dp pipeline "task"`

Run the full pipeline. Each step spawns one agent, waits for completion, captures output, kills the session, then moves to the next.

```bash
dp pipeline "Fix the login form validation"
dp pipeline "Refactor auth module" --agent gemini
dp pipeline "Add dark mode" --retries 2 --skip-review
```

| Flag | Default | Description |
|------|---------|-------------|
| `--agent` | claude | AI runtime (claude, gemini, codex, etc.) |
| `--timeout` | 10m | Max time per step |
| `--retries` | 3 | Max coder/tester or coder/design-qa retries |
| `--skip-review` | false | Skip the reviewer step |
| `--skip-qa` | false | Skip design-qa even if available |

### `dp pipeline agent <name> "prompt"`

Run a single agent interactively — spawns a tmux session and attaches your terminal.

```bash
dp pipeline agent architect "Design a caching layer"
dp pipeline agent coder "Implement the plan"
dp pipeline agent tester "Test the auth module"
dp pipeline agent reviewer "Review the latest changes" --detach
```

### `dp pipeline status`

Show running pipeline sessions with working/idle state.

### `dp pipeline peek <name>`

Read an agent's recent terminal output.

```bash
dp pipeline peek coder
dp pipeline peek tester -n 200
```

### `dp setup-agents`

Interactive setup that generates `.claude/agents/*.md` files for your project. Detects your stack, interviews you about preferences, and creates tailored agents.

```bash
dp setup-agents
dp setup-agents --agent gemini
```

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

| Agent | Role | When |
|-------|------|------|
| **architect** | Plans the implementation | Always |
| **coder** | Writes code, runs linter | Always |
| **tester** | Writes and runs tests | Always |
| **reviewer** | Reviews changes for quality | Always (skippable) |
| **design-qa** | Screenshots and visual QA | Only if `.claude/agents/design-qa.md` exists |

`dp setup-agents` generates these based on your project type:

| Project Type | Agents |
|-------------|--------|
| Backend/API | architect, coder, tester, reviewer |
| Frontend/Web | architect, coder, tester, reviewer, **design-qa** |
| Fullstack | architect, coder, tester, reviewer, **design-qa** |
| Native mobile | architect, coder, tester, reviewer, **design-qa** |

## Multi-Runtime Support

DevPit works with multiple AI CLIs. The `--agent` flag selects the runtime:

```bash
dp pipeline "task" --agent claude    # Claude Code (default)
dp pipeline "task" --agent gemini    # Gemini CLI
dp pipeline "task" --agent codex     # OpenAI Codex
dp pipeline "task" --agent copilot   # GitHub Copilot
```

Each runtime has its own readiness detection, prompt delivery, and startup dialog handling built into the tmux layer.

## How It's Built

DevPit's tmux layer handles the hard parts of agent orchestration:

- **Session management** — creation, the 8-step nudge protocol, idle detection with 2-consecutive-poll filtering, NBSP-normalized prompt matching, verified Enter delivery
- **Agent preset system** — runtime-specific commands, args, readiness detection for Claude, Gemini, Codex, Copilot, and others
- **Startup dialog acceptance** — auto-dismisses workspace trust and bypass-permissions dialogs

## License

MIT
