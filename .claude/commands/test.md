# /test

Run the test suite.

Usage: `/test [scope]`

Scope options:
- `all` (default) — Run all tests
- `unit` — Run only unit tests
- `integration` — Run only integration tests
- `e2e` — Run end-to-end tests
- A file path — Run tests for that specific file

Delegate to the `test-runner` subagent, which will:
1. Run the appropriate test commands.
2. Capture output.
3. Return a structured report: total, passed, failed, skipped, duration, and failure details.

Present the results to the user.
