#!/usr/bin/env bash
# context-loader.sh — Outputs the first 30 lines of progress.md for context injection.
# Used by UserPromptSubmit hook so Claude always knows the current phase.
set -uo pipefail

PROGRESS_FILE="docs/progress.md"

if [ -f "$PROGRESS_FILE" ]; then
  head -n 30 "$PROGRESS_FILE"
else
  echo "No progress.md found. Check docs/progress.md."
fi

exit 0
