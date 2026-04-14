#!/usr/bin/env bash
# check-secrets.sh — Scans staged files for secrets. Exit 2 to block commit.
set -euo pipefail

# Patterns that indicate secrets
SECRET_PATTERNS=(
  'AKIA[0-9A-Z]{16}'                    # AWS Access Key
  'sk-[a-zA-Z0-9]{20,}'                 # OpenAI / Stripe secret key
  'sk-ant-[a-zA-Z0-9-]{20,}'            # Anthropic API key
  'ghp_[a-zA-Z0-9]{36}'                 # GitHub personal access token
  'gho_[a-zA-Z0-9]{36}'                 # GitHub OAuth token
  'glpat-[a-zA-Z0-9\-]{20,}'            # GitLab personal access token
  'xox[bpoas]-[a-zA-Z0-9\-]+'           # Slack tokens
  'eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*'  # JWT
  'password\s*[:=]\s*["\x27][^"\x27]{8,}'  # password assignments
)

# File patterns that should never be committed
FORBIDDEN_FILES=(
  '\.env$'
  '\.env\.'
  '\.key$'
  '\.pem$'
  '^creds'
)

FOUND=0

# Check staged files for secret patterns
STAGED_FILES=$(git diff --cached --name-only --diff-filter=ACM 2>/dev/null || echo "")

if [ -z "$STAGED_FILES" ]; then
  exit 0
fi

# Check forbidden file names
for pattern in "${FORBIDDEN_FILES[@]}"; do
  MATCHES=$(echo "$STAGED_FILES" | grep -E "$pattern" || true)
  if [ -n "$MATCHES" ]; then
    echo "BLOCKED: Forbidden file detected in staged changes:"
    echo "$MATCHES"
    FOUND=1
  fi
done

# Check file contents for secret patterns
for file in $STAGED_FILES; do
  if [ ! -f "$file" ]; then
    continue
  fi
  for pattern in "${SECRET_PATTERNS[@]}"; do
    if grep -qE "$pattern" "$file" 2>/dev/null; then
      echo "BLOCKED: Potential secret found in $file (pattern: $pattern)"
      FOUND=1
    fi
  done
done

if [ "$FOUND" -eq 1 ]; then
  echo ""
  echo "Commit blocked by secret scanner. Remove secrets before committing."
  exit 2
fi

exit 0
