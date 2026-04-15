#!/usr/bin/env bash
# OpenSynapse Quick Start
# Checks prerequisites and starts the platform via Docker Compose.
# Make executable: chmod +x scripts/quick-start.sh

set -euo pipefail

COMPOSE_FILE="deploy/docker/docker-compose.yml"
WEB_URL="http://localhost:8080"
HEALTH_URL="http://localhost:8090/health"
MAX_WAIT=60

# ---------- helpers ----------

info()  { printf "[INFO]  %s\n" "$1"; }
error() { printf "[ERROR] %s\n" "$1" >&2; }
ok()    { printf "[OK]    %s\n" "$1"; }

check_command() {
  if ! command -v "$1" &>/dev/null; then
    error "$1 is required but not installed."
    error "Install $1 and try again."
    exit 1
  fi
}

# ---------- prerequisite checks ----------

info "Checking prerequisites..."

check_command docker

# Check for docker compose (v2 plugin) or docker-compose (standalone)
if docker compose version &>/dev/null; then
  COMPOSE_CMD="docker compose"
elif command -v docker-compose &>/dev/null; then
  COMPOSE_CMD="docker-compose"
else
  error "docker compose is required but not available."
  error "Install Docker Compose v2 and try again."
  exit 1
fi

ok "docker: $(docker --version)"
ok "compose: $($COMPOSE_CMD version 2>/dev/null || echo 'available')"

# ---------- resolve repo root ----------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

if [ ! -f "$REPO_ROOT/$COMPOSE_FILE" ]; then
  error "Cannot find $COMPOSE_FILE relative to repository root ($REPO_ROOT)."
  error "Make sure you are running this script from the opensynapse repository."
  exit 1
fi

# ---------- start services ----------

info "Starting OpenSynapse services..."
cd "$REPO_ROOT"
$COMPOSE_CMD -f "$COMPOSE_FILE" up -d --build

# ---------- wait for health ----------

info "Waiting for control plane to become healthy (up to ${MAX_WAIT}s)..."

elapsed=0
while [ "$elapsed" -lt "$MAX_WAIT" ]; do
  if curl -sf "$HEALTH_URL" >/dev/null 2>&1; then
    ok "Control plane is healthy."
    break
  fi
  sleep 2
  elapsed=$((elapsed + 2))
done

if [ "$elapsed" -ge "$MAX_WAIT" ]; then
  error "Control plane did not become healthy within ${MAX_WAIT}s."
  error "Check logs with: $COMPOSE_CMD -f $COMPOSE_FILE logs"
  exit 1
fi

# ---------- done ----------

echo ""
echo "============================================"
echo "  OpenSynapse is running."
echo "  Web UI:        $WEB_URL"
echo "  Control Plane: http://localhost:8090"
echo "============================================"
echo ""
echo "To stop:  $COMPOSE_CMD -f $COMPOSE_FILE down"
echo "To logs:  $COMPOSE_CMD -f $COMPOSE_FILE logs -f"
