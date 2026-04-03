# Verify Script

**Always generate this for every project that has a test framework.** The verify script is a utility that the **tester agent** runs to identify and execute affected tests. It is NOT a hook — running it on every response is wasteful and slow. Instead, the tester agent calls it as part of its workflow.

Write to `scripts/verify.sh`. Make it executable (`chmod +x`).

## What it does

1. Finds all changed files (staged + unstaged) via `git diff`
2. Separates source files from test files
3. Uses codegraph (if available) to find transitively affected test files
4. Falls back to direct test file matching from the diff
5. Runs only the affected tests
6. Reports pass/fail status

## Template

Generate a `scripts/verify.sh` tailored to the project's test framework and file patterns. The script should:

```bash
#!/usr/bin/env bash
#
# Verify script — called by the tester agent to check changed files and run affected tests.
#

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ── Gather changed files ──
CHANGED=$(git -C "$PROJECT_ROOT" diff --name-only HEAD 2>/dev/null || true)
STAGED=$(git -C "$PROJECT_ROOT" diff --name-only --cached 2>/dev/null || true)
ALL_CHANGED=$(printf '%s\n%s' "$CHANGED" "$STAGED" | sort -u | grep -v '^$' || true)

if [ -z "$ALL_CHANGED" ]; then
  echo "verify: no changes detected, skipping"
  exit 0
fi

# ── Show the diff ──
echo "═══ Git Diff ═══"
git -C "$PROJECT_ROOT" diff --stat HEAD 2>/dev/null || true
echo ""

echo "Changed files:"
echo "$ALL_CHANGED" | sed 's/^/  /'
echo ""

# ── Test file pattern matching ──
# CUSTOMIZE: Update these patterns to match the project's test file conventions
is_test_file() {
  case "$1" in
    {test_patterns})
      return 0 ;;
    *)
      return 1 ;;
  esac
}

# ── Find affected test files ──
AFFECTED_TESTS=""
SOURCE_FILES=$(echo "$ALL_CHANGED" | while IFS= read -r f; do
  is_test_file "$f" || echo "$f"
done)

# Strategy 1: codegraph affected (preferred — traces import dependencies)
if command -v codegraph >/dev/null 2>&1 && [ -d "$PROJECT_ROOT/.codegraph" ]; then
  if [ -n "$SOURCE_FILES" ]; then
    CG_RESULTS=$(echo "$SOURCE_FILES" | codegraph affected --stdin --filter "{test_glob}" --quiet --path "$PROJECT_ROOT" 2>/dev/null || true)
    if [ -n "$CG_RESULTS" ]; then
      AFFECTED_TESTS="$CG_RESULTS"
      echo "codegraph affected test files:"
      echo "$CG_RESULTS" | sed 's/^/  /'
      echo ""
    fi
  fi
fi

# Strategy 2: test files directly in the diff
CHANGED_TESTS=$(echo "$ALL_CHANGED" | while IFS= read -r f; do
  is_test_file "$f" && echo "$f"
done || true)

ALL_AFFECTED=$(printf '%s\n%s' "$AFFECTED_TESTS" "$CHANGED_TESTS" | sort -u | grep -v '^$' || true)

if [ -z "$ALL_AFFECTED" ]; then
  echo "No affected test files found."
  exit 0
fi

# ── Run only affected tests ──
# CUSTOMIZE: Replace with the project's actual test runner command
{test_runner_command}
```

## Customization points

Replace these placeholders with project-specific values:

| Placeholder | Description | Examples |
|-------------|-------------|----------|
| `{test_patterns}` | Shell glob patterns for test files in the `case` statement | `*.spec.*\|*.test.*\|e2e/*\|*/__tests__/*` (JS), `*_test.go` (Go), `test_*\|*_test.py` (Python) |
| `{test_glob}` | Codegraph `--filter` glob for test files | `e2e/*`, `**/*_test.go`, `tests/**` |
| `{test_runner_command}` | The command to run affected tests | `npx playwright test $SPEC_FILES`, `go test ./...`, `pytest $TEST_FILES` |

## Important

**Do NOT register this as a hook.** Running tests on every response is wasteful — it runs even when editing config files or answering questions. The tester agent owns test execution as part of the workflow.
