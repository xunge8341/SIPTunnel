---
name: task-card-executor
description: Use this skill when asked to execute the next queued repository task card, run Codex app automations in order, or process one task card end-to-end with gates and evidence. Do not use it for ad hoc coding unrelated to the repository queue.
---

# Task card executor

This repository uses machine-readable task cards under `agent/tasks/` and deterministic scripts under `scripts/agent/`.

## Use this skill when

- The user asks to execute queued work in order.
- The user asks to run the next task card.
- A Codex app automation prompt asks for one queued task to be processed.

## Workflow

1. Claim exactly one task.
   - Linux/macOS: `./scripts/agent/claim-next-task.sh`
   - Windows: `./scripts/agent/claim-next-task.ps1`
2. Read the claimed task card completely.
3. Make only the code changes needed for that one task.
4. Run the task through:
   - Linux/macOS: `./scripts/agent/run-task.sh <task-file>`
   - Windows: `./scripts/agent/run-task.ps1 -TaskFile <task-file>`
5. Finalize the task.
   - Linux/macOS: `./scripts/agent/finalize-task.sh --task <task-file> --evidence-dir <dir>`
   - Windows: `./scripts/agent/finalize-task.ps1 -TaskFile <task-file> -EvidenceDir <dir>`
6. Stop. Never continue to another queued task in the same run.

## Required behavior

- Respect `scope.include` and `scope.exclude`.
- Run all `required_gates`.
- Save evidence under `artifacts/agent/<task-id>/<timestamp>/`.
- If the queue is locked, stop and report that no work was claimed.
- If a gate fails, finalize the task as blocked and stop.

## Outputs

Each run should leave:
- code changes for one task only
- `gate-results.json`
- `summary.md`
- final task movement into `done/` or `blocked/`
