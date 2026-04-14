#!/usr/bin/env bash
# lint-file.sh — Runs the appropriate linter based on file extension.
# Non-blocking: reports issues but does not exit non-zero.
set -uo pipefail

FILE="$1"

if [ ! -f "$FILE" ]; then
  exit 0
fi

EXT="${FILE##*.}"

case "$EXT" in
  ts|tsx|js|jsx)
    if command -v npx &>/dev/null && [ -f "node_modules/.bin/eslint" ]; then
      npx eslint --no-error-on-unmatched-pattern "$FILE" 2>&1 || true
    fi
    ;;
  go)
    if command -v gofmt &>/dev/null; then
      DIFF=$(gofmt -l "$FILE" 2>&1)
      if [ -n "$DIFF" ]; then
        echo "gofmt: $FILE needs formatting"
        gofmt -d "$FILE" 2>&1 || true
      fi
    fi
    if command -v go &>/dev/null; then
      DIR=$(dirname "$FILE")
      go vet "./$DIR/..." 2>&1 || true
    fi
    ;;
  rs)
    if command -v rustfmt &>/dev/null; then
      rustfmt --check "$FILE" 2>&1 || true
    fi
    ;;
esac

exit 0
