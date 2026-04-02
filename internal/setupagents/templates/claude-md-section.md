# CLAUDE.md Agent Workflow Section

After generating all agent files, you **MUST** update the project's `CLAUDE.md` with agent workflow instructions. **Without this, fresh Claude Code sessions will ignore the agents and act as a generalist — defeating the entire purpose of setup-agents.**

This is what makes the agents actually get used. The `description` field in agent frontmatter helps, but it is not reliable enough on its own. Explicit instructions in CLAUDE.md are loaded into every session and ensure Claude delegates properly.

## Steps

1. Read the existing `CLAUDE.md` in the project root. If none exists, create one.
2. Check if an `## Agent Workflow` section already exists — if so, replace it entirely.
3. Add the agent workflow section **at the very top of the file**, before all other content. This ensures Claude sees it first in every session.

## Template

Tailor the content to match the agents that were actually generated. Use this as a template — do NOT copy it verbatim. Fill in the project's actual agent names, test framework, review priorities, viewports, and any custom agents:

```markdown
## Agent Workflow

This project uses specialized AI agents in `.claude/agents/`. For any feature, bug fix, or significant change, **delegate to `@orchestrator`** — it manages the full workflow and returns a concise summary.

### How it works

`@orchestrator` runs the pipeline internally:
1. `@architect` → plans the implementation
2. `@coder` → writes code (linter only, no tests)
3. `@tester` → writes/runs tests (loops with coder if failures)
4. `@reviewer` → reviews the final diff
5. `@design-qa` → screenshots affected pages (UI changes only)

All verbose agent output stays inside the orchestrator's context. You get back a structured summary with: changes made, test results, review findings, and any issues.

### Direct agent access

You can also invoke agents directly when you want a specific one:

| Agent | Purpose | Invoke |
|-------|---------|--------|
| **orchestrator** | Full workflow — plan, code, test, review, QA | `@orchestrator` |
| **architect** | Plan only — scope, trade-offs, risks | `@architect` |
| **coder** | Implement only — code + lint | `@coder` |
| **tester** | Test only — write/run affected tests | `@tester` |
| **reviewer** | Review only — check diff for issues | `@reviewer` |
| **design-qa** | Visual QA only — screenshot + inspect | `@design-qa` |

### When NOT to delegate

- Quick questions about the codebase (just answer directly)
- Reading/explaining code (just read and explain)
- Git operations, deployments, or config changes (handle directly)
```

## Adjustments

- **Backend-only projects:** Remove design-qa from the pipeline description. The orchestrator will skip it automatically for non-UI changes.
- **Custom agents:** Add rows for any extra agents generated from Batch 4 Q4 (e.g., shopify-expert, security, devops).
- **Fill in specifics:** Replace any placeholder values with the actual values from the interview.
- **Preserve existing content:** When a CLAUDE.md already exists, keep all existing sections intact below the new agent workflow section.
