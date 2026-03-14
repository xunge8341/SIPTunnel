#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UI_DIR="$ROOT_DIR/gateway-ui"
DIST_DIR="$UI_DIR/dist"
BUILD_NONCE="${1:-$(date -u +%Y%m%dT%H%M%SZ)-$$}"

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found" >&2
  exit 1
fi

cd "$UI_DIR"

if [ ! -d node_modules ]; then
  echo "[ui-build] node_modules not found, running npm install"
  npm install
fi

if [ -d "$DIST_DIR" ]; then
  echo "[ui-build] removing stale dist at $DIST_DIR"
  rm -rf "$DIST_DIR"
fi

npm run build

if [ ! -d "$DIST_DIR" ]; then
  echo "[ui-build] build completed but dist missing: $DIST_DIR" >&2
  exit 1
fi

printf '%s\n' "$BUILD_NONCE" > "$DIST_DIR/.siptunnel-build-nonce"

echo "[ui-build] build output: $DIST_DIR"
echo "[ui-build] build nonce: $BUILD_NONCE"
