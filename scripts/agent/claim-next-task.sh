#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
QUEUE_DIR="$ROOT_DIR/agent/tasks"
LOCK_FILE="$QUEUE_DIR/.queue.lock"

if [[ -f "$LOCK_FILE" ]]; then
  echo "[LOCKED] queue lock exists: $LOCK_FILE" >&2
  cat "$LOCK_FILE" >&2 || true
  exit 3
fi

python3 - "$ROOT_DIR" "$LOCK_FILE" <<'PY'
import json, os, sys
from pathlib import Path
from datetime import datetime, timezone

root = Path(sys.argv[1])
lock_file = Path(sys.argv[2])
queue_dir = root / 'agent' / 'tasks'
in_progress_dir = queue_dir / 'in_progress'
done_ids = {p.stem for p in (queue_dir / 'done').glob('*.json')}
priority_rank = {'P0': 0, 'P1': 1, 'P2': 2}

existing = sorted(in_progress_dir.glob('*.json'))
if existing:
    chosen = existing[0]
    lock_file.write_text(json.dumps({
        'task_file': str(chosen.relative_to(root)).replace('\\', '/'),
        'task_id': chosen.stem,
        'locked_at': datetime.now(timezone.utc).isoformat(),
        'reason': 'resume-in-progress'
    }, ensure_ascii=False, indent=2), encoding='utf-8')
    print(str(chosen.relative_to(root)).replace('\\', '/'))
    raise SystemExit(0)

candidates = []
for state in ('active', 'backlog'):
    for path in sorted((queue_dir / state).glob('*.json')):
        try:
            data = json.loads(path.read_text(encoding='utf-8'))
        except Exception:
            continue
        deps = set(data.get('depends_on') or [])
        if not deps.issubset(done_ids):
            continue
        pr = priority_rank.get(data.get('priority', 'P9'), 9)
        queue_order = int(data.get('queue_order', 999999))
        # active tasks stay within the same priority bucket but sort ahead of backlog.
        state_rank = 0 if state == 'active' else 1
        candidates.append((pr, queue_order, state_rank, path.name, path, data))

if not candidates:
    print('')
    raise SystemExit(0)

_, _, _, _, chosen_path, data = sorted(candidates)[0]
target = in_progress_dir / chosen_path.name
target.parent.mkdir(parents=True, exist_ok=True)
data['status'] = 'in_progress'
target.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
chosen_path.unlink()
lock_file.write_text(json.dumps({
    'task_file': str(target.relative_to(root)).replace('\\', '/'),
    'task_id': data.get('id', target.stem),
    'locked_at': datetime.now(timezone.utc).isoformat(),
    'priority': data.get('priority'),
}, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
print(str(target.relative_to(root)).replace('\\', '/'))
PY
