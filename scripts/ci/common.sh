#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACT_DIR="${ARTIFACT_DIR:-$ROOT_DIR/artifacts/ci}"
LOG_DIR="$ARTIFACT_DIR/logs"
REPORT_DIR="$ARTIFACT_DIR/reports"
METRICS_DIR="$ARTIFACT_DIR/metrics"

mkdir -p "$LOG_DIR" "$REPORT_DIR" "$METRICS_DIR"

SEP=$'\x1f'
RESULTS=()
FAILURES=0

run_gate() {
  local name="$1"
  shift
  local logfile="$LOG_DIR/${name}.log"
  local start end elapsed status summary

  start="$(date +%s)"
  if (
    cd "$ROOT_DIR"
    "$@"
  ) >"$logfile" 2>&1; then
    status="PASS"
    summary=""
  else
    status="FAIL"
    summary="$(tail -n 40 "$logfile")"
    FAILURES=$((FAILURES + 1))
  fi
  end="$(date +%s)"
  elapsed=$((end - start))

  echo "[$status] $name (${elapsed}s)"
  if [[ "$status" == "FAIL" ]]; then
    printf '%s\n' "$summary"
  fi

  local summary_b64
  summary_b64="$(printf '%s' "$summary" | base64 | tr -d '\n')"
  RESULTS+=("${name}${SEP}${status}${SEP}${elapsed}${SEP}${*}${SEP}${summary_b64}")
}

write_report() {
  local suite="$1"
  local report_json="$REPORT_DIR/${suite}-report.json"
  local report_md="$REPORT_DIR/${suite}-report.md"

  RESULTS_PAYLOAD="$(printf '%s\n' "${RESULTS[@]}")"
  export RESULTS_PAYLOAD report_json report_md suite FAILURES

  python3 - <<'PY'
import base64
import json
import os
from datetime import datetime, timezone

sep = "\x1f"
rows = [r for r in os.environ["RESULTS_PAYLOAD"].splitlines() if r.strip()]
items = []
for row in rows:
    name, status, elapsed, command, summary_b64 = row.split(sep)
    items.append({
        "name": name,
        "status": status,
        "duration_sec": int(elapsed),
        "command": command,
        "failure_summary": base64.b64decode(summary_b64.encode()).decode("utf-8", errors="replace") if summary_b64 else "",
    })

overall = "PASS" if int(os.environ["FAILURES"]) == 0 else "FAIL"
report = {
    "suite": os.environ["suite"],
    "generated_at": datetime.now(timezone.utc).isoformat(),
    "overall": overall,
    "summary": {
        "total": len(items),
        "pass": sum(1 for i in items if i["status"] == "PASS"),
        "fail": sum(1 for i in items if i["status"] == "FAIL"),
    },
    "items": items,
}

with open(os.environ["report_json"], "w", encoding="utf-8") as f:
    json.dump(report, f, ensure_ascii=False, indent=2)

lines = [
    f"# {os.environ['suite']} 报告",
    "",
    f"- GeneratedAt(UTC): `{report['generated_at']}`",
    f"- Overall: **{overall}**",
    "",
    "| Gate | Status | Duration(s) | Command | Fail Summary |",
    "|---|---|---:|---|---|",
]
for item in items:
    fail = item["failure_summary"].replace("\n", "<br>") if item["failure_summary"] else "-"
    lines.append(f"| `{item['name']}` | {item['status']} | {item['duration_sec']} | `{item['command']}` | {fail} |")

with open(os.environ["report_md"], "w", encoding="utf-8") as f:
    f.write("\n".join(lines) + "\n")
PY

  echo "report: $report_md"
  echo "report: $report_json"
}
