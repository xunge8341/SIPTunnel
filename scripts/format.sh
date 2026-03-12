#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR/gateway-server"
gofmt -w $(find . -name '*.go' -not -path './vendor/*')

cd "$ROOT_DIR/gateway-ui"
npm run format
