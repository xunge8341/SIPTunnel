#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVER_DIR="$ROOT_DIR/gateway-server"
BASE_CONFIG_PATH="${SMOKE_CONFIG_PATH:-$SERVER_DIR/configs/config.yaml}"
START_GATEWAY="${SMOKE_START_GATEWAY:-true}"
WAIT_SECONDS="${SMOKE_WAIT_SECONDS:-25}"
WINDOWS_GATEWAY_BIN="$ROOT_DIR/dist/bin/windows/amd64/gateway.exe"
LINUX_GATEWAY_BIN="$ROOT_DIR/dist/bin/linux/amd64/gateway"
LOG_FILE="${SMOKE_LOG_FILE:-$ROOT_DIR/.smoke-gateway.log}"
SMOKE_ROOT="${SMOKE_WORK_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/siptunnel-smoke-XXXXXX")}" 
SMOKE_DATA_DIR="$SMOKE_ROOT/data"
GENERATED_CONFIG_PATH="$SMOKE_ROOT/config.yaml"

get_free_tcp_port() {
  python3 - <<'PY'
import socket
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
}

get_free_udp_range_start() {
  python3 - <<'PY'
import socket
RANGE_LEN = 102
for start in range(30000, 60000 - RANGE_LEN, 103):
    socks = []
    try:
        for port in range(start, start + RANGE_LEN):
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            s.bind(("127.0.0.1", port))
            socks.append(s)
        print(start)
        break
    except OSError:
        pass
    finally:
        for s in socks:
            s.close()
else:
    raise SystemExit("unable to allocate free UDP port range")
PY
}

render_smoke_config() {
  python3 - "$BASE_CONFIG_PATH" "$GENERATED_CONFIG_PATH" "$GATEWAY_PORT_EFFECTIVE" "$SIP_PORT_EFFECTIVE" "$RTP_START_EFFECTIVE" "$RTP_END_EFFECTIVE" <<'PY'
from pathlib import Path
import re
import sys
src_path = Path(sys.argv[1])
dst_path = Path(sys.argv[2])
http_port, sip_port, rtp_start, rtp_end = sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6]
lines = src_path.read_text(encoding='utf-8').splitlines()
section = ''
for idx, line in enumerate(lines):
    m = re.match(r'^(?P<key>[A-Za-z0-9_]+):\s*$', line)
    if m:
        section = m.group('key')
        continue
    if re.match(r'^\S', line):
        section = ''
    if section == 'ui' and re.match(r'^\s{2}listen_port:\s*', line):
        lines[idx] = f'  listen_port: {http_port}'
    elif section == 'sip' and re.match(r'^\s{2}listen_port:\s*', line):
        lines[idx] = f'  listen_port: {sip_port}'
    elif section == 'rtp' and re.match(r'^\s{2}port_start:\s*', line):
        lines[idx] = f'  port_start: {rtp_start}'
    elif section == 'rtp' and re.match(r'^\s{2}port_end:\s*', line):
        lines[idx] = f'  port_end: {rtp_end}'
dst_path.write_text('\n'.join(lines) + '\n', encoding='utf-8')
PY
}


new_smoke_node_config() {
  local target_path="$1"
  local sip_port="$2"
  local rtp_start="$3"
  local rtp_end="$4"
  mkdir -p "$(dirname "$target_path")"
  cat >"$target_path" <<EOF
{
  "local_node": {
    "node_id": "gateway-a-01",
    "node_name": "Smoke Gateway",
    "node_role": "gateway",
    "network_mode": "SENDER_SIP__RECEIVER_RTP",
    "sip_listen_ip": "0.0.0.0",
    "sip_listen_port": ${sip_port},
    "sip_transport": "TCP",
    "rtp_listen_ip": "0.0.0.0",
    "rtp_port_start": ${rtp_start},
    "rtp_port_end": ${rtp_end},
    "rtp_transport": "UDP"
  },
  "peers": []
}
EOF
}


assert_smoke_config() {
  python3 - "$GENERATED_CONFIG_PATH" "$GATEWAY_PORT_EFFECTIVE" "$SIP_PORT_EFFECTIVE" "$RTP_START_EFFECTIVE" "$RTP_END_EFFECTIVE" <<'PY'
from pathlib import Path
import re
import sys
path = Path(sys.argv[1])
http_port, sip_port, rtp_start, rtp_end = sys.argv[2:6]
text = path.read_text(encoding='utf-8')
checks = [
    (rf'(?m)^  listen_port: {re.escape(http_port)}\\s*$', 'ui.listen_port'),
    (rf'(?m)^  listen_port: {re.escape(sip_port)}\\s*$', 'sip.listen_port'),
    (rf'(?m)^  port_start: {re.escape(rtp_start)}\\s*$', 'rtp.port_start'),
    (rf'(?m)^  port_end: {re.escape(rtp_end)}\\s*$', 'rtp.port_end'),
]
for pattern, label in checks:
    if not re.search(pattern, text):
        raise SystemExit(f'[smoke] generated config missing expected {label}, config={path}')
PY
}

