#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
METADATA_FILE="$ROOT_DIR/gateway-server/internal/server/embedded-ui/.siptunnel-ui-embed.json"
BACKUP_FILE="$METADATA_FILE.bak-test"

cleanup() {
  if [ -f "$BACKUP_FILE" ]; then
    mv -f "$BACKUP_FILE" "$METADATA_FILE"
  fi
}
trap cleanup EXIT

if [ -f "$METADATA_FILE" ]; then
  mv -f "$METADATA_FILE" "$BACKUP_FILE"
fi

echo "[test-ui-guard] case 1: missing metadata should block delivery build"
set +e
"$ROOT_DIR/scripts/build.sh" native delivery >/tmp/test-ui-guard-case1.log 2>&1
status=$?
set -e
if [ "$status" -eq 0 ]; then
  echo "[test-ui-guard] expected build to fail when metadata missing" >&2
  cat /tmp/test-ui-guard-case1.log >&2
  exit 1
fi

echo "[test-ui-guard] case 1 passed"

if [ -f "$BACKUP_FILE" ]; then
  mv -f "$BACKUP_FILE" "$METADATA_FILE"
fi

echo "[test-ui-guard] case 2: successful embed should allow delivery build"
"$ROOT_DIR/scripts/embed-ui.sh"
"$ROOT_DIR/scripts/build.sh" native delivery

echo "[test-ui-guard] all cases passed"
