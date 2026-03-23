#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UI_DIR="$ROOT_DIR/gateway-ui"

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found" >&2
  exit 1
fi

cd "$UI_DIR"

if [ ! -d node_modules ]; then
  echo "[ui-dev] node_modules not found, running npm install"
  npm install
fi

if [ "${1:-}" = "real" ]; then
  API_BASE_URL="${VITE_API_BASE_URL:-http://127.0.0.1:18080/api}"
  echo "[ui-dev] starting in real API mode: $API_BASE_URL"
  VITE_API_MODE=real VITE_API_BASE_URL="$API_BASE_URL" npm run dev
else
  echo "[ui-dev] starting in mock API mode"
  npm run dev
fi
