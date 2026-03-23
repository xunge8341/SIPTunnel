#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
QUEUE_DIR="$ROOT_DIR/agent/tasks"
LOCK_FILE="$QUEUE_DIR/.queue.lock"
TASK_FILE=""
EVIDENCE_DIR=""

usage() {
  echo "Usage: ./scripts/agent/finalize-task.sh --task <task.json> --evidence-dir <dir>"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --task) TASK_FILE="$2"; shift 2 ;;
    --evidence-dir) EVIDENCE_DIR="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "[ERR] unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

[[ -n "$TASK_FILE" ]] || { echo "[ERR] --task is required" >&2; exit 1; }
[[ -n "$EVIDENCE_DIR" ]] || { echo "[ERR] --evidence-dir is required" >&2; exit 1; }

python3 - "$ROOT_DIR" "$TASK_FILE" "$EVIDENCE_DIR" "$LOCK_FILE" <<'PY'
import json, sys
from pathlib import Path
from datetime import datetime, timezone

root = Path(sys.argv[1])
task_path = Path(sys.argv[2])
if not task_path.is_absolute():
    task_path = root / task_path

ev_dir = Path(sys.argv[3])
lock_file = Path(sys.argv[4])
report = json.loads((ev_dir / 'gate-results.json').read_text(encoding='utf-8'))
overall = report.get('overall', 'FAIL')
dest_dir = root / 'agent' / 'tasks' / ('done' if overall == 'PASS' else 'blocked')
dest_dir.mkdir(parents=True, exist_ok=True)

data = json.loads(task_path.read_text(encoding='utf-8'))
data['status'] = 'done' if overall == 'PASS' else 'blocked'
data['last_execution'] = {
    'overall': overall,
    'evidence_dir': str(ev_dir.relative_to(root)).replace('\\', '/'),
    'finalized_at': datetime.now(timezone.utc).isoformat(),
}

dest_path = dest_dir / task_path.name
dest_path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
if task_path.exists() and task_path != dest_path:
    task_path.unlink()

summary = {
    'task_id': data.get('id', dest_path.stem),
    'final_status': data['status'],
    'task_file': str(dest_path.relative_to(root)).replace('\\', '/'),
    'evidence_dir': str(ev_dir.relative_to(root)).replace('\\', '/'),
    'overall': overall,
    'finalized_at': data['last_execution']['finalized_at'],
}
(ev_dir / 'finalize-summary.json').write_text(json.dumps(summary, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
lines = [
    '# Agent Finalize Summary',
    '',
    f"- TaskId: `{summary['task_id']}`",
    f"- FinalStatus: **{summary['final_status']}**",
    f"- Overall: **{summary['overall']}**",
    f"- TaskFile: `{summary['task_file']}`",
    f"- EvidenceDir: `{summary['evidence_dir']}`",
    f"- FinalizedAt(UTC): `{summary['finalized_at']}`",
    '',
]
(ev_dir / 'finalize-summary.md').write_text('\n'.join(lines) + '\n', encoding='utf-8')
if lock_file.exists():
    lock_file.unlink()
print(str(dest_path.relative_to(root)).replace('\\', '/'))
PY
