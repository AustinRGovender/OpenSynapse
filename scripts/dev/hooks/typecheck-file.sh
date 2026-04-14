#!/usr/bin/env bash
# typecheck-file.sh — Runs type checking scoped to the changed file's package.
# Non-blocking: reports errors but does not exit non-zero.
set -uo pipefail

FILE="$1"

if [ ! -f "$FILE" ]; then
  exit 0
fi

EXT="${FILE##*.}"

case "$EXT" in
  ts|tsx)
    # Find the nearest tsconfig.json
    DIR=$(dirname "$FILE")
    while [ "$DIR" != "." ] && [ "$DIR" != "/" ]; do
      if [ -f "$DIR/tsconfig.json" ]; then
        if command -v npx &>/dev/null; then
          npx tsc --noEmit --project "$DIR/tsconfig.json" 2>&1 || true
        fi
        break
      fi
      DIR=$(dirname "$DIR")
    done
    ;;
  go)
    DIR=$(dirname "$FILE")
    if command -v go &>/dev/null; then
      go build "./$DIR/..." 2>&1 || true
    fi
    ;;
esac

exit 0
