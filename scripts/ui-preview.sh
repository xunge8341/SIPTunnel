#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UI_DIR="$ROOT_DIR/gateway-ui"

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found" >&2
  exit 1
fi

cd "$UI_DIR"

if [ ! -d dist ]; then
  echo "[ui-preview] dist not found, running ui build first"
  "$ROOT_DIR/scripts/ui-build.sh"
fi

npm run preview
