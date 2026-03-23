#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SERVER_DIR="$ROOT_DIR/gateway-server"
PROFILE="${1:-local}"
REPORT_DIR="${REGRESSION_REPORT_DIR:-$ROOT_DIR/artifacts/regression}"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
REPORT_BASENAME="regression-${PROFILE}-${TIMESTAMP}"
REPORT_JSON="$REPORT_DIR/${REPORT_BASENAME}.json"
REPORT_MD="$REPORT_DIR/${REPORT_BASENAME}.md"

mkdir -p "$REPORT_DIR"

if [[ ! -d "$SERVER_DIR" ]]; then
  echo "gateway-server directory not found: $SERVER_DIR" >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "go command is required" >&2
  exit 1
fi

CMD_command_chain_local=(go test ./internal/service/sipcontrol -run 'TestDispatcherRouteCommandCreate|TestHandlersAcceptedFlow' -count=1)
CMD_command_chain_smoke=(go test ./internal/service/sipcontrol -run 'TestDispatcherRouteCommandCreate|TestDispatcherRejectsBadSignature|TestHandlersAcceptedFlow' -count=1)
CMD_command_chain_full=(go test ./internal/service/sipcontrol -count=1)

CMD_file_chain_local=(go test ./internal/service/filetransfer -run 'TestReceiverCollectComplete|TestReceiverWithConfiguredPaths' -count=1)
CMD_file_chain_smoke=(go test ./internal/service/filetransfer -run 'TestReceiverCollectComplete|TestReceiverOutOfOrderComplete|TestReceiverDetectMissingAndRetransmit' -count=1)
CMD_file_chain_full=(go test ./internal/service/filetransfer -count=1)

CMD_sip_tcp_local=(go test ./internal/service/siptcp -run 'TestFramerHalfAndStickyPackets' -count=1)
CMD_sip_tcp_smoke=(go test ./internal/service/siptcp -run 'TestServerClientIntegration|TestFramerHalfAndStickyPackets' -count=1)
CMD_sip_tcp_full=(go test ./internal/service/siptcp -count=1)

CMD_rtp_udp_local=(go test ./internal/service/filetransfer -run 'TestMarshalUnmarshalChunkPacketRoundTrip' -count=1)
CMD_rtp_udp_smoke=(go test ./internal/service/filetransfer -run 'TestMarshalUnmarshalChunkPacketRoundTrip|TestReceiverDetectMissingAndRetransmit' -count=1)
CMD_rtp_udp_full=(go test ./internal/service/filetransfer -run 'TestMarshalUnmarshalChunkPacketRoundTrip|TestReceiverDetectMissingAndRetransmit|TestReceiverDuplicateChunkDeduplicate' -count=1)

CMD_rtp_tcp_local=(go test ./internal/service/filetransfer -run 'TestNewTransportTCPBootstrapAndSessionLifecycle' -count=1)
CMD_rtp_tcp_smoke=(go test ./internal/service/filetransfer -run 'TestNewTransportTCPBootstrapAndSessionLifecycle|TestTCPSessionWriteReadPacket|TestTCPTransportSessionWithReceiverIntegration' -count=1)
CMD_rtp_tcp_full=(go test ./internal/service/filetransfer -run 'TestNewTransportTCPBootstrapAndSessionLifecycle|TestTCPSessionWriteReadPacket|TestTCPTransportSessionWithReceiverIntegration|TestTCPSessionReadErrorMetrics|TestTCPSessionWriteErrorMetrics' -count=1)

CMD_config_validate_local=(go test ./internal/config -run 'TestDefaultNetworkConfig|TestConfigYAMLSample_NetworkSectionValid' -count=1)
CMD_config_validate_smoke=(go test ./internal/config -run 'TestParseNetworkConfigYAML|TestConfigYAMLSample_NetworkSectionValid|TestSIPConfigUDPMessageSizeRisk' -count=1)
CMD_config_validate_full=(go test ./internal/config ./cmd/gatewayctl -run 'TestParseNetworkConfigYAML|TestConfigYAMLSample_NetworkSectionValid|TestValidateConfig' -count=1)

CMD_selfcheck_local=(go test ./internal/selfcheck -run 'TestRunnerRun_AllPass' -count=1)
CMD_selfcheck_smoke=(go test ./internal/selfcheck -run 'TestRunnerRun_AllPass|TestRunnerRun_ErrorsAndWarns|TestRunnerRun_RTPTCPReservedWarn' -count=1)
CMD_selfcheck_full=(go test ./internal/selfcheck -count=1)

CMD_api_smoke_local=(go test ./internal/server -run 'TestHealthz|TestSelfCheckEndpoint' -count=1)
CMD_api_smoke_smoke=(go test ./internal/server -run 'TestHealthz|TestSelfCheckEndpoint|TestNodeNetworkStatusEndpointAndAudit|TestTasksListAndGetWithFilters' -count=1)
CMD_api_smoke_full=(go test ./internal/server -count=1)

