#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <summary.json> [command_cc] [file_cc] [rtp_pool] [max_conn] [rps] [burst]" >&2
  exit 1
fi

SUMMARY_FILE="$1"
COMMAND_CC="${2:-100}"
FILE_CC="${3:-60}"
RTP_POOL="${4:-256}"
MAX_CONN="${5:-200}"
RPS="${6:-300}"
BURST="${7:-450}"

cd gateway-server

go run ./cmd/loadtest \
  -analyze-summary "$SUMMARY_FILE" \
  -current-command-max-concurrent "$COMMAND_CC" \
  -current-file-max-concurrent "$FILE_CC" \
  -current-rtp-port-pool "$RTP_POOL" \
  -current-max-connections "$MAX_CONN" \
  -current-rate-limit-rps "$RPS" \
  -current-rate-limit-burst "$BURST"
