#!/usr/bin/env bash
set -euo pipefail

LISTEN_PORT="${LISTEN_PORT:-${GATEWAY_PORT:-18080}}"
MEDIA_PORT_START="${MEDIA_PORT_START:-20000}"
MEDIA_PORT_END="${MEDIA_PORT_END:-20100}"
NODE_ROLE="${NODE_ROLE:-receiver}"
AUTO_FIX_PORTS="${AUTO_FIX_PORTS:-false}"

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

port_owner() {
  local port="$1"
  if command -v lsof >/dev/null 2>&1; then
    local row
    row="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | awk 'NR==2 {print $1"(pid="$2")"}')"
    if [[ -n "$row" ]]; then
      echo "$row"
      return 0
    fi
  fi
  return 1
}

suggest_free_port() {
  if command -v python3 >/dev/null 2>&1; then
    python3 - <<'PY'
import socket
s=socket.socket()
s.bind(("",0))
print(s.getsockname()[1])
s.close()
PY
    return 0
  fi
  return 1
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
  if owner="$(port_owner "$LISTEN_PORT")"; then
    echo "        detected_owner=$owner"
  fi
  echo "        diagnose: ss -ltnp"
  echo "        diagnose: lsof -i :$LISTEN_PORT"
  if [[ "${AUTO_FIX_PORTS,,}" == "true" ]]; then
    if free_port="$(suggest_free_port)"; then
      echo "        auto-fix candidate: export LISTEN_PORT=$free_port"
      echo "        note: auto-fix is suggestion only; verify with ./scripts/preflight.sh before restart"
    fi
  fi
  exit 1
fi

echo "[OK] preflight passed"
echo "       LISTEN_PORT=$LISTEN_PORT"
echo "       MEDIA_PORT_RANGE=${MEDIA_PORT_START}-${MEDIA_PORT_END}"
echo "       NODE_ROLE=$NODE_ROLE"
