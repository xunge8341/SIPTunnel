#!/usr/bin/env bash
set -euo pipefail

AGENT_ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
AGENT_ARTIFACT_ROOT="${AGENT_ARTIFACT_ROOT:-$AGENT_ROOT_DIR/artifacts/agent}"

agent_now() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

agent_timestamp() {
  date +%Y%m%d-%H%M%S
}

agent_ensure_dir() {
  mkdir -p "$1"
}

agent_task_value() {
  local task_file="$1"
  local expr="$2"
  python3 - "$task_file" "$expr" <<'PY'
import json, sys
from pathlib import Path
obj = json.loads(Path(sys.argv[1]).read_text(encoding='utf-8'))
expr = sys.argv[2]
cur = obj
for part in expr.split('.'):
    if part == '':
        continue
    cur = cur[part]
if isinstance(cur, list):
    print(','.join(str(x) for x in cur))
elif cur is None:
    print('')
else:
    print(cur)
PY
}

agent_task_json_pretty() {
  local task_file="$1"
  python3 - "$task_file" <<'PY'
import json, sys
from pathlib import Path
obj = json.loads(Path(sys.argv[1]).read_text(encoding='utf-8'))
print(json.dumps(obj, ensure_ascii=False, indent=2))
PY
}

agent_write_json() {
  local path="$1"
  local payload="$2"
  printf '%s\n' "$payload" > "$path"
}
