#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR/gateway-server"

args=(
  ./cmd/loadtest
  -targets "${TARGETS:-mapping-forward}"
  -mapping-url "${MAPPING_URL:-http://127.0.0.1:18090/orders}"
  -mapping-method "${MAPPING_METHOD:-POST}"
  -mapping-body-size "${MAPPING_BODY_SIZE:-4096}"
  -concurrency "${CONCURRENCY:-64}"
  -qps "${QPS:-0}"
  -duration "${DURATION:-60s}"
  -output-dir "${OUTPUT_DIR:-./loadtest/results}"
  -timeout "${TIMEOUT:-5s}"
  -gateway-base-url "${GATEWAY_BASE_URL:-}"
  -diag-interval "${DIAG_INTERVAL:-0s}"
)
if [[ "${ALLOW_PROBE_PATH:-true}" =~ ^([Tt]rue|1|yes|on)$ ]]; then
  args+=( -allow-probe-path )
fi
go run "${args[@]}"
