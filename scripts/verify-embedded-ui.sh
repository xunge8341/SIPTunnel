#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR/gateway-server"

go test ./internal/server -run EmbeddedUIFallback -count=1

echo "embedded ui verification passed"
