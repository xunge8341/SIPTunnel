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
UI_GUARD_SCRIPT="$ROOT_DIR/scripts/ui-delivery-guard.mjs"
UI_GUARD_REPORT="$ROOT_DIR/artifacts/acceptance/ui-delivery-guard-latest.json"
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

if ! command -v node >/dev/null 2>&1; then
  echo "node not found" >&2
  exit 1
fi

GUARD_MODE="repair"
if [ "$CHECK_ONLY" = "1" ]; then
  GUARD_MODE="verify"
fi
echo "[ui-build] running UI delivery guard (mode: $GUARD_MODE)"
node "$UI_GUARD_SCRIPT" --mode "$GUARD_MODE" --allow-missing-embedded-metadata --report "$UI_GUARD_REPORT"
echo "[ui-build] delivery guard report: $UI_GUARD_REPORT"

if [ "$CHECK_ONLY" = "1" ]; then
  echo "[ui-build] check-only mode completed; manifest, guardrail, and toolchain validation passed"
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
