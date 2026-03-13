#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVER_DIR="$ROOT_DIR/gateway-server"
CONFIG_PATH="${SMOKE_CONFIG_PATH:-$SERVER_DIR/configs/config.yaml}"
BASE_URL="${SMOKE_BASE_URL:-http://127.0.0.1:${GATEWAY_PORT:-18080}}"
START_GATEWAY="${SMOKE_START_GATEWAY:-true}"
WAIT_SECONDS="${SMOKE_WAIT_SECONDS:-25}"
LOG_FILE="${SMOKE_LOG_FILE:-$ROOT_DIR/.smoke-gateway.log}"

wait_for_healthz() {
  local deadline=$((SECONDS + WAIT_SECONDS))
  while (( SECONDS < deadline )); do
    if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

cleanup() {
  if [[ -n "${GATEWAY_PID:-}" ]] && kill -0 "$GATEWAY_PID" >/dev/null 2>&1; then
    kill "$GATEWAY_PID" >/dev/null 2>&1 || true
    wait "$GATEWAY_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

if [[ "$START_GATEWAY" == "true" ]]; then
  echo "[smoke] starting gateway-server for smoke test..."
  (
    cd "$SERVER_DIR"
    go run ./cmd/gateway --config "$CONFIG_PATH"
  ) >"$LOG_FILE" 2>&1 &
  GATEWAY_PID=$!

  if ! wait_for_healthz; then
    echo "[smoke] gateway start timeout, log=$LOG_FILE"
    exit 1
  fi
fi

(
  cd "$SERVER_DIR"
  go run ./cmd/opssmoke --base-url "$BASE_URL" --config "$CONFIG_PATH"
)
