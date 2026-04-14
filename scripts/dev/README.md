# Developer Scripts

Hook scripts used by Claude Code's `.claude/settings.json` configuration.

## hooks/

| Script                  | Event          | Blocking | Purpose                                       |
| ----------------------- | -------------- | -------- | --------------------------------------------- |
| check-secrets.sh        | PreToolUse     | Yes      | Scans staged files for API keys and secrets    |
| lint-file.sh            | PostToolUse    | No       | Runs linter appropriate to file type           |
| typecheck-file.sh       | PostToolUse    | No       | Runs type checker scoped to changed package    |
| test-related.sh         | PostToolUse    | No       | Runs tests co-located with the changed file    |
| context-loader.sh       | UserPromptSubmit | No     | Injects current phase from progress.md         |
| block-dangerous.sh      | PreToolUse     | Yes      | Blocks destructive shell commands              |
| check-hex-colours.sh    | PostToolUse    | Yes      | Blocks hex colours outside design tokens       |
| check-openapi-drift.sh  | PostToolUse    | No       | Warns when TypeScript API client is stale      |
