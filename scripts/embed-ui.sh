#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
UI_DIR="$ROOT_DIR/gateway-ui"
DIST_DIR="$UI_DIR/dist"
TARGET_DIR="$ROOT_DIR/gateway-server/internal/server/embedded-ui"
METADATA_FILE="$TARGET_DIR/.siptunnel-ui-embed.json"
UI_GUARD_SCRIPT="$ROOT_DIR/scripts/ui-delivery-guard.mjs"
BUILD_NONCE="$(date +%Y%m%dT%H%M%S)-$$"

hash_dir() {
  local dir="$1"
  local exclude_name="${2:-}"
  if [ ! -d "$dir" ]; then
    echo "missing"
    return
  fi
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

latest_ui_write_local() {
  local latest
  latest="$({
    [ -d "$UI_DIR/src" ] && find "$UI_DIR/src" -type f -printf '%T@\n' || true
    [ -d "$UI_DIR/public" ] && find "$UI_DIR/public" -type f -printf '%T@\n' || true
    [ -f "$UI_DIR/index.html" ] && stat -c '%Y' "$UI_DIR/index.html" || true
    [ -f "$UI_DIR/package.json" ] && stat -c '%Y' "$UI_DIR/package.json" || true
    [ -f "$UI_DIR/package-lock.json" ] && stat -c '%Y' "$UI_DIR/package-lock.json" || true
    [ -f "$UI_DIR/vite.config.ts" ] && stat -c '%Y' "$UI_DIR/vite.config.ts" || true
  } | sort -nr | head -n1)"

  if [ -z "$latest" ]; then
    echo ""
    return
  fi

  date -d "@${latest%%.*}" +%Y-%m-%d\ %H:%M:%S
}

echo "[embed-ui] running UI build with nonce: $BUILD_NONCE"
bash "$ROOT_DIR/scripts/ui-build.sh" "$BUILD_NONCE"

if [ ! -d "$DIST_DIR" ]; then
  echo "[embed-ui] UI build output missing: $DIST_DIR" >&2
  exit 1
fi

MARKER_FILE="$DIST_DIR/.siptunnel-build-nonce"
if [ ! -f "$MARKER_FILE" ]; then
  echo "[embed-ui] build marker missing: $MARKER_FILE" >&2
  exit 1
fi

ACTUAL_NONCE="$(tr -d '\r\n' < "$MARKER_FILE")"
if [ -z "$ACTUAL_NONCE" ] || [ "$ACTUAL_NONCE" != "$BUILD_NONCE" ]; then
  echo "[embed-ui] build marker nonce mismatch (expected: $BUILD_NONCE, actual: $ACTUAL_NONCE)" >&2
  exit 1
fi

echo "[embed-ui] build marker validated, syncing embedded assets"

rm -rf "$TARGET_DIR"
mkdir -p "$TARGET_DIR"
cp -R "$DIST_DIR/." "$TARGET_DIR/"
rm -f "$TARGET_DIR/.siptunnel-build-nonce"

mkdir -p "$TARGET_DIR/errors"
if [ ! -f "$TARGET_DIR/errors/404.html" ]; then
  cat > "$TARGET_DIR/errors/404.html" <<'HTML'
<!doctype html>
<html><head><meta charset="utf-8"><title>404 Not Found</title></head><body><h1>404 Not Found</h1><p>页面未找到 / Requested resource was not found.</p></body></html>
HTML
fi
if [ ! -f "$TARGET_DIR/errors/500.html" ]; then
  cat > "$TARGET_DIR/errors/500.html" <<'HTML'
<!doctype html>
<html><head><meta charset="utf-8"><title>500 Internal Server Error</title></head><body><h1>500 Internal Server Error</h1><p>UI fallback page is temporarily unavailable.</p></body></html>
HTML
fi

if [ ! -f "$TARGET_DIR/favicon.svg" ]; then
  cat > "$TARGET_DIR/favicon.svg" <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 128 128"><rect width="128" height="128" rx="24" fill="#1677ff"/><text x="64" y="74" text-anchor="middle" font-family="Arial, sans-serif" font-size="44" fill="#fff">ST</text></svg>
SVG
fi


GUARD_JSON="$(node "$UI_GUARD_SCRIPT" --mode verify --allow-missing-embedded-metadata --json-stdout)"
GUARD_STATUS="$(printf '%s' "$GUARD_JSON" | node -e 'const fs=require("fs"); const data=JSON.parse(fs.readFileSync(0,"utf8")); process.stdout.write(data.status === "pass" ? "aligned" : "degraded")')"
GUARD_DETAIL="$(printf '%s' "$GUARD_JSON" | node -e 'const fs=require("fs"); const data=JSON.parse(fs.readFileSync(0,"utf8")); process.stdout.write(JSON.stringify(String(data.detail || "")).slice(1, -1))')"
GUARD_REMOVED_COUNT="$(printf '%s' "$GUARD_JSON" | node -e 'const fs=require("fs"); const data=JSON.parse(fs.readFileSync(0,"utf8")); process.stdout.write(String((data.removed_legacy_files || []).length))')"
GUARD_REMAINING_COUNT="$(printf '%s' "$GUARD_JSON" | node -e 'const fs=require("fs"); const data=JSON.parse(fs.readFileSync(0,"utf8")); process.stdout.write(String((data.remaining_legacy_files || []).length))')"
GUARD_HIT_COUNT="$(printf '%s' "$GUARD_JSON" | node -e 'const fs=require("fs"); const data=JSON.parse(fs.readFileSync(0,"utf8")); process.stdout.write(String((data.active_legacy_symbol_hits || []).length))')"

EMBEDDED_AT="$(date +%Y-%m-%d\ %H:%M:%S)"
DIST_HASH="$(hash_dir "$DIST_DIR" ".siptunnel-build-nonce")"
EMBEDDED_HASH="$(hash_dir "$TARGET_DIR" ".siptunnel-ui-embed.json")"
UI_SOURCE_LATEST="$(latest_ui_write_local)"

cat > "$METADATA_FILE" <<JSON
{
  "schema_version": 1,
  "generated_by": "scripts/embed-ui.sh",
  "build_nonce": "$BUILD_NONCE",
  "embedded_at_local": "$EMBEDDED_AT",
  "ui_source_latest_write_local": "$UI_SOURCE_LATEST",
  "dist_hash_sha256": "$DIST_HASH",
  "embedded_hash_sha256": "$EMBEDDED_HASH",
  "delivery_guard_status": "$GUARD_STATUS",
  "delivery_guard_detail": "$GUARD_DETAIL",
  "delivery_guard_removed_count": $GUARD_REMOVED_COUNT,
  "delivery_guard_remaining_count": $GUARD_REMAINING_COUNT,
  "delivery_guard_hit_count": $GUARD_HIT_COUNT,
  "dist_dir": "$DIST_DIR",
  "embedded_dir": "$TARGET_DIR"
}
JSON

echo "[embed-ui] UI source latest write: $UI_SOURCE_LATEST"
echo "[embed-ui] embedded at (local): $EMBEDDED_AT"
echo "[embed-ui] embedded hash: $EMBEDDED_HASH"
echo "[embed-ui] metadata: $METADATA_FILE"
echo "embedded UI assets synced to $TARGET_DIR"
