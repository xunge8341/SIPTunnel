#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR/gateway-server"
if [ -n "$(gofmt -l $(find . -name '*.go' -not -path './vendor/*'))" ]; then
  echo "Go files are not formatted. Run ./scripts/format.sh"
  exit 1
fi

cd "$ROOT_DIR/gateway-ui"
npm run lint
