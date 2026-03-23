#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR/gateway-server"

if [[ -z "${MAPPING_URL:-}" ]]; then
  echo "MAPPING_URL is required" >&2
  exit 1
fi

go run ./cmd/loadtest \
  -targets "${TARGETS:-mapping-forward}" \
  -mapping-url "${MAPPING_URL}" \
  -mapping-method "${MAPPING_METHOD:-POST}" \
  -mapping-body-size "${MAPPING_BODY_SIZE:-4096}" \
  -concurrency "${CONCURRENCY:-64}" \
  -qps "${QPS:-0}" \
  -duration "${DURATION:-60s}" \
  -output-dir "${OUTPUT_DIR:-./loadtest/results}" \
  -timeout "${TIMEOUT:-5s}" \
  -gateway-base-url "${GATEWAY_BASE_URL:-}" \
  -diag-interval "${DIAG_INTERVAL:-0s}" \
  -strict-real-mapping
