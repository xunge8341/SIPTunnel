#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MATRIX_FILE="${1:-$ROOT_DIR/gateway-server/tests/netem/matrix.boundary-mp4.json}"
exec "$ROOT_DIR/scripts/netem/run.sh" "$MATRIX_FILE"