resolve_gateway_command() {
  if [[ -n "${SMOKE_GATEWAY_CMD:-}" ]]; then
    GATEWAY_CMD=("$SMOKE_GATEWAY_CMD")
    GATEWAY_CMD_SOURCE="env"
  elif [[ -x "$LINUX_GATEWAY_BIN" ]]; then
    GATEWAY_CMD=("$LINUX_GATEWAY_BIN")
    GATEWAY_CMD_SOURCE="dist"
  else
    GATEWAY_CMD=(go run ./cmd/gateway)
    GATEWAY_CMD_SOURCE="go-run"
  fi
}

validate_smoke_config() {
  (
    cd "$SERVER_DIR"
    "${GATEWAY_CMD[@]}" validate-config -f "$GENERATED_CONFIG_PATH"
  )
}

wait_for_gateway_ready() {
  local deadline=$((SECONDS + WAIT_SECONDS))
  while (( SECONDS < deadline )); do
    if [[ -n "${GATEWAY_PID:-}" ]] && ! kill -0 "$GATEWAY_PID" >/dev/null 2>&1; then
      echo "[smoke] gateway exited early, log=$LOG_FILE"
      return 1
    fi
    if curl -fsS "$BASE_URL/healthz" | grep -q '"status":"ok"'; then
      if curl -fsS "$BASE_URL/readyz" | grep -q '"status":"ready"'; then
        return 0
      fi
    fi
    sleep 1
  done
  return 1
}

cleanup() {
  if [[ -n "${GATEWAY_PID:-}" ]] && kill -0 "$GATEWAY_PID" >/dev/null 2>&1; then
    kill "$GATEWAY_PID" >/dev/null 2>&1 || true
    wait "$GATEWAY_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

GATEWAY_PORT_EFFECTIVE="${GATEWAY_PORT:-$(get_free_tcp_port)}"
SIP_PORT_EFFECTIVE="${SMOKE_SIP_PORT:-$(get_free_tcp_port)}"
RTP_START_EFFECTIVE="${SMOKE_RTP_START:-$(get_free_udp_range_start)}"
RTP_END_EFFECTIVE="$((RTP_START_EFFECTIVE + 101))"
BASE_URL="${SMOKE_BASE_URL:-http://127.0.0.1:${GATEWAY_PORT_EFFECTIVE}}"

mkdir -p "$SMOKE_ROOT"
render_smoke_config
assert_smoke_config
new_smoke_node_config "$SMOKE_DATA_DIR/final/node_config.json" "$SIP_PORT_EFFECTIVE" "$RTP_START_EFFECTIVE" "$RTP_END_EFFECTIVE"
resolve_gateway_command
( cd "$SERVER_DIR" && go mod tidy -compat=1.23.0 )
if [[ "$GATEWAY_CMD_SOURCE" == "go-run" ]]; then
  if (( WAIT_SECONDS < 45 )); then WAIT_SECONDS=45; fi
fi
validate_smoke_config
export GATEWAY_DATA_DIR="$SMOKE_DATA_DIR"

if [[ "$START_GATEWAY" == "true" ]]; then
  echo "[smoke] starting gateway-server for smoke test..."
  echo "[smoke] config: $GENERATED_CONFIG_PATH"
  echo "[smoke] data dir: $SMOKE_DATA_DIR"
  echo "[smoke] base url: $BASE_URL"
  echo "[smoke] gateway command source: $GATEWAY_CMD_SOURCE"
  (
    cd "$SERVER_DIR"
    "${GATEWAY_CMD[@]}" --config "$GENERATED_CONFIG_PATH"
  ) >"$LOG_FILE" 2>&1 &
  GATEWAY_PID=$!

  if ! wait_for_gateway_ready; then
    echo "[smoke] gateway start timeout or readyz check failed, log=$LOG_FILE"
    exit 1
  fi
fi

(
  cd "$SERVER_DIR"
  go run ./cmd/opssmoke --base-url "$BASE_URL" --config "$GENERATED_CONFIG_PATH"
)
