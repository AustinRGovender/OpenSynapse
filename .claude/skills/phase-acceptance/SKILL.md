---
name: phase-acceptance
description: Run the acceptance tests for the current implementation phase and update progress tracking
tools:
  - Read
  - Bash
  - Edit
  - Glob
  - Grep
---

# Phase Acceptance Testing

This skill runs when finishing a phase (via `/phase-done` or when the user says they want to finish a phase).

## Procedure

1. **Determine the current phase** by reading `docs/progress.md`. Find the row with status "in progress".

2. **Look up the acceptance criteria** in `docs/06-implementation-plan.md` for that phase. Each phase has an "Acceptance" paragraph listing specific criteria.

3. **Execute each criterion**:
   - Build commands (e.g., `pnpm build`, `go build ./...`)
   - Test commands (e.g., `pnpm test`, `go test ./...`)
   - Verification steps (e.g., "the web app can create plans", "docker compose up works")
   - Run them and capture output

4. **Report results** per criterion: PASS or FAIL with details.

5. **If all pass**:
   - Update `docs/progress.md`: set the current phase's status to "complete" and fill in the "Completed" date
   - Prepare a commit message: `Phase N: <short description from the phase title>`
   - Prompt the user for commit confirmation

6. **If any fail**:
   - Stop and report the failure
   - List actionable next steps to fix each failure
   - Do not update progress.md or commit

## Reference

- Implementation plan: `docs/06-implementation-plan.md`
- Progress tracker: `docs/progress.md`
