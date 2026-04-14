#!/usr/bin/env bash
# check-openapi-drift.sh — Warns when the TypeScript API client may be stale.
# Non-blocking: emits a warning but does not exit non-zero.
set -uo pipefail

FILE="$1"

# Only trigger on handler or OpenAPI spec changes
case "$FILE" in
  apps/control-plane/internal/handlers/*|apps/control-plane/api/openapi.yaml)
    ;;
  *)
    exit 0
    ;;
esac

GENERATED="packages/api-client/src/generated.ts"

if [ ! -f "$GENERATED" ]; then
  echo "WARNING: API client has not been generated yet. Run: pnpm --filter api-client generate"
  exit 0
fi

# Check if the generated file is older than the changed file
if [ "$FILE" -nt "$GENERATED" ]; then
  echo "WARNING: The API client may be stale. '$FILE' is newer than '$GENERATED'."
  echo "Run: pnpm --filter api-client generate"
fi

exit 0
