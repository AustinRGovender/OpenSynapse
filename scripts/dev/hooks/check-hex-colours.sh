#!/usr/bin/env bash
# check-hex-colours.sh — Blocks hex colours in UI component files outside tokens.ts.
# Exit 2 to block the write.
set -uo pipefail

FILE="$1"

if [ ! -f "$FILE" ]; then
  exit 0
fi

# Only check files under packages/ui/src/components/ and apps/web/src/
case "$FILE" in
  packages/ui/src/components/*|apps/web/src/*)
    ;;
  *)
    exit 0
    ;;
esac

# Allow the tokens file itself
case "$FILE" in
  packages/ui/src/tokens.ts|packages/ui/src/tokens.js)
    exit 0
    ;;
esac

# Allow test files and story files
case "$FILE" in
  *.test.*|*.stories.*|*.spec.*)
    exit 0
    ;;
esac

# Search for hex colour patterns: #RGB, #RGBA, #RRGGBB, #RRGGBBAA
# Exclude CSS custom property references like var(--color), Tailwind classes, and comments
HEX_MATCHES=$(grep -nE '#[0-9a-fA-F]{3,8}\b' "$FILE" 2>/dev/null | grep -vE '^\s*//' | grep -vE 'url\(#' || true)

if [ -n "$HEX_MATCHES" ]; then
  echo "BLOCKED: Hex colour(s) found in $FILE"
  echo "$HEX_MATCHES"
  echo ""
  echo "Add colours to packages/ui/src/tokens.ts instead of using hex values directly."
  echo "See docs/05-ui-ux-spec.md for the design token system."
  exit 2
fi

exit 0
