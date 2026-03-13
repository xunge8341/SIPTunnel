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
  echo "[ui-build] node_modules not found, running npm install"
  npm install
fi

npm run build
echo "[ui-build] build output: $UI_DIR/dist"
