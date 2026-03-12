#!/usr/bin/env bash
set -euo pipefail

LISTEN_PORT="${LISTEN_PORT:-${GATEWAY_PORT:-18080}}"
MEDIA_PORT_START="${MEDIA_PORT_START:-20000}"
MEDIA_PORT_END="${MEDIA_PORT_END:-20100}"
NODE_ROLE="${NODE_ROLE:-receiver}"

is_integer() {
  [[ "$1" =~ ^[0-9]+$ ]]
}

check_port_value() {
  local name="$1"
  local value="$2"
  if ! is_integer "$value"; then
    echo "[ERROR] ${name} must be integer, got: ${value}"
    exit 1
  fi
  if (( value < 1 || value > 65535 )); then
    echo "[ERROR] ${name} must be in [1, 65535], got: ${value}"
    exit 1
  fi
}

port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -ltn "sport = :$port" | tail -n +2 | grep -q .
  elif command -v lsof >/dev/null 2>&1; then
    lsof -iTCP:"$port" -sTCP:LISTEN -Pn | tail -n +2 | grep -q .
  else
    return 1
  fi
}

check_port_value "LISTEN_PORT" "$LISTEN_PORT"
check_port_value "MEDIA_PORT_START" "$MEDIA_PORT_START"
check_port_value "MEDIA_PORT_END" "$MEDIA_PORT_END"

if (( MEDIA_PORT_START > MEDIA_PORT_END )); then
  echo "[ERROR] MEDIA_PORT_START must <= MEDIA_PORT_END"
  exit 1
fi

if [[ "$NODE_ROLE" != "receiver" && "$NODE_ROLE" != "sender" ]]; then
  echo "[ERROR] NODE_ROLE must be receiver or sender, got: $NODE_ROLE"
  exit 1
fi

if port_in_use "$LISTEN_PORT"; then
  echo "[ERROR] LISTEN_PORT=$LISTEN_PORT is already in use"
  exit 1
fi

echo "[OK] preflight passed"
echo "       LISTEN_PORT=$LISTEN_PORT"
echo "       MEDIA_PORT_RANGE=${MEDIA_PORT_START}-${MEDIA_PORT_END}"
echo "       NODE_ROLE=$NODE_ROLE"
