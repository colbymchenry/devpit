# Create Pipeline

You are creating a pipeline for DevPit — a sequential multi-agent pipeline CLI that spawns AI agents in tmux sessions, passing context between steps.

There are two modes:

1. **Default mode** — Create the standard development pipeline (architect → coder → tester → reviewer → design-qa). The user said `--default` or asked for the standard pipeline.
2. **Custom mode** — Create a custom workflow from the user's description. The user provided a prompt describing what they want.

Check the `PIPELINE_MODE` environment variable:
- `PIPELINE_MODE=default` → Default mode
- `PIPELINE_MODE=custom` → Custom mode (user's prompt follows in the conversation)

---

## Default Mode

When in default mode, follow this complete flow to generate the standard pipeline agents.

### Phase 0: Check for existing agents

Before scanning, check if agents already exist:

```bash
ls .claude/agents/*.md 2>/dev/null
```

If agent files exist, use AskUserQuestion:

- header: "Existing Agents"
- question: "I found existing agents: [list names]. What would you like to do?"
- multiSelect: false
- options:
  - "Regenerate all" — Start fresh, overwrite everything
  - "Update existing" — Re-detect stack and update agent prompts, keep memory intact
  - "Add new only" — Keep existing agents, just add any missing ones
  - "Cancel" — Exit without changes

If the user picks "Cancel", stop. If "Add new only", skip generating agents that already exist. If "Update existing", preserve the `memory` directory contents but regenerate the agent `.md` files.

### Phase 1: Detect

Scan the project to build a profile. Look for:

| Signal | Where to look |
|--------|--------------|
| Language(s) | File extensions, config files |
| Framework | package.json, Cargo.toml, requirements.txt, go.mod, composer.json, Gemfile, etc. |
| Project type | **Frontend** (React, Vue, Svelte, Next.js, Shopify theme, HTML/CSS, etc.), **Backend** (Express, Django, FastAPI, Go services, etc.), or **Fullstack** (both) |
| Test framework | Jest, Vitest, Pytest, Playwright, Cypress, Go test, RSpec, etc. |
| Linting/formatting | ESLint, Prettier, Biome, Black, Ruff, RuboCop, etc. |
| CI/CD | .github/workflows/, .gitlab-ci.yml, Jenkinsfile, etc. |
| Build tools | Vite, Webpack, Turbopack, esbuild, etc. |
| Project structure | Monorepo vs single, feature-based vs layered, src/ layout |
| Existing conventions | CLAUDE.md, .editorconfig, contribution guides |
| Package manager | npm, pnpm, yarn, bun, pip, cargo, etc. |
| Type system | TypeScript, Flow, Python type hints, etc. |
| CodeGraph | Does `.codegraph/` exist? Agents should use codegraph-scoped workflows |
| Dev server | What command starts the local server? What port? (for Playwright screenshots) |
| Has UI | Does this project render visual output a user sees in a browser? |

**Classify the project type.** This determines the agent set and validation strategy:

| Type | Indicators | Agent strategy |
|------|-----------|----------------|
| **Frontend / Web app** | HTML/CSS, templates, components, browser rendering, Shopify themes, SPAs, SSR sites, Storybook | Playwright screenshots + visual QA + e2e tests (strongly recommended) |
| **Backend / API** | REST/GraphQL APIs, CLI tools, libraries, no browser UI | curl/httpie + unit/integration tests |
| **Fullstack** | Both frontend rendering and backend API | Both strategies — Playwright for UI, curl/tests for API |
| **Native mobile** | iOS (Swift/SwiftUI), Android (Kotlin), React Native, Flutter | Maestro screenshots via simulator/emulator + unit/integration tests |

**Important:** If the project renders anything in a browser, it qualifies for the Playwright screenshot validation loop. This includes Shopify themes, Next.js, React SPAs, Vue apps, Svelte, static sites, component libraries, etc. Native mobile apps use Maestro + simulator/emulator screenshots instead.

**Mobile sub-classification** (when project type is native mobile):

| Platform | Indicators | Simulator/Emulator | Screenshot command |
|----------|-----------|-------------------|-------------------|
| **iOS** | .xcodeproj, .xcworkspace, Swift/SwiftUI files, Podfile | iOS Simulator (`xcrun simctl`) | `xcrun simctl io booted screenshot /tmp/screen.png` |
| **Android** | build.gradle, AndroidManifest.xml, Kotlin/Java files | Android Emulator (`adb`) | `adb exec-out screencap -p > /tmp/screen.png` |
| **React Native** | react-native in package.json, Metro bundler | Either — detect from project config | Platform-specific command above |
| **Flutter** | pubspec.yaml, dart files, flutter in config | Either — detect from project config | Platform-specific command above |

Present a concise summary table of what you found, including the project type classification.

### Phase 1.5: CodeGraph Setup

CodeGraph builds a semantic knowledge graph of the codebase, enabling agents to trace dependencies, find affected test files, and scope their work to only what changed. Without it, agents have to guess which files are affected by a change. With it, they know precisely — including transitive dependencies the diff doesn't show.

**This is strongly recommended for all projects.** Tell the user why: "CodeGraph lets your agents trace the full dependency graph so they only test, review, and QA files actually affected by your changes — including downstream files not in the diff."

#### Step 1: Check if codegraph is installed

```bash
command -v codegraph && codegraph --version
```

**If NOT installed:** Use AskUserQuestion:

- header: "CodeGraph"
- question: "I strongly recommend CodeGraph — it lets agents trace dependencies so they only test/review affected files, not the whole project. Install it now?"
- multiSelect: false
- options:
  - "Yes, install it (Recommended)" — Installs CodeGraph via npx
  - "Skip for now" — Agents will fall back to git diff file matching (less precise)

If the user chooses to install, run:

```bash
npx @colbymchenry/codegraph
```

#### Step 2: Check if the project is indexed

If codegraph IS installed, check for an existing index:

```bash
ls .codegraph/ 2>/dev/null
```

**If `.codegraph/` does NOT exist:** Use AskUserQuestion:

- header: "Index"
- question: "CodeGraph is installed but this project hasn't been indexed yet. Index it now so agents can trace dependencies?"
- multiSelect: false
- options:
  - "Yes, index it (Recommended)" — Builds the knowledge graph for this project
  - "Skip for now" — Agents will fall back to git diff file matching

If the user chooses to index, run:

```bash
codegraph init -i
```

**If `.codegraph/` already exists:** Good — note this in the detection summary.

#### Impact on agent generation

- **CodeGraph available + indexed:** All agents get the full codegraph-scoped workflow.
- **CodeGraph not available or not indexed:** All agents get fallback scoping using `git diff --name-only`.

### Phase 2: Interview

**IMPORTANT**: Use the `AskUserQuestion` tool for ALL questions. Do NOT ask questions as plain text. Each AskUserQuestion call supports 1-4 questions with 2-4 options each.

Wait for the user's answers before proceeding to the next batch.

#### Batch 1 — Confirm detection

**Q1:**
- header: "Stack"
- question: "I detected [summarize stack + project type]. Does this look right?"
- multiSelect: false
- options:
  - "Yes, looks good" — Detected stack is correct
  - "Needs corrections" — I'll provide corrections after this

**Q2** (only if no test framework detected):
- header: "Testing"
- question: "What testing framework do you want to use?"
- multiSelect: false
- options: Pick 3-4 relevant options based on the detected language/framework.

**Q3** (only if no linter detected):
- header: "Linting"
- question: "Do you use a linter or formatter?"
- multiSelect: false
- options: Pick 2-3 relevant options based on the detected language.

#### Batch 2 — Preferences

**Q1:**
- header: "Review"
- question: "What matters most in code review?"
- multiSelect: true
- options:
  - "Security" — Catch vulnerabilities, injection, auth issues
  - "Performance" — Identify bottlenecks, unnecessary re-renders, N+1 queries
  - "Readability" — Clear naming, simple logic, easy to follow
  - "Consistency" — Follow existing patterns, naming conventions, project style

**Q2:**
- header: "Architecture"
- question: "Any architecture patterns to enforce?"
- multiSelect: false
- options: Pick 3-4 relevant options based on the detected framework.

**Q3:**
- header: "Testing"
- question: "What level of test coverage do you aim for?"
- multiSelect: false
- options:
  - "Pragmatic" — Test what matters, skip the obvious (Recommended)
  - "Unit + Integration" — Thorough unit tests plus integration tests
  - "Full coverage" — Unit, integration, and e2e tests
  - "Unit only" — Just unit tests

**Q4:**
- header: "Conventions"
- question: "Any commit/PR conventions?"
- multiSelect: false
- options:
  - "Conventional Commits" — feat:, fix:, chore:, etc.
  - "Descriptive" — Plain English, no prefix format
  - "Project has existing" — Follow what's already in git log
  - "No preference" — Whatever works

**Q5:**
- header: "Attribution"
- question: "Should commits and PRs hide AI involvement?"
- multiSelect: false
- options:
  - "Undercover mode" — No "Co-Authored-By: Claude", no AI mentions
  - "Show attribution (Default)" — Include Co-Authored-By trailer

#### Batch 3 — Visual validation strategy

**Skip this batch entirely for backend-only projects.**

**For web projects (frontend or fullstack):**

**Q1:**
- header: "Validation"
- question: "For browser-rendered UIs, Playwright screenshots let agents visually verify their work. Enable this?"
- multiSelect: false
- options:
  - "Yes, Playwright screenshots (Recommended)" — Agents screenshot after UI changes, visually inspect, fix issues
  - "Tests only" — Just run the test suite
  - "Manual" — I'll verify visually myself

**Q2:**
- header: "Dev server"
- question: "What's your local dev server URL?"
- multiSelect: false
- options: Auto-detect from package.json scripts, framework defaults, or config.

**Q3:**
- header: "Viewports"
- question: "Which viewports should agents check?"
- multiSelect: false
- options:
  - "Mobile + Desktop (Recommended)" — 375x812 and 1440x900
  - "Mobile + Tablet + Desktop" — 375x812, 768x1024, and 1440x900
  - "Desktop only" — 1440x900

**For native mobile projects:**

**Q1:**
- header: "Mobile Validation"
- question: "Maestro screenshots let agents visually verify your mobile app in a simulator/emulator. Enable this?"
- multiSelect: false
- options:
  - "Yes, Maestro screenshots (Recommended)"
  - "Tests only"
  - "Manual"

**Q2:**
- header: "Platform"
- question: "Which platform(s) should agents validate?"
- multiSelect: false
- options: Auto-detect from project config (iOS only, Android only, Both).

**Q3:**
- header: "Screens"
- question: "How should agents navigate to affected screens for screenshots?"
- multiSelect: false
- options:
  - "Maestro flows (Recommended)"
  - "Deep links"
  - "Manual launch"

#### Batch 4 — Agent configuration

**Q1:**
- header: "Location"
- question: "Where should I install these agents?"
- multiSelect: false
- options:
  - "Project (Recommended)" — `.claude/agents/` — travels with the repo
  - "Global" — `~/.claude/agents/` — available in all your projects

**Q2:**
- header: "Models"
- question: "Which model strategy for agents?"
- multiSelect: false
- options:
  - "Optimized (Recommended)" — Opus for architect, Sonnet for reviewer/tester, inherit for coder
  - "All inherit" — Every agent uses whatever model your session is running
  - "All Sonnet" — Fast and cost-effective
  - "All Opus" — Maximum capability

**Q3:**
- header: "Memory"
- question: "Should agents have persistent memory across sessions?"
- multiSelect: false
- options:
  - "Project (Recommended)" — Agents learn project patterns, shareable via git
  - "Local" — Project-specific but gitignored
  - "Global" — Agents remember across all projects
  - "None" — No persistent memory

**Q4:**
- header: "Extras"
- question: "Any additional specialized agents?"
- multiSelect: true
- options: Pick 3-4 relevant suggestions based on the detected stack.

**Note:** For visual projects (Batch 3), the `design-qa` agent is automatically generated.

### Phase 3: Generate

Create each subagent as a `.md` file in the chosen location.

Use the detected stack info AND user answers to tailor every agent. Don't write generic instructions — reference the actual framework, test runner, linter, and conventions of THIS project.

#### Subagent file format

Each generated file follows this structure:

```markdown
---
name: {agent-name}
description: {what it does}
tools: {comma-separated tool list}
model: {opus, sonnet, haiku, or inherit}
memory: {user, project, local, or omit}
maxTurns: {optional}
effort: {optional — low, medium, high, or extreme}
permissionMode: {optional}
---

{System prompt — the ONLY prompt the subagent sees.}
```

#### Generate Workflow YAML — CRITICAL

**Always generate a workflow YAML file at `.claude/workflows/default.yaml`.** This is required — the pipeline engine loads all workflows from YAML, including the default. Without this file, `dp pipeline` will fail.

Generate the workflow based on the agents being created. For the standard pipeline:

```yaml
name: default
description: Standard development pipeline (architect, coder, tester, reviewer, design-qa)

steps:
  - name: architect

  - name: coder
    context: [architect]

  - name: tester
    context: [coder]
    loop:
      goto: coder
      max: 3
      pass: "PIPELINE_RESULT:PASS"
      fail: "PIPELINE_RESULT:FAIL"

  - name: reviewer
    context: [architect, coder, tester]

  - name: design-qa
    context: [coder, reviewer]
    optional: true
    loop:
      goto: coder
      max: 3
      pass: "ALL_CLEAR"
      fail: "ISSUES_FOUND"
```

**Adjust based on interview answers:**
- If the project is backend-only, omit the `design-qa` step
- If the user chose extras in Batch 4 Q4, add steps for those agents
- The `optional: true` flag means the step is skipped if its agent file doesn't exist

#### Agent generation templates

Each agent has a detailed generation template in the `templates/agents/` directory. Read the template for each agent you generate.

**Codegraph integration** — If `.codegraph/` exists, every agent's system prompt MUST include codegraph sections. Read [templates/codegraph-integration.md](templates/codegraph-integration.md) for details.

Generate these agents:
- **architect** — Read [templates/agents/architect.md](templates/agents/architect.md)
- **coder** — Read [templates/agents/coder.md](templates/agents/coder.md)
- **reviewer** — Read [templates/agents/reviewer.md](templates/agents/reviewer.md)
- **tester** — Read [templates/agents/tester.md](templates/agents/tester.md)
- **design-qa** (visual projects only) — Read [templates/agents/design-qa.md](templates/agents/design-qa.md)
- **orchestrator** (ALWAYS) — Read [templates/agents/orchestrator.md](templates/agents/orchestrator.md)
- Any extras from Batch 4 Q4

#### Response formats

Read [templates/response-formats.md](templates/response-formats.md) for format sections to include in each agent's system prompt.

#### Update Project CLAUDE.md — CRITICAL

Read [templates/claude-md-section.md](templates/claude-md-section.md) for the template. After generating agents, you **MUST** update the project's `CLAUDE.md` with agent workflow instructions.

#### Generate Verify Script

Read [templates/scripts/verify.sh](templates/scripts/verify.sh). Write to `scripts/verify.sh`. Make executable.

### Phase 4: Verify

After generating, use AskUserQuestion:

- header: "Done"
- question: "Pipeline created! Want to test it with a small task?"
- multiSelect: false
- options:
  - "Test it" — Run `dp pipeline` with a small feature
  - "Skip" — I'll try it out later

Tell the user:
- Run with: `dp pipeline "your task"`
- Or use the TUI: `dp`
- Agents are markdown files they can edit to refine
- If codegraph is initialized, agents automatically scope to affected files

---

## Custom Mode

When in custom mode, you're designing a custom workflow. The user's prompt describes what they want (e.g., "a benchmarking loop that tests, analyzes, and improves until a target is met").

### Phase 1: Detect project stack

Same as default mode Phase 1 — scan the project to understand the stack. This informs how you write agent prompts.

### Phase 1.5: CodeGraph Setup

Same as default mode Phase 1.5.

### Phase 2: Design the workflow

Based on the user's prompt, design a workflow. Think about:

1. **What steps are needed?** Each step is one agent execution.
2. **What context flows between steps?** Each step can declare which prior step outputs to include.
3. **Are there loops?** A step can loop back to an earlier step on failure.
4. **What are the exit conditions?** Each loop needs pass/fail markers.

Use AskUserQuestion to confirm your design:

- header: "Workflow"
- question: "Here's the workflow I designed: [describe steps and loops]. Does this look right?"
- multiSelect: false
- options:
  - "Looks good" — Proceed with this design
  - "Needs changes" — I'll describe what to change

Then ask about agent configuration:

- header: "Models"
- question: "Which model strategy for these agents?"
- multiSelect: false
- options:
  - "Optimized (Recommended)" — Higher-capability models for analysis, standard for execution
  - "All inherit" — Every agent uses whatever model your session is running
  - "All Sonnet" — Fast and cost-effective
  - "All Opus" — Maximum capability

### Phase 3: Generate

#### Workflow YAML format

Create the workflow file at `.claude/workflows/<name>.yaml`. The format:

```yaml
name: <workflow-name>
description: <what this workflow does>

steps:
  - name: <step-name>        # Unique identifier for this step
    agent: <agent-name>       # Agent file name (without .md). Defaults to name.
    context: [step1, step2]   # Prior step names whose output is included in prompt
    directive: "..."          # Per-step instruction added to the prompt
    optional: true/false      # Skip if agent file doesn't exist (default: false)
    loop:                     # Optional: loop back on failure
      goto: <step-name>      # Step to jump back to
      max: <number>           # Max iterations (default: 3)
      pass: "MARKER"         # Output text that signals success
      fail: "MARKER"         # Output text that signals failure
```

**Rules:**
- Step names must be unique
- `loop.goto` must reference an earlier step (no forward jumps)
- `loop.goto` cannot reference the step itself (no self-loops)
- `context` entries must reference earlier steps
- If `loop` is set, both `pass` and `fail` markers are required

Read [templates/workflow-example.yaml](templates/workflow-example.yaml) for a complete example.

#### Agent files

For each unique agent in the workflow, generate a `.claude/agents/<agent-name>.md` file.

Each agent needs a tailored system prompt that:
- Describes its specific role in the workflow
- References the project's actual stack/tools (from Phase 1 detection)
- Includes clear instructions for what to output
- For agents in loop steps: explicitly mentions the pass/fail markers to use

Use the subagent file format from the default mode section above.

**Important:** If the workflow reuses agent names from the standard pipeline (architect, coder, tester, reviewer), you can read the corresponding template in `templates/agents/` for inspiration. For entirely new agent types, write the system prompt from scratch based on the role.

#### Update CLAUDE.md

Same as default mode — update the project's CLAUDE.md.

### Phase 4: Verify

After generating, tell the user:

```
Workflow created!

  Workflow: .claude/workflows/<name>.yaml
  Agents:  .claude/agents/<agent1>.md, <agent2>.md, ...

  Run with: dp pipeline "<your task>" --workflow <name>
  Or use the TUI: dp
```

Use AskUserQuestion:

- header: "Done"
- question: "Custom pipeline created! Want to test it?"
- multiSelect: false
- options:
  - "Test it" — Run the workflow with a small task
  - "Skip" — I'll try it out later
