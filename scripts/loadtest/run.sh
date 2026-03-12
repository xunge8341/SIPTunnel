#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT_DIR/gateway-server"

go run ./cmd/loadtest \
  -targets "sip-command-create,sip-status-receipt,rtp-udp-upload,rtp-tcp-upload,http-invoke" \
  -concurrency "${CONCURRENCY:-20}" \
  -qps "${QPS:-100}" \
  -file-size "${FILE_SIZE:-1048576}" \
  -chunk-size "${CHUNK_SIZE:-65536}" \
  -transfer-mode "${TRANSFER_MODE:-mixed}" \
  -duration "${DURATION:-30s}" \
  -sip-address "${SIP_ADDRESS:-127.0.0.1:5060}" \
  -rtp-address "${RTP_ADDRESS:-127.0.0.1:25000}" \
  -http-url "${HTTP_URL:-http://127.0.0.1:18080/demo/process}" \
  -output-dir "${OUTPUT_DIR:-./loadtest/results}" \
  -timeout "${TIMEOUT:-3s}"
