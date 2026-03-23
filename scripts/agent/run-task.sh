#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "$ROOT_DIR/scripts/agent/common.sh"

TASK_FILE="${1:-}"
EVIDENCE_ROOT="${AGENT_ARTIFACT_ROOT:-$ROOT_DIR/artifacts/agent}"
PROFILE=""
DRY_RUN=0

usage() {
  echo "Usage: ./scripts/agent/run-task.sh <task.json> [--profile <name>] [--dry-run]"
}

if [[ -z "$TASK_FILE" || "$TASK_FILE" == "-h" || "$TASK_FILE" == "--help" ]]; then
  usage
  exit 0
fi
shift || true
while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      PROFILE="$2"; shift 2 ;;
    --dry-run)
      DRY_RUN=1; shift ;;
    *)
      echo "[ERR] unknown argument: $1" >&2; usage; exit 1 ;;
  esac
done

TASK_ID="$(agent_task_value "$TASK_FILE" id)"
STAMP="$(agent_timestamp)"
EVIDENCE_DIR="$EVIDENCE_ROOT/$TASK_ID/$STAMP"
agent_ensure_dir "$EVIDENCE_DIR"

printf '[INFO] task=%s\n' "$TASK_ID"
printf '[INFO] evidence=%s\n' "$EVIDENCE_DIR"

if [[ -n "$PROFILE" ]]; then
  "$ROOT_DIR/scripts/agent/run-gates.sh" --task "$TASK_FILE" --profile "$PROFILE" --evidence-dir "$EVIDENCE_DIR" $( [[ "$DRY_RUN" -eq 1 ]] && echo --dry-run )
else
  "$ROOT_DIR/scripts/agent/run-gates.sh" --task "$TASK_FILE" --evidence-dir "$EVIDENCE_DIR" $( [[ "$DRY_RUN" -eq 1 ]] && echo --dry-run )
fi

"$ROOT_DIR/scripts/agent/collect-evidence.sh" --task "$TASK_FILE" --evidence-dir "$EVIDENCE_DIR"

echo "$EVIDENCE_DIR"
