#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$ROOT_DIR/scripts/agent/common.sh"

TASK_FILE=""
EVIDENCE_DIR=""
PROFILE=""
DRY_RUN=0
GATE_NAMES=()

usage() {
  cat <<USAGE
Usage:
  ./scripts/agent/run-gates.sh [--task <task.json>] [--evidence-dir <dir>] [--profile <name>] [--dry-run] [gate1 gate2 ...]
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --task)
      TASK_FILE="$2"; shift 2 ;;
    --evidence-dir)
      EVIDENCE_DIR="$2"; shift 2 ;;
    --profile)
      PROFILE="$2"; shift 2 ;;
    --dry-run)
      DRY_RUN=1; shift ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      GATE_NAMES+=("$1"); shift ;;
  esac
done

if [[ -z "$EVIDENCE_DIR" ]]; then
  EVIDENCE_DIR="$AGENT_ARTIFACT_ROOT/manual/$(agent_timestamp)"
fi
agent_ensure_dir "$EVIDENCE_DIR/logs"
agent_ensure_dir "$EVIDENCE_DIR/gates"

if [[ ${#GATE_NAMES[@]} -eq 0 && -n "$TASK_FILE" ]]; then
  IFS=',' read -r -a GATE_NAMES <<< "$(agent_task_value "$TASK_FILE" required_gates)"
fi

if [[ ${#GATE_NAMES[@]} -eq 0 && -n "$PROFILE" ]]; then
  mapfile -t GATE_NAMES < <(python3 - "$ROOT_DIR/agent/gates.json" "$PROFILE" <<'PY'
import json, sys
from pathlib import Path
obj = json.loads(Path(sys.argv[1]).read_text(encoding='utf-8'))
for item in obj.get('default_profiles', {}).get(sys.argv[2], []):
    print(item)
PY
)
fi

if [[ ${#GATE_NAMES[@]} -eq 0 ]]; then
  mapfile -t GATE_NAMES < <(python3 - "$ROOT_DIR/agent/gates.json" <<'PY'
import json, sys
from pathlib import Path
obj = json.loads(Path(sys.argv[1]).read_text(encoding='utf-8'))
for item in obj.get('default_profiles', {}).get('default', []):
    print(item)
PY
)
fi

RESULTS=()
FAILURES=0
PLATFORM="linux"

run_gate() {
  local name="$1"
  shift
  local gate_dir="$EVIDENCE_DIR/gates/$name"
  local log_file="$EVIDENCE_DIR/logs/${name}.log"
  agent_ensure_dir "$gate_dir"
  local start end elapsed status summary
  start="$(date +%s)"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[DRY-RUN] $name :: $*" | tee "$log_file"
    status="PASS"
    summary=""
  elif (
    cd "$ROOT_DIR"
    "$@"
  ) >"$log_file" 2>&1; then
    status="PASS"
    summary=""
  else
    status="FAIL"
    summary="$(tail -n 40 "$log_file" || true)"
    FAILURES=$((FAILURES + 1))
  fi
  end="$(date +%s)"
  elapsed=$((end - start))
  echo "[$status] $name (${elapsed}s)"
  local summary_b64
  summary_b64="$(printf '%s' "$summary" | base64 | tr -d '\n')"
  RESULTS+=("$name|$status|$elapsed|$log_file|$summary_b64")
}

run_named_gate() {
  local gate="$1"
  local gate_dir="$EVIDENCE_DIR/gates/$gate"
  case "$gate" in
    repo_consistency)
      run_gate "$gate" env ARTIFACT_DIR="$gate_dir" bash -lc 'bash ./scripts/check-consistency.sh'
      ;;
    strict_source)
      run_gate "$gate" env ACCEPTANCE_REPORT_DIR="$gate_dir" bash -lc 'bash ./scripts/acceptance/verify_phase1_strict_source.sh'
      ;;
    smoke)
      run_gate "$gate" env SMOKE_WORK_DIR="$gate_dir/work" SMOKE_LOG_FILE="$gate_dir/smoke.log" bash -lc 'bash ./scripts/smoke/run.sh'
      ;;
    longrun_smoke)
      run_gate "$gate" env LONGRUN_REPORT_DIR="$gate_dir/longrun" bash -lc 'bash ./scripts/longrun/run.sh smoke'
      ;;
    ci_quality)
      run_gate "$gate" env ARTIFACT_DIR="$gate_dir" bash -lc 'bash ./scripts/ci/run_quality_gates.sh'
      ;;
    ui_delivery_guard)
      run_gate "$gate" bash -lc 'node ./scripts/ui-delivery-guard.mjs'
      ;;
    netem_smoke)
      run_gate "$gate" env NETEM_REPORT_DIR="$gate_dir" bash -lc 'bash ./scripts/netem/run.sh smoke'
      ;;
    *)
      echo "[ERR] unsupported gate: $gate" >&2
      FAILURES=$((FAILURES + 1))
      RESULTS+=("$gate|FAIL|0|unsupported|$(printf 'unsupported gate' | base64 | tr -d '\n')")
      ;;
  esac
}

for gate in "${GATE_NAMES[@]}"; do
  [[ -n "$gate" ]] || continue
  run_named_gate "$gate"
done

RESULTS_PAYLOAD="$(printf '%s\n' "${RESULTS[@]}")"
export RESULTS_PAYLOAD EVIDENCE_DIR FAILURES PLATFORM TASK_FILE PROFILE
python3 - <<'PY'
import base64, json, os
from datetime import datetime, timezone
from pathlib import Path

rows = [r for r in os.environ.get('RESULTS_PAYLOAD', '').splitlines() if r.strip()]
items = []
for row in rows:
    name, status, elapsed, log_path, summary_b64 = row.split('|', 4)
    items.append({
        'name': name,
        'status': status,
        'duration_sec': int(elapsed),
        'log_path': log_path,
        'failure_summary': base64.b64decode(summary_b64.encode()).decode('utf-8', errors='replace') if summary_b64 else ''
    })
report = {
    'generated_at': datetime.now(timezone.utc).isoformat(),
    'platform': os.environ.get('PLATFORM', 'linux'),
    'task_file': os.environ.get('TASK_FILE') or None,
    'profile': os.environ.get('PROFILE') or None,
    'overall': 'PASS' if int(os.environ.get('FAILURES', '0')) == 0 else 'FAIL',
    'summary': {
        'total': len(items),
        'pass': sum(1 for i in items if i['status'] == 'PASS'),
        'fail': sum(1 for i in items if i['status'] == 'FAIL'),
    },
    'items': items,
}
out_dir = Path(os.environ['EVIDENCE_DIR'])
out_dir.mkdir(parents=True, exist_ok=True)
(out_dir / 'gate-results.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
lines = [
    '# Agent Gate Results',
    '',
    f"- GeneratedAt(UTC): `{report['generated_at']}`",
    f"- Platform: `{report['platform']}`",
    f"- Overall: **{report['overall']}**",
    '',
    '| Gate | Status | Duration(s) | Log | Fail Summary |',
    '|---|---|---:|---|---|',
]
for item in items:
    fail = item['failure_summary'].replace('\n', '<br>') if item['failure_summary'] else '-'
    lines.append(f"| `{item['name']}` | {item['status']} | {item['duration_sec']} | `{item['log_path']}` | {fail} |")
(out_dir / 'gate-results.md').write_text('\n'.join(lines) + '\n', encoding='utf-8')
PY

echo "$EVIDENCE_DIR/gate-results.md"
echo "$EVIDENCE_DIR/gate-results.json"

if [[ "$FAILURES" -gt 0 ]]; then
  exit 1
fi
