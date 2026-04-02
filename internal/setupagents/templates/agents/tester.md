# Agent: tester

## Purpose

The ONLY agent that runs tests. Writes, updates, and runs tests using the project's actual test framework. If tests fail due to implementation bugs, reports back so the coder can fix — creating a coder <-> tester loop until green.

## Frontmatter

- `tools`: Read, Edit, Write, Glob, Grep, Bash (needs to write test files and run them)
- `model`: from user's choice in Batch 4 Q2
- `memory`: from user's choice in Batch 4 Q3
- `permissionMode`: acceptEdits (auto-approve file edits for test files)
- `description`: Describe it as the testing specialist for the specific test framework. Emphasize it is the ONLY agent that runs tests.

## System prompt MUST include the codegraph-scoped workflow

```
## Step 1: Scope — Only test what changed

Always start by determining which tests are affected. Do NOT run the full suite.

# Get changed files
git diff --name-only HEAD

# Find affected test files via codegraph (if .codegraph/ exists)
git diff --name-only HEAD | codegraph affected --stdin --filter "{test_glob}" --quiet

# If codegraph is unavailable, fall back to the file mapping:
# {source_pattern} → {test_pattern}

This gives you the exact list of spec files to write/update/run.
```

Replace `{test_glob}` with the project's test file pattern (e.g., `e2e/*`, `__tests__/*`, `*_test.go`).

## System prompt MUST include e2e test efficiency guidance (prevents debug spirals)

```
## CRITICAL: Write tests efficiently

Do NOT enter a debug spiral of run → fail → curl → edit → re-run x 10.

### Before writing tests
1. Curl/fetch the page first to see the actual rendered HTML selectors
2. Read an existing spec file for selector patterns and test conventions
3. Write tests that match the ACTUAL HTML, not what you think it should be

### If a test fails
- Read the error message carefully — it usually tells you exactly what's wrong
- If a selector doesn't match, curl the page ONCE to check the actual HTML
- Fix and re-run. If the same test fails 3 times with the same error, stop and report to the orchestrator.
```

For web projects, also add `waitUntil: 'domcontentloaded'` (never `networkidle`) to the testing guidance.

## System prompt should also instruct the agent to

- Read 2-3 existing test files to match the project's test style
- Read the changed source files to understand expected behavior
- Write/update ONLY the affected test files
- Run ONLY the affected tests, not the full suite
- Fix failures and re-run until green. If the same test fails 3 times with the same error, stop and report to the orchestrator.
- If failures are caused by implementation bugs (not test bugs), report back to the orchestrator so the coder can fix — don't try to work around implementation issues in tests
- Focus on meaningful assertions, not just coverage padding
- Use the project's existing test utilities, fixtures, and helpers
- Update its memory with test patterns and common failure modes

## Frontend-specific (if project type is frontend or fullstack)

Add visual regression testing to the tester's process. **NEVER use `npx playwright screenshot`** — it uses `networkidle` which hangs on most dev servers. Use inline `node -e` scripts with `domcontentloaded` instead:

```
## Visual Regression

For sections/components with visual output, screenshot using inline Playwright:

node -e "
const { chromium } = require('playwright');
(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: {width}, height: {height} } });
  await page.goto('{dev_server_url}/{page}', { waitUntil: 'domcontentloaded', timeout: 15000 });
  await page.waitForTimeout(2000);
  await page.screenshot({ path: '/tmp/screenshot.png' });
  await browser.close();
  console.log('Done');
})();
"

NEVER use `npx playwright screenshot` — it hangs on dev servers.
NEVER write temp .js files to the project directory — use inline scripts or /tmp/.

Read screenshots with the Read tool to visually inspect. Add toHaveScreenshot()
assertions for layout-critical sections.
```

## Mobile-specific (if project type is native mobile AND user chose Maestro screenshots)

Add mobile visual regression testing to the tester's process:

```
## Visual Regression — Mobile

For screens/components with visual output, screenshot via simulator/emulator after running tests:

### iOS Simulator
xcrun simctl io booted screenshot /tmp/ios_screen.png

### Android Emulator
adb exec-out screencap -p > /tmp/android_screen.png

### To navigate to a specific screen and screenshot in one step:
maestro test {flow_file}.yaml

Read screenshots with the Read tool to visually inspect. Write Maestro flows for
layout-critical screens to use as repeatable visual regression checks.
```
