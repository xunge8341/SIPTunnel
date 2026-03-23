#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$ROOT_DIR/scripts/agent/common.sh"

TASK_FILE=""
EVIDENCE_DIR=""

usage() {
  echo "Usage: ./scripts/agent/collect-evidence.sh --task <task.json> --evidence-dir <dir>"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --task)
      TASK_FILE="$2"; shift 2 ;;
    --evidence-dir)
      EVIDENCE_DIR="$2"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "[ERR] unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

[[ -n "$TASK_FILE" ]] || { echo "[ERR] --task is required" >&2; exit 1; }
[[ -n "$EVIDENCE_DIR" ]] || { echo "[ERR] --evidence-dir is required" >&2; exit 1; }

agent_ensure_dir "$EVIDENCE_DIR/config-snapshot"
cp "$TASK_FILE" "$EVIDENCE_DIR/task-snapshot.json"
cp "$ROOT_DIR/agent/mission.json" "$EVIDENCE_DIR/config-snapshot/mission.json"
cp "$ROOT_DIR/agent/constraints.json" "$EVIDENCE_DIR/config-snapshot/constraints.json"
cp "$ROOT_DIR/agent/modules.json" "$EVIDENCE_DIR/config-snapshot/modules.json"
cp "$ROOT_DIR/agent/gates.json" "$EVIDENCE_DIR/config-snapshot/gates.json"

TASK_ID="$(agent_task_value "$TASK_FILE" id)"
TASK_TITLE="$(agent_task_value "$TASK_FILE" title)"
NOW_UTC="$(agent_now)"
PLATFORM="linux"

python3 - "$EVIDENCE_DIR" "$TASK_ID" "$TASK_TITLE" "$TASK_FILE" "$NOW_UTC" "$PLATFORM" <<'PY'
import json, sys
from pathlib import Path

ev_dir = Path(sys.argv[1])
execution = {
    'task_id': sys.argv[2],
    'task_title': sys.argv[3],
    'task_file': sys.argv[4],
    'collected_at': sys.argv[5],
    'platform': sys.argv[6],
}
(ev_dir / 'execution-summary.json').write_text(json.dumps(execution, ensure_ascii=False, indent=2), encoding='utf-8')

gate_path = ev_dir / 'gate-results.json'
gates = {}
if gate_path.exists():
    gates = json.loads(gate_path.read_text(encoding='utf-8'))
lines = [
    '# Agent Execution Summary',
    '',
    f"- TaskId: `{execution['task_id']}`",
    f"- Title: {execution['task_title']}",
    f"- Platform: `{execution['platform']}`",
    f"- CollectedAt(UTC): `{execution['collected_at']}`",
    f"- TaskSnapshot: `{(ev_dir / 'task-snapshot.json').as_posix()}`",
    f"- GateResults: `{gate_path.as_posix()}`",
    '',
]
if gates:
    lines.extend([
        '## Gate Summary',
        '',
        f"- Overall: **{gates.get('overall', 'UNKNOWN')}**",
        f"- Total: {gates.get('summary', {}).get('total', 0)}",
        f"- Pass: {gates.get('summary', {}).get('pass', 0)}",
        f"- Fail: {gates.get('summary', {}).get('fail', 0)}",
        ''
    ])
(ev_dir / 'summary.md').write_text('\n'.join(lines) + '\n', encoding='utf-8')
PY

echo "$EVIDENCE_DIR/summary.md"
echo "$EVIDENCE_DIR/execution-summary.json"
