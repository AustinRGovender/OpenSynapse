# /phase-done

Complete the current implementation phase.

1. Invoke the `phase-acceptance` skill to run acceptance tests for the current phase.
2. If all tests pass:
   - Update `docs/progress.md` with status "complete" and the completion date.
   - Prepare a commit with message format: `Phase N: <short description>`
   - Show the user what will be committed and ask for confirmation.
3. If any tests fail:
   - Report failures with actionable next steps.
   - Do not update progress or commit.
