#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER_DIR="$ROOT_DIR/gateway-server"
DIST_DIR="$ROOT_DIR/dist/bin"
EMBEDDED_UI_DIR="$ROOT_DIR/gateway-server/internal/server/embedded-ui"
UI_METADATA_FILE="$EMBEDDED_UI_DIR/.siptunnel-ui-embed.json"

MODE="${1:-native}"
UI_POLICY="${2:-delivery}"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.buildTime=$BUILD_TIME"

mkdir -p "$DIST_DIR"

hash_dir() {
  local dir="$1"
  local exclude_name="${2:-}"
  (
    cd "$dir"
    find . -type f \
      | sed 's#^\./##' \
      | while read -r rel; do
          local base
          base="$(basename "$rel")"
          if [ -n "$exclude_name" ] && [ "$base" = "$exclude_name" ]; then
            continue
          fi
          local h
          h="$(sha256sum "$rel" | awk '{print $1}')"
          printf '%s:%s\n' "$rel" "$h"
        done \
      | LC_ALL=C sort
  ) | sha256sum | awk '{print $1}'
}

assert_ui_embed_ready() {
  local policy="$1"

  if [ "$policy" = "dev" ]; then
    echo "[build] UI policy: dev (skip embedded UI guard)"
    return
  fi

  echo "[build] UI policy: delivery (require fresh embedded UI)"

  if [ ! -f "$UI_METADATA_FILE" ]; then
    echo "[build] UI metadata missing: $UI_METADATA_FILE. Run ./scripts/embed-ui.sh first." >&2
    exit 1
  fi

  local expected_hash embedded_at source_latest actual_hash latest_ui_epoch embedded_epoch
  expected_hash="$(python3 - <<'PY' "$UI_METADATA_FILE"
import json,sys
with open(sys.argv[1],encoding='utf-8') as f:
  m=json.load(f)
print((m.get('embedded_hash_sha256') or '').lower())
PY
)"
  embedded_at="$(python3 - <<'PY' "$UI_METADATA_FILE"
import json,sys
with open(sys.argv[1],encoding='utf-8') as f:
  m=json.load(f)
print(m.get('embedded_at_utc') or '')
PY
)"
  source_latest="$(python3 - <<'PY' "$UI_METADATA_FILE"
import json,sys
with open(sys.argv[1],encoding='utf-8') as f:
  m=json.load(f)
print(m.get('ui_source_latest_write_utc') or '')
PY
)"

  if [ -z "$expected_hash" ]; then
    echo "[build] UI metadata invalid: embedded_hash_sha256 missing in $UI_METADATA_FILE" >&2
    exit 1
  fi

  actual_hash="$(hash_dir "$EMBEDDED_UI_DIR" ".siptunnel-ui-embed.json")"
  echo "[build] UI embedded_at_utc: $embedded_at"
  echo "[build] UI source latest write: $source_latest"
  echo "[build] UI hash expected: $expected_hash"
  echo "[build] UI hash actual:   $actual_hash"

  if [ "$expected_hash" != "$actual_hash" ]; then
    echo "[build] UI embed validation failed: embedded assets do not match metadata. Re-run ./scripts/embed-ui.sh." >&2
    exit 1
  fi

  latest_ui_epoch="$({
    find "$ROOT_DIR/gateway-ui" -type f \
      | grep -Ev '/(dist|node_modules)/' \
      | xargs -r stat -c '%Y'
  } | sort -nr | head -n1)"

  if [ -n "$latest_ui_epoch" ] && [ -n "$embedded_at" ]; then
    embedded_epoch="$(date -u -d "$embedded_at" +%s)"
    if [ "$embedded_epoch" -lt "$latest_ui_epoch" ]; then
      echo "[build] UI latest check: false" >&2
      echo "[build] UI embed is stale. Latest UI source write is newer than embedded timestamp. Run ./scripts/embed-ui.sh." >&2
      exit 1
    fi
  fi

  echo "[build] UI latest check: true"
  echo "[build] UI embed validation: PASS"
}

build_one() {
  local goos="$1"
  local goarch="$2"
  local ext=""
  if [[ "$goos" == "windows" ]]; then
    ext=".exe"
  fi
  local output_dir="$DIST_DIR/${goos}/${goarch}"
  local output="$output_dir/gateway${ext}"
  mkdir -p "$output_dir"
  echo "[build] ${goos}/${goarch} -> ${output}"
  (
    cd "$SERVER_DIR"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags "$LDFLAGS" -o "$output" ./cmd/gateway
  )
}

assert_ui_embed_ready "$UI_POLICY"

case "$MODE" in
  native)
    host_os="$(go env GOOS)"
    host_arch="$(go env GOARCH)"
    build_one "$host_os" "$host_arch"
    ;;
  matrix)
    build_one linux amd64
    build_one linux arm64
    build_one windows amd64
    build_one darwin amd64
    ;;
  *)
    echo "Usage: ./scripts/build.sh [native|matrix] [delivery|dev]"
    exit 1
    ;;
esac

echo "[build] done"