CMD_repo_full=(go test ./... -count=1)

case "$PROFILE" in
  local|smoke|full) ;;
  *)
    echo "Usage: $0 [local|smoke|full]" >&2
    exit 2
    ;;
esac

results=()
failures=0
SEP=$'\x1f'

run_item() {
  local name="$1"
  shift
  local cmd=("$@")
  local tmp
  tmp="$(mktemp)"
  local start end elapsed status summary output

  start="$(date +%s)"
  if (
    cd "$SERVER_DIR"
    "${cmd[@]}"
  ) >"$tmp" 2>&1; then
    status="PASS"
    summary=""
  else
    status="FAIL"
    failures=$((failures + 1))
    summary="$(tail -n 20 "$tmp")"
  fi
  end="$(date +%s)"
  elapsed=$((end - start))

  output="$(cat "$tmp")"
  rm -f "$tmp"

  printf '[%s] %s (%ss)\n' "$status" "$name" "$elapsed"
  if [[ "$status" == "FAIL" ]]; then
    printf '%s\n' "$summary"
  fi

  local output_b64 summary_b64
  output_b64="$(printf '%s' "$output" | base64 | tr -d '\n')"
  summary_b64="$(printf '%s' "$summary" | base64 | tr -d '\n')"
  results+=("${name}${SEP}${status}${SEP}${elapsed}${SEP}${cmd[*]}${SEP}${summary_b64}${SEP}${output_b64}")
}

has_rtp_tcp_tests="0"
if rg -n "TestTCPTransportSessionWithReceiverIntegration" "$SERVER_DIR/internal/service/filetransfer/transport_integration_test.go" >/dev/null 2>&1; then
  has_rtp_tcp_tests="1"
fi

ITEMS=(command_chain file_chain sip_tcp rtp_udp config_validate selfcheck api_smoke)
if [[ "$has_rtp_tcp_tests" == "1" ]]; then
  ITEMS+=(rtp_tcp)
fi
if [[ "$PROFILE" == "full" ]]; then
  ITEMS+=(repo_full)
fi

for item in "${ITEMS[@]}"; do
  if [[ "$item" == "repo_full" ]]; then
    run_item "$item" "${CMD_repo_full[@]}"
    continue
  fi
  key="CMD_${item}_${PROFILE}[@]"
  run_item "$item" "${!key}"
done

total="${#results[@]}"
pass_count=$((total - failures))
overall="PASS"
if [[ "$failures" -gt 0 ]]; then
  overall="FAIL"
fi

RESULTS_PAYLOAD="$(printf '%s\n' "${results[@]}")"
export RESULTS_PAYLOAD PROFILE REPORT_JSON REPORT_MD overall pass_count failures total SEP
python - <<'PY'
import base64, json, os
from datetime import datetime, timezone

sep = os.environ["SEP"]
raw = os.environ.get("RESULTS_PAYLOAD", "")
items = []
for line in raw.splitlines():
    if not line:
        continue
    name, status, duration, command, summary_b64, output_b64 = line.split(sep)
    summary = base64.b64decode(summary_b64).decode("utf-8", errors="replace") if summary_b64 else ""
    output = base64.b64decode(output_b64).decode("utf-8", errors="replace") if output_b64 else ""
    items.append({
        "name": name,
        "status": status,
        "duration_sec": int(duration),
        "command": command,
        "failure_summary": summary,
        "output": output,
    })

report = {
    "profile": os.environ["PROFILE"],
    "generated_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "overall": os.environ["overall"],
    "summary": {
        "total": int(os.environ["total"]),
        "pass": int(os.environ["pass_count"]),
        "fail": int(os.environ["failures"]),
    },
    "items": items,
}

with open(os.environ["REPORT_JSON"], "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)

lines = [
    "# SIPTunnel 发布前回归报告",
    "",
    f"- Profile: `{report['profile']}`",
    f"- GeneratedAt(Local): `{report['generated_at']}`",
    f"- Overall: **{report['overall']}**",
    "",
    "| 测试项 | 结果 | 耗时(秒) | 失败摘要 |",
    "|---|---|---:|---|",
]
for item in items:
    summary = item["failure_summary"].replace("\n", "<br>") if item["failure_summary"] else "-"
    lines.append(f"| `{item['name']}` | {item['status']} | {item['duration_sec']} | {summary} |")

lines += ["", f"JSON: `{os.environ['REPORT_JSON']}`", ""]

with open(os.environ["REPORT_MD"], "w", encoding="utf-8") as f:
    f.write("\n".join(lines))
PY

echo "Regression report generated:"
echo "- $REPORT_MD"
echo "- $REPORT_JSON"

if [[ "$overall" == "FAIL" ]]; then
  exit 1
fi
