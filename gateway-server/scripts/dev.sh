#!/usr/bin/env bash
set -euo pipefail

GATEWAY_PORT=${GATEWAY_PORT:-18080} go run ./cmd/gateway
