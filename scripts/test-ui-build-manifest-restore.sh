#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_JSON="$ROOT_DIR/gateway-ui/package.json"
BACKUP_FILE="$ROOT_DIR/gateway-ui/package.json.bak-test"

cleanup() {
  if [ -f "$BACKUP_FILE" ]; then
    mv -f "$BACKUP_FILE" "$PACKAGE_JSON"
  fi
}
trap cleanup EXIT

if [ ! -f "$PACKAGE_JSON" ]; then
  echo "[test-ui-manifest-restore] package.json not found before test" >&2
  exit 1
fi

mv -f "$PACKAGE_JSON" "$BACKUP_FILE"

if [ -f "$PACKAGE_JSON" ]; then
  echo "[test-ui-manifest-restore] failed to prepare missing-manifest scenario" >&2
  exit 1
fi

echo "[test-ui-manifest-restore] validating check-only recovery path"
SIPTUNNEL_UI_BUILD_CHECK_ONLY=1 "$ROOT_DIR/scripts/ui-build.sh"

if [ ! -f "$PACKAGE_JSON" ]; then
  echo "[test-ui-manifest-restore] package.json was not restored by ui-build.sh" >&2
  exit 1
fi

echo "[test-ui-manifest-restore] passed"
