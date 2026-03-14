#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UI_DIR="$ROOT_DIR/gateway-ui"
DIST_DIR="$UI_DIR/dist"
BUILD_NONCE="${1:-$(date -u +%Y%m%dT%H%M%SZ)-$$}"
CHECK_ONLY="${SIPTUNNEL_UI_BUILD_CHECK_ONLY:-0}"

if ! command -v npm >/dev/null 2>&1; then
  echo "npm not found" >&2
  exit 1
fi

PACKAGE_JSON="$UI_DIR/package.json"
if [ ! -f "$PACKAGE_JSON" ]; then
  if command -v git >/dev/null 2>&1 && git -C "$ROOT_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1 && git -C "$ROOT_DIR" ls-files --error-unmatch gateway-ui/package.json >/dev/null 2>&1; then
    echo "[ui-build] package.json missing, restoring from git index: gateway-ui/package.json"
    git -C "$ROOT_DIR" checkout -- gateway-ui/package.json
  fi
fi

if [ ! -f "$PACKAGE_JSON" ]; then
  echo "UI package manifest missing: $PACKAGE_JSON. Please restore gateway-ui/package.json before running UI build (for git repos you can run: git checkout -- gateway-ui/package.json)." >&2
  exit 1
fi

if [ "$CHECK_ONLY" = "1" ]; then
  echo "[ui-build] check-only mode completed; manifest and toolchain validation passed"
  exit 0
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
