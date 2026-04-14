#!/usr/bin/env bash
# test-related.sh — Finds and runs tests related to the changed file.
# Non-blocking: reports results but does not exit non-zero.
set -uo pipefail

FILE="$1"

if [ ! -f "$FILE" ]; then
  exit 0
fi

EXT="${FILE##*.}"
DIR=$(dirname "$FILE")
BASENAME=$(basename "$FILE" ".$EXT")

case "$EXT" in
  ts|tsx)
    # Look for co-located test files
    TEST_FILE=""
    for candidate in "$DIR/${BASENAME}.test.tsx" "$DIR/${BASENAME}.test.ts" "$DIR/__tests__/${BASENAME}.test.tsx" "$DIR/__tests__/${BASENAME}.test.ts"; do
      if [ -f "$candidate" ]; then
        TEST_FILE="$candidate"
        break
      fi
    done
    if [ -n "$TEST_FILE" ] && command -v npx &>/dev/null; then
      npx vitest run "$TEST_FILE" --reporter=verbose 2>&1 || true
    fi
    ;;
  go)
    if command -v go &>/dev/null; then
      go test "./$DIR/..." -v -count=1 -timeout=30s 2>&1 || true
    fi
    ;;
esac

exit 0
