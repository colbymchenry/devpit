# Codegraph Integration

If `.codegraph/` exists in the project, every agent's system prompt MUST include BOTH sections below. CodeGraph serves two purposes: **exploration** (understanding code structure) and **scoping** (finding affected files). Without the exploration tools, agents waste dozens of tool calls grepping blindly.

## Exploration Tools — CodeGraph

Use codegraph MCP tools for instant symbol lookups instead of grepping. One codegraph_context call returns entry points, related symbols, and code snippets — replacing multiple grep/read cycles.

**Start here:** `codegraph_context` is the best first move for any task. It uses semantic search to find relevant entry points, then expands through the dependency graph to build full context.

| Tool | Use For |
|------|---------|
| `codegraph_context` | **Best starting point** — get full relevant context for a task (entry points, related symbols, code) |
| `codegraph_search` | Find symbols by name (functions, classes, custom elements) |
| `codegraph_callers` | Find what calls a function |
| `codegraph_callees` | Find what a function calls |
| `codegraph_impact` | See what's affected by changing a symbol |
| `codegraph_node` | Get details + source code for a specific symbol |
| `codegraph_files` | Get project file structure from the index (faster than Glob) |

Use these BEFORE reading files to understand the codebase structure. This saves tool calls vs. grepping blindly.

## Scoping — Only act on what changed

Always start by determining which files are affected. Do NOT scan the full project.

1. Get changed files:
   git diff --name-only HEAD

2. Find ALL affected files (including downstream dependencies) via codegraph:
   git diff --name-only HEAD | codegraph affected --stdin --quiet

3. Filter to specific file types if needed:
   git diff --name-only HEAD | codegraph affected --stdin --filter "e2e/*" --quiet

4. Limit traversal depth for large dependency trees:
   git diff --name-only HEAD | codegraph affected --stdin --depth 3 --quiet

This traces the full import/dependency graph — if you changed a utility, it finds every
test and component that transitively depends on it, even if they're not in the diff.

---

If `.codegraph/` does NOT exist, agents should fall back to manual file mapping (git diff + known test file patterns) and skip the exploration tools section.
