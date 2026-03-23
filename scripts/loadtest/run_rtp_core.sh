#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR/gateway-server"

EFFECTIVE_CHUNK_SIZE="${CHUNK_SIZE:-65536}"
if [[ -z "${CHUNK_SIZE:-}" && "${RTP_TRANSPORT:-UDP}" =~ ^([Uu][Dd][Pp])$ ]]; then
  EFFECTIVE_CHUNK_SIZE="61440"
fi

go run ./cmd/loadtest \
  -targets "${TARGETS:-rtp-upload}" \
  -rtp-address "${RTP_ADDRESS:-127.0.0.1:25000}" \
  -transfer-mode "${RTP_TRANSPORT:-UDP}" \
  -file-size "${FILE_SIZE:-1048576}" \
  -chunk-size "$EFFECTIVE_CHUNK_SIZE" \
  -concurrency "${CONCURRENCY:-32}" \
  -qps "${QPS:-0}" \
  -duration "${DURATION:-60s}" \
  -output-dir "${OUTPUT_DIR:-./loadtest/results}" \
  -timeout "${TIMEOUT:-5s}" \
  -gateway-base-url "${GATEWAY_BASE_URL:-}" \
  -diag-interval "${DIAG_INTERVAL:-0s}"
