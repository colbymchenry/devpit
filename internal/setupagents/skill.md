---
name: setup-agents
description: Bootstrap specialized AI subagents (architect, coder, reviewer, tester) tailored to any project. Use when the user wants to set up agent workflows, create specialized coding agents, or says "setup agents". Analyzes the codebase, interviews the user about preferences, and generates project-specific subagent definitions.
user_invocable: true
allowed-tools: Read, Glob, Grep, Bash, Write, AskUserQuestion
---

# Setup Agents

You are bootstrapping specialized subagents for a project. Each subagent runs in its own isolated context window with a custom system prompt, tool restrictions, model choice, and optional persistent memory.

Subagents are markdown files in `.claude/agents/` (project-level) or `~/.claude/agents/` (global). They use YAML frontmatter for configuration and the markdown body as the system prompt.

## Overview

1. **Detect** — Scan the project, understand the stack, classify as frontend/backend/fullstack
2. **CodeGraph** — Ensure codegraph is installed and project is indexed
3. **Interview** — Ask the user targeted questions using AskUserQuestion
4. **Generate** — Create tailored subagent `.md` files based on findings + answers
5. **Verify** — Confirm everything is wired up

---

## Phase 0: Check for existing agents

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

---

## Phase 1: Detect

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

---

## Phase 1.5: CodeGraph Setup

CodeGraph builds a semantic knowledge graph of the codebase, enabling agents to trace dependencies, find affected test files, and scope their work to only what changed. Without it, agents have to guess which files are affected by a change. With it, they know precisely — including transitive dependencies the diff doesn't show.

**This is strongly recommended for all projects.** Tell the user why: "CodeGraph lets your agents trace the full dependency graph so they only test, review, and QA files actually affected by your changes — including downstream files not in the diff."

### Step 1: Check if codegraph is installed

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

This runs the interactive installer which configures the MCP server, permissions, and global instructions.

### Step 2: Check if the project is indexed

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

This initializes the project and runs a full index.

**If `.codegraph/` already exists:** Good — note this in the detection summary. Run `codegraph status` to confirm the index is current and show the user.

### Impact on agent generation

- **CodeGraph available + indexed:** All agents get the full codegraph-scoped workflow (`git diff | codegraph affected --stdin`). This is the optimal setup.
- **CodeGraph not available or not indexed:** All agents get fallback scoping instructions using `git diff --name-only` + manual file pattern matching. Still works, but can miss transitive dependencies.

---

## Phase 2: Interview

**IMPORTANT**: Use the `AskUserQuestion` tool for ALL questions. Do NOT ask questions as plain text. Each AskUserQuestion call supports 1-4 questions with 2-4 options each.

Wait for the user's answers before proceeding to the next batch.

### Batch 1 — Confirm detection (1 AskUserQuestion call)

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
- options: Pick 3-4 relevant options based on the detected language/framework. For frontend projects, always include Playwright. Examples:
  - "Playwright" — E2E browser testing with visual regression (Recommended for frontend)
  - "Jest" — Popular for JavaScript/TypeScript unit tests
  - "Vitest" — Fast, Vite-native test runner
  - "Pytest" — Python testing framework

**Q3** (only if no linter detected):
- header: "Linting"
- question: "Do you use a linter or formatter?"
- multiSelect: false
- options: Pick 2-3 relevant options based on the detected language. Examples:
  - "ESLint + Prettier" — Standard JS/TS linting and formatting
  - "Biome" — Fast all-in-one linter and formatter
  - "None" — No linter or formatter

If the user selects "Needs corrections", ask a follow-up AskUserQuestion or read their text input before continuing.

### Batch 2 — Preferences (1-2 AskUserQuestion calls, up to 5 questions)

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
- options: Pick 3-4 relevant options based on the detected framework. Examples for a web app:
  - "Feature-based" — Group by feature (auth/, products/, orders/)
  - "Component-driven" — Composable, reusable UI components
  - "MVC/Layered" — Separate models, views, controllers/routes
  - "No preference" — Follow whatever the codebase already does

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
  - "Undercover mode" — No "Co-Authored-By: Claude", no AI mentions in commit messages or PR descriptions
  - "Show attribution (Default)" — Include Co-Authored-By trailer and standard AI attribution

