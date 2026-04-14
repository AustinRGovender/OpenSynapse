#!/usr/bin/env bash
# block-dangerous.sh — Blocks dangerous commands. Exit 2 to block execution.
set -uo pipefail

COMMAND="$1"

# Block rm -rf at root or home
if echo "$COMMAND" | grep -qE 'rm\s+-rf\s+(/|~|\$HOME|%USERPROFILE%)'; then
  echo "BLOCKED: rm -rf at root or home directory is not allowed."
  exit 2
fi

# Block force push to main/master
if echo "$COMMAND" | grep -qE 'git\s+push\s+.*--force.*\s+(main|master)'; then
  echo "BLOCKED: Force push to main/master is not allowed."
  exit 2
fi
if echo "$COMMAND" | grep -qE 'git\s+push\s+-f\s+.*\s+(main|master)'; then
  echo "BLOCKED: Force push to main/master is not allowed."
  exit 2
fi

# Block git reset --hard without specific ref (too dangerous)
if echo "$COMMAND" | grep -qE 'git\s+reset\s+--hard\s*$'; then
  echo "BLOCKED: git reset --hard without a specific ref is dangerous. Specify a commit."
  exit 2
fi

# Block sudo rm
if echo "$COMMAND" | grep -qE 'sudo\s+rm'; then
  echo "BLOCKED: sudo rm is not allowed from this environment."
  exit 2
fi

# Block piping curl to bash from non-allowlisted domains
if echo "$COMMAND" | grep -qE 'curl\s.*\|\s*(bash|sh)'; then
  # Allow known safe domains
  if ! echo "$COMMAND" | grep -qE 'curl\s.*(get\.pnpm\.io|rustup\.rs|raw\.githubusercontent\.com)'; then
    echo "BLOCKED: Piping curl to bash from an unknown domain is not allowed."
    exit 2
  fi
fi

exit 0
