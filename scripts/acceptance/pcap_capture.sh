#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACT_DIR="${PCAP_ARTIFACT_DIR:-$ROOT_DIR/artifacts/pcap}"
STATE_DIR="$ARTIFACT_DIR/state"
mkdir -p "$ARTIFACT_DIR" "$STATE_DIR"

ACTION="${1:-}"
IFACE="${PCAP_IFACE:-any}"
SIP_PORT="${PCAP_SIP_PORT:-5060}"
RTP_START="${PCAP_RTP_START:-30000}"
RTP_END="${PCAP_RTP_END:-30101}"
PEER_IP="${PCAP_PEER_IP:-}"
LABEL="${PCAP_LABEL:-phase1-strict}"
TS="$(date +%Y%m%d-%H%M%S)"
PCAP_FILE="${PCAP_FILE:-$ARTIFACT_DIR/${LABEL}-${TS}.pcap}"
PID_FILE="$STATE_DIR/${LABEL}.pid"
META_FILE="$STATE_DIR/${LABEL}.json"

if ! command -v tcpdump >/dev/null 2>&1; then
  echo "tcpdump is required" >&2
  exit 1
fi

capture_filter="(tcp port ${SIP_PORT} or udp port ${SIP_PORT} or (udp portrange ${RTP_START}-${RTP_END}))"
if [[ -n "$PEER_IP" ]]; then
  capture_filter="host ${PEER_IP} and ${capture_filter}"
fi

start_capture() {
  if [[ -f "$PID_FILE" ]] && kill -0 "$(cat "$PID_FILE")" >/dev/null 2>&1; then
    echo "capture already running: pid=$(cat "$PID_FILE")" >&2
    exit 1
  fi
  echo "starting pcap capture..."
  echo "iface=$IFACE"
  echo "pcap=$PCAP_FILE"
  echo "filter=$capture_filter"
  tcpdump -i "$IFACE" -s 0 -U -w "$PCAP_FILE" "$capture_filter" >/dev/null 2>&1 &
  pid=$!
  echo "$pid" > "$PID_FILE"
  cat > "$META_FILE" <<META
{
  "label": "$LABEL",
  "pcap_file": "$PCAP_FILE",
  "iface": "$IFACE",
  "sip_port": "$SIP_PORT",
  "rtp_port_start": "$RTP_START",
  "rtp_port_end": "$RTP_END",
  "peer_ip": "$PEER_IP",
  "filter": "$capture_filter",
  "started_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "pid": $pid
}
META
  echo "$PCAP_FILE"
}

stop_capture() {
  if [[ ! -f "$PID_FILE" ]]; then
    echo "no capture pid file: $PID_FILE" >&2
    exit 1
  fi
  pid="$(cat "$PID_FILE")"
  if kill -0 "$pid" >/dev/null 2>&1; then
    kill -INT "$pid" >/dev/null 2>&1 || kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
  rm -f "$PID_FILE"
  echo "stopped capture: ${META_FILE}"
}

case "$ACTION" in
  start) start_capture ;;
  stop) stop_capture ;;
  *)
    echo "Usage: $0 {start|stop}" >&2
    exit 2
    ;;
esac
