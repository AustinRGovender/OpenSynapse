# /review

Run a code review on uncommitted changes.

Delegate to the `code-reviewer` subagent. It will:
1. Run `git diff` and `git diff --cached` to see all changes.
2. Check against project standards (design tokens, test coverage, API client sync, ADRs, security).
3. Return a structured review with verdict (PASS or NEEDS_CHANGES) and specific issues.

Present the review results to the user. If there are issues, offer to fix them.