### Batch 3 — Visual validation strategy (1 AskUserQuestion call)

**Skip this batch entirely for backend-only projects.**

---

#### Batch 3A — Web projects (frontend or fullstack)

**For any project that renders in a browser** (web apps, Shopify themes, SPAs, SSR sites, component libraries with Storybook, etc.), Playwright screenshots are the recommended validation approach. This is critical — AI agents can't see what they're building without screenshots. Even if the project already uses Jest, Vitest, or another test runner for unit tests, Playwright screenshots should be layered on top as the visual verification loop.

Tell the user: "For any frontend that renders in a browser, I strongly recommend Playwright screenshots as your visual validation loop. This gives agents actual eyes on what they're building — they screenshot after every UI change, inspect it, and fix issues before reporting done. This works alongside whatever test framework you already use."

**Q1:**
- header: "Validation"
- question: "For browser-rendered UIs, Playwright screenshots let agents visually verify their work. Enable this?"
- multiSelect: false
- options:
  - "Yes, Playwright screenshots (Recommended)" — Agents screenshot after UI changes, visually inspect, fix issues before reporting done
  - "Tests only" — Just run the test suite, no visual verification
  - "Manual" — I'll verify visually myself

**Q2:**
- header: "Dev server"
- question: "What's your local dev server URL?"
- multiSelect: false
- options: Auto-detect from package.json scripts, framework defaults, or config. Examples:
  - "http://localhost:3000" — Detected from [framework/config]
  - "http://localhost:5173" — Vite default
  - "http://127.0.0.1:9292" — Shopify CLI dev server

**Q3:**
- header: "Viewports"
- question: "Which viewports should agents check?"
- multiSelect: false
- options:
  - "Mobile + Desktop (Recommended)" — 375x812 and 1440x900
  - "Mobile + Tablet + Desktop" — 375x812, 768x1024, and 1440x900
  - "Desktop only" — 1440x900

---

#### Batch 3B — Native mobile projects

**For native mobile apps** (iOS, Android, React Native, Flutter), Maestro + simulator/emulator screenshots give agents visual verification of their work. Without this, agents are coding blind — they can't see the UI they're building.

Tell the user: "For native mobile apps, I recommend Maestro as your visual validation loop. Maestro can launch your app in a simulator/emulator, navigate to screens, and take screenshots — giving agents actual eyes on what they're building. It's lightweight, uses simple YAML flows, and works with iOS Simulator and Android Emulator."

**Q1:**
- header: "Mobile Validation"
- question: "Maestro screenshots let agents visually verify your mobile app in a simulator/emulator. Enable this?"
- multiSelect: false
- options:
  - "Yes, Maestro screenshots (Recommended)" — Agents screenshot after UI changes via simulator/emulator, visually inspect, fix issues before reporting done
  - "Tests only" — Just run the test suite, no visual verification
  - "Manual" — I'll verify visually myself

**Q2:**
- header: "Platform"
- question: "Which platform(s) should agents validate?"
- multiSelect: false
- options: Auto-detect from project config. Examples:
  - "iOS only" — iOS Simulator (requires Xcode)
  - "Android only" — Android Emulator (requires Android Studio)
  - "Both iOS + Android" — Validate on both platforms

