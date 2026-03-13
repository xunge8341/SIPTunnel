#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
UI_DIR="$ROOT_DIR/gateway-ui"
TARGET_DIR="$ROOT_DIR/gateway-server/internal/server/embedded-ui"

"$ROOT_DIR/scripts/ui-build.sh"

rm -rf "$TARGET_DIR/assets"
mkdir -p "$TARGET_DIR"
cp -R "$UI_DIR/dist/." "$TARGET_DIR/"

echo "embedded UI assets synced to $TARGET_DIR"
