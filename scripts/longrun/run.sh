#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:-${LONGRUN_MODE:-smoke}}"

case "$MODE" in
  smoke|1h|6h|24h) ;;
  *)
    echo "[ERR] unsupported mode: $MODE (expected smoke/1h/6h/24h)" >&2
    exit 1
    ;;
esac

: "${LONGRUN_ENABLE:=1}"
: "${LONGRUN_REPORT_DIR:=$ROOT_DIR/gateway-server/tests/longrun/output}"

export LONGRUN_ENABLE LONGRUN_MODE="$MODE" LONGRUN_REPORT_DIR

if [[ -n "${LONGRUN_DURATION:-}" ]]; then
  export LONGRUN_DURATION
fi
if [[ -n "${LONGRUN_SAMPLE_INTERVAL:-}" ]]; then
  export LONGRUN_SAMPLE_INTERVAL
fi

cd "$ROOT_DIR/gateway-server"
go test ./tests/longrun -run TestLongRunStability -count=1 -v

echo "[INFO] longrun completed; reports in $LONGRUN_REPORT_DIR"