**Q3:**
- header: "Screens"
- question: "How should agents navigate to affected screens for screenshots?"
- multiSelect: false
- options:
  - "Maestro flows (Recommended)" — Agents write short YAML flows to navigate and screenshot. I'll provide existing flows if I have them.
  - "Deep links" — App supports deep links to specific screens (e.g., myapp://settings)
  - "Manual launch" — Just screenshot whatever is on screen after app launch

### Batch 4 — Agent configuration (1 AskUserQuestion call, up to 5 questions)

**Q1:**
- header: "Location"
- question: "Where should I install these agents?"
- multiSelect: false
- options:
  - "Project (Recommended)" — `.claude/agents/` — travels with the repo, shareable with team
  - "Global" — `~/.claude/agents/` — available in all your projects

**Q2:**
- header: "Models"
- question: "Which model strategy for agents?"
- multiSelect: false
- options:
  - "Optimized (Recommended)" — Opus for architect, Sonnet for reviewer/tester, inherit for coder
  - "All inherit" — Every agent uses whatever model your session is running
  - "All Sonnet" — Fast and cost-effective for all agents
  - "All Opus" — Maximum capability for all agents

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
- options: Pick 3-4 relevant suggestions based on the detected stack. Examples:
  - "DevOps" — Infrastructure, deployment, CI/CD specialist
  - "Security" — Security-focused analysis and hardening
  - "Documenter" — Documentation and API reference writer
  - "Migrator" — Upgrade and migration specialist

**Q5:**
- header: "Git Context Hook"
- question: "Want a session start hook that injects branch-aware git context? It shows Claude your recent commits, branch diff, and working directory state at the start of every session — no need to re-run git log."
- multiSelect: false
- options:
  - "Yes (Recommended)" — Generates a SessionStart hook that injects git history and branch context
  - "Skip" — Claude will use its default git status snapshot

**Note:** The agent-reminder hook (UserPromptSubmit) is NOT optional — it is always generated. It's what ensures Claude delegates to the right agent on every prompt instead of doing work directly. Without it, Claude drifts from the workflow mid-session.

**Note:** For any project where visual validation was enabled in Batch 3 (Playwright for web OR Maestro for mobile), the `design-qa` agent is automatically generated — it does NOT need to be selected as an extra. It is part of the core agent set for any project with visual output.

---

## Phase 3: Generate

Create each subagent as a `.md` file in the chosen location.

Use the detected stack info AND user answers to tailor every agent. Don't write generic instructions — reference the actual framework, test runner, linter, and conventions of THIS project.

### Subagent file format

Each generated file follows this structure:

```markdown
---
name: {agent-name}
description: {what it does, when Claude should delegate to it — be specific}
tools: {comma-separated tool list — restrict appropriately}
model: {opus, sonnet, haiku, or inherit}
memory: {user, project, local, or omit}
maxTurns: {optional — cap agentic turns to prevent runaway agents}
effort: {optional — low, medium, high, or extreme}
permissionMode: {optional — dontAsk, auto, acceptEdits, plan, bubble, bypassPermissions}
---

{System prompt — this is the ONLY prompt the subagent sees.
It replaces the default Claude Code system prompt entirely.
Include everything the agent needs: role, process, conventions, commands.}
```

#### Available frontmatter fields

| Field | Required | Values | Purpose |
|:------|:---------|:-------|:--------|
| `name` | Yes | string | Agent identifier, used with `@name` |
| `description` | Yes | string | Shown to Claude to help it decide when to delegate |
| `tools` | No | comma-separated list, or `*` for all | Tool whitelist. Omit = all tools |
| `disallowedTools` | No | comma-separated list | Tool blacklist. Use instead of `tools` when easier to exclude a few |
| `model` | No | opus, sonnet, haiku, inherit | Model override. `inherit` = use parent session's model |
| `memory` | No | user, project, local | Persistent memory scope. **Note:** enabling memory auto-injects Read, Write, and Edit tools even on otherwise read-only agents |
| `maxTurns` | No | integer | Cap on agentic turns. Prevents runaway agents |
| `effort` | No | low, medium, high, extreme | Controls reasoning depth |
| `permissionMode` | No | dontAsk, auto, acceptEdits, plan, bubble, bypassPermissions | How the agent handles permission prompts |
| `initialPrompt` | No | string | Prepended to the first user turn. Use for checklists or setup instructions |
| `hooks` | No | object | Agent-scoped hooks (e.g., a per-agent Stop hook) |

---

### Agent generation templates

Each agent has a detailed generation template in the `templates/agents/` directory. Read the template for each agent you generate — it contains the purpose, frontmatter fields, and system prompt guidance.

**Codegraph integration** — If `.codegraph/` exists in the project, every agent's system prompt MUST include codegraph exploration tools AND scoping sections. Read [templates/codegraph-integration.md](templates/codegraph-integration.md) for the exact sections to include.

If `.codegraph/` does NOT exist, agents should fall back to manual file mapping (git diff + known test file patterns) and skip the codegraph sections.

---

### Agent: `architect`

Read [templates/agents/architect.md](templates/agents/architect.md) for the full generation template.

### Agent: `coder`

Read [templates/agents/coder.md](templates/agents/coder.md) for the full generation template.

### Agent: `reviewer`

Read [templates/agents/reviewer.md](templates/agents/reviewer.md) for the full generation template.

### Agent: `tester`

Read [templates/agents/tester.md](templates/agents/tester.md) for the full generation template.

### Agent: `design-qa` (auto-generated for visual projects)

Read [templates/agents/design-qa.md](templates/agents/design-qa.md) for the full generation template. Generate this automatically if the user enabled visual validation in Batch 3.

### Additional agents

If the user selected extras in Batch 4 Q4, generate those too following the same pattern: purpose-built, project-specific, with appropriate tool restrictions, model, and codegraph-scoped workflows where relevant.

### Agent: `orchestrator` (ALWAYS generate this)

Read [templates/agents/orchestrator.md](templates/agents/orchestrator.md) for the full generation template.

---

### Response formats for ALL agents

Read [templates/response-formats.md](templates/response-formats.md) for the response format sections to include in each agent's system prompt.

---

### Update Project CLAUDE.md — CRITICAL

Read [templates/claude-md-section.md](templates/claude-md-section.md) for the template and instructions.

After generating all agent files, you **MUST** update the project's `CLAUDE.md` with agent workflow instructions. Without this, fresh sessions ignore the agents.

---

### Generate Session Hooks

**Always generate the agent-reminder hook.** Read [templates/hooks/agent-reminder.sh](templates/hooks/agent-reminder.sh) for the reference implementation and settings registration.

**Git context hook (optional)** — if the user chose "Yes" in Batch 4 Q5, read [templates/hooks/git-context.sh](templates/hooks/git-context.sh) for the reference implementation.

Write scripts to `.claude/hooks/` (or `~/.claude/hooks/` for global). Make all scripts executable (`chmod +x`).

Register hooks in `.claude/settings.local.json`. Merge new hooks without overwriting existing ones.

---

### Generate Verify Script (ALWAYS generate for projects with tests)

Read [templates/scripts/verify.sh](templates/scripts/verify.sh) for the reference implementation and placeholder table.

Write to `scripts/verify.sh`. Make it executable. **Do NOT register as a Stop hook** — the tester agent calls it as part of its workflow.

---

## Phase 4: Verify

After generating all agents, use AskUserQuestion:

- header: "Done"
- question: "Agents created! Want to test the orchestrator with a small task?"
- multiSelect: false
- options:
  - "Test orchestrator" — Give it a feature to plan, implement, test, and review
  - "Test architect" — Give it a feature to plan
  - "Test coder" — Give it something small to implement
  - "Skip" — I'll try them out later

Also tell the user:
- Use `@orchestrator` for any feature, fix, or significant change — it runs the full pipeline and returns a concise summary
- You can also invoke agents directly: `@architect`, `@coder`, `@tester`, `@reviewer`, `@design-qa`
- Agents are markdown files they can edit to refine — each one has a response format section you can customize
- Agents with memory will get smarter over time
- If codegraph is initialized, agents will automatically scope their work to only affected files
- The CLAUDE.md workflow section and agent-reminder hook keep Claude on track every session
