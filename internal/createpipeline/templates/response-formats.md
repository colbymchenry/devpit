# Response Formats for All Agents

Every agent's system prompt MUST include a response format section. Agents should produce **structured, complete output** — the orchestrator needs enough detail to coordinate the next step. No word limits. Use headings, bullets, and file references so output is scannable and actionable.

Add the appropriate format template to each agent's system prompt. Use concrete format templates — agents follow examples better than abstract instructions.

## Architect

```
## Response Format

Use structured output the orchestrator can pass directly to the coder.

## Summary
[1-2 sentence overview of the approach]

## Affected Files
- `path/file.ext` — [what changes and why]

## Implementation Plan
[Numbered steps with file paths, line references, and specific changes]

## Risks & Considerations
- [Edge cases, breaking changes, performance concerns]

## Open Questions
- [Anything that needs user input before proceeding]
```

## Coder

```
## Response Format

Report what you changed with enough detail that the orchestrator knows what to pass to the tester.

## Changes
- `file.ext` — [what changed and why]

## Lint
[pass/fail + details if failed]

## Notes
[Decisions made, trade-offs, anything the tester or reviewer should know]
```

## Tester

```
## Response Format

Report results with enough detail for the orchestrator to decide if a coder fix loop is needed.

## Results
- X tests passed, Y failed

## Failures (if any)
- test name: full error summary → likely cause (implementation bug vs test bug)

## Changes
- `e2e/file.spec.ts` — [what was added/updated and why]
```

## Reviewer

```
## Response Format

Focus on blockers and actionable suggestions. Every finding must include a concrete fix.

### Blockers
- `file:line` — issue → fix

### Suggestions
- `file:line` — issue → fix

### Good
- [What was done well — reinforces patterns worth keeping]
```

## Design QA

```
## Response Format

Report screenshot results and any issues found. Don't describe what looks fine — only call out problems.

## Screenshots
- `/tmp/desktop.png` — [OK or issue]
- `/tmp/mobile.png` — [OK or issue]

## Issues (if any)
- [viewport] [description] → [fix suggestion]
```
