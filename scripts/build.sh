#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVER_DIR="$ROOT_DIR/gateway-server"
DIST_DIR="$ROOT_DIR/dist"

MODE="${1:-native}"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.buildTime=$BUILD_TIME"

mkdir -p "$DIST_DIR"

build_one() {
  local goos="$1"
  local goarch="$2"
  local ext=""
  if [[ "$goos" == "windows" ]]; then
    ext=".exe"
  fi
  local output="$DIST_DIR/gateway-${goos}-${goarch}${ext}"
  echo "[build] ${goos}/${goarch} -> ${output}"
  (
    cd "$SERVER_DIR"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags "$LDFLAGS" -o "$output" ./cmd/gateway
  )
}

case "$MODE" in
  native)
    host_os="$(go env GOOS)"
    host_arch="$(go env GOARCH)"
    build_one "$host_os" "$host_arch"
    ;;
  matrix)
    build_one linux amd64
    build_one windows amd64
    build_one darwin amd64
    ;;
  *)
    echo "Usage: ./scripts/build.sh [native|matrix]"
    exit 1
    ;;
esac

echo "[build] done"
