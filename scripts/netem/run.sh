#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MATRIX_FILE="${1:-$ROOT_DIR/gateway-server/tests/netem/matrix.json}"
OUTPUT_DIR="${NETEM_OUTPUT_DIR:-$ROOT_DIR/gateway-server/tests/netem/output}"
PROBE_COMMAND="${NETEM_PROBE_COMMAND:-}"

mkdir -p "$OUTPUT_DIR"
SAMPLES_JSONL="$OUTPUT_DIR/samples.jsonl"
: > "$SAMPLES_JSONL"

if ! command -v tc >/dev/null 2>&1; then
  echo "[WARN] tc 不存在，跳过自动注入。请按文档执行手工步骤。" >&2
  exit 0
fi

cleanup_tc() {
  local ifc="$1"
  sudo tc qdisc del dev "$ifc" root 2>/dev/null || true
}

apply_condition() {
  local ifc="$1" delay="$2" jitter="$3" loss="$4" reorder="$5" bw="$6"
  local netem=""
  [[ "$delay" != "0" ]] && netem+=" delay ${delay}ms"
  [[ "$jitter" != "0" ]] && netem+=" ${jitter}ms distribution normal"
  [[ "$loss" != "0" ]] && netem+=" loss ${loss}%"
  [[ "$reorder" != "0" ]] && netem+=" reorder ${reorder}% 50%"

  cleanup_tc "$ifc"
  if [[ -n "$netem" ]]; then
    sudo tc qdisc add dev "$ifc" root handle 1: netem $netem
  fi
  if [[ "$bw" != "0" ]]; then
    sudo tc qdisc add dev "$ifc" parent 1:1 handle 10: tbf rate "${bw}kbit" burst 32kbit latency 400ms
  fi
}

python3 - "$MATRIX_FILE" <<'PY' > "$OUTPUT_DIR/cases.tsv"
import json,sys
m=json.load(open(sys.argv[1]))
for c in m.get("cases",[]):
    cond=c.get("condition",{})
    print("\t".join([
        c.get("name",""),
        c.get("link",""),
        c.get("interface",m.get("default_interface","eth0")),
        str(c.get("target_port",0)),
        c.get("protocol","udp"),
        str(cond.get("delay_ms",0)),
        str(cond.get("jitter_ms",0)),
        str(cond.get("loss_percent",0)),
        str(cond.get("reorder_percent",0)),
        str(cond.get("disconnect_ms",0)),
        str(cond.get("bandwidth_kbps",0)),
    ]))
PY

while IFS=$'\t' read -r name link ifc port proto delay jitter loss reorder disconnect bw; do
  echo "[INFO] running case=$name link=$link if=$ifc port=$port/$proto"
  apply_condition "$ifc" "$delay" "$jitter" "$loss" "$reorder" "$bw"

  if [[ "$disconnect" != "0" ]]; then
    sudo iptables -I OUTPUT -p "$proto" --dport "$port" -j DROP
    sleep "$(awk "BEGIN {print ${disconnect}/1000}")"
    sudo iptables -D OUTPUT -p "$proto" --dport "$port" -j DROP
  fi

  if [[ -n "$PROBE_COMMAND" ]]; then
    NETEM_CASE="$name" NETEM_LINK="$link" NETEM_PORT="$port" NETEM_PROTO="$proto" \
      NETEM_DELAY_MS="$delay" NETEM_JITTER_MS="$jitter" NETEM_LOSS_PERCENT="$loss" \
      NETEM_REORDER_PERCENT="$reorder" NETEM_DISCONNECT_MS="$disconnect" NETEM_BANDWIDTH_KBPS="$bw" \
      bash -lc "$PROBE_COMMAND" >> "$SAMPLES_JSONL"
  else
    cat >> "$SAMPLES_JSONL" <<JSON
{"link":"$link","scenario":"$name","condition":{"delay_ms":$delay,"jitter_ms":$jitter,"loss_percent":$loss,"reorder_percent":$reorder,"disconnect_ms":$disconnect,"bandwidth_kbps":$bw},"attempts":0,"successes":0,"avg_latency_ms":0,"retransmissions":0,"recovery_time_ms":0,"manual_validation":["设置 NETEM_PROBE_COMMAND 以接入自动探测并输出 JSONL","当前样本由框架占位生成，需按 docs/network-degradation-testing.md 手工验证"]}
JSON
  fi

  cleanup_tc "$ifc"
done < "$OUTPUT_DIR/cases.tsv"

cd "$ROOT_DIR/gateway-server"
go run ./cmd/netemreport -input "$SAMPLES_JSONL" -template "$ROOT_DIR/gateway-server/tests/netem/report_template.md" -out "$OUTPUT_DIR/report.md"
echo "[INFO] report generated: $OUTPUT_DIR/report.md"
