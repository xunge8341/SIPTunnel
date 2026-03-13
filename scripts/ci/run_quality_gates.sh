#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$ROOT_DIR/scripts/ci/common.sh"

BENCH_LOG="$LOG_DIR/benchmark_smoke.log"

run_gate "unit_tests" bash -lc 'cd gateway-server && go test ./... -short -count=1'
run_gate "protocol_codec_tests" bash -lc "cd gateway-server && go test ./internal/protocol/sip ./internal/protocol/rtpfile ./internal/rtp ./internal/control -count=1"
run_gate "e2e_smoke" bash -lc 'cd scripts && ./regression/run.sh smoke'
run_gate "network_config_validation" bash -lc 'cd gateway-server && go test ./internal/config ./internal/selfcheck -run "Test(ParseNetworkConfigYAML|ConfigYAMLSample_NetworkSectionValid|SIPConfigUDPMessageSizeRisk|RunnerRun_AllPass|RunnerRun_RTPTCPReservedWarn)" -count=1 && cd .. && LISTEN_PORT=18080 MEDIA_PORT_START=20000 MEDIA_PORT_END=20100 NODE_ROLE=receiver ./scripts/preflight.sh'
run_gate "benchmark_smoke" bash -lc "cd gateway-server && GOMAXPROCS=1 go test ./internal/protocol/sip ./internal/security ./internal/rtp ./internal/protocol/rtpfile ./internal/service/httpinvoke -run '^$' -bench 'Benchmark(SIPJSONDecodeValidate|Signer(Sign|Verify)|RTPHeader(Encode|Decode)|File(Split|Assemble)|HTTP(MapByTemplate|InvokeWrapper))$' -benchmem -benchtime=100ms -count=1"

BENCH_LOG_PATH="$BENCH_LOG" python3 - <<'PY' > "$METRICS_DIR/benchmark-smoke-snapshot.json"
import json
import os
import pathlib
import re

log = pathlib.Path(os.environ['BENCH_LOG_PATH'])
items = []
if log.exists():
    for line in log.read_text(encoding="utf-8", errors="replace").splitlines():
        m = re.match(r'^(Benchmark\S+)\s+\d+\s+([0-9.]+) ns/op(?:\s+([0-9.]+) B/op\s+([0-9.]+) allocs/op)?', line.strip())
        if not m:
            continue
        items.append({
            "benchmark": m.group(1),
            "ns_per_op": float(m.group(2)),
            "bytes_per_op": float(m.group(3)) if m.group(3) else None,
            "allocs_per_op": float(m.group(4)) if m.group(4) else None,
        })
print(json.dumps({"benchmarks": items}, ensure_ascii=False, indent=2))
PY

LOG_DIR_PATH="$LOG_DIR" python3 - <<'PY' > "$REPORT_DIR/log-summary.md"
from pathlib import Path
import os

log_dir = Path(os.environ['LOG_DIR_PATH'])
lines = ["# CI 日志摘要", ""]
for f in sorted(log_dir.glob("*.log")):
    txt = f.read_text(encoding="utf-8", errors="replace").splitlines()
    lines.append(f"## {f.name}")
    if not txt:
        lines.append("- 空日志")
    else:
        tail = txt[-20:]
        lines.append("```text")
        lines.extend(tail)
        lines.append("```")
    lines.append("")
print("\n".join(lines))
PY

write_report "ci-quality-gates"

if [[ "$FAILURES" -gt 0 ]]; then
  exit 1
fi
