#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CLAIMED_TASK="$($ROOT_DIR/scripts/agent/claim-next-task.sh || true)"
CLAIMED_TASK="${CLAIMED_TASK//$'\r'/}"

if [[ -z "$CLAIMED_TASK" ]]; then
  echo "[INFO] no eligible task claimed"
  exit 0
fi

TASK_PATH="$ROOT_DIR/$CLAIMED_TASK"
EVIDENCE_DIR="$($ROOT_DIR/scripts/agent/run-task.sh "$TASK_PATH")"
$ROOT_DIR/scripts/agent/finalize-task.sh --task "$TASK_PATH" --evidence-dir "$EVIDENCE_DIR" >/dev/null
printf '%s\n' "$CLAIMED_TASK"
printf '%s\n' "$EVIDENCE_DIR"
