# Repository automation rules for Codex

Codex reads this file before starting work. Follow these rules for every run in this repository.

## Primary goal

Use repository task cards and automation scripts to complete exactly one queued task per run. Respect task scope, required gates, and evidence rules.

## Queue discipline

- Always claim work through `scripts/agent/claim-next-task.sh` on Linux/macOS or `scripts/agent/claim-next-task.ps1` on Windows.
- Never pick a task card manually when the request is to execute queued work in order.
- Process at most one task card per run.
- If a task is already in `agent/tasks/in_progress/`, continue that task instead of claiming a different one.
- Respect dependency ordering and priority ordering. The queue scripts enforce `P0 > P1 > P2` and only release tasks whose `depends_on` items are already in `agent/tasks/done/`.

## Execution rules

- Read the claimed task card fully before making changes.
- Modify code only inside the task card `scope.include` paths. Treat `scope.exclude` as off limits.
- Run every required gate listed in `required_gates`.
- Save evidence under `artifacts/agent/<task-id>/<timestamp>/`.
- Do not start a second task after finishing the first one.

## Finalization rules

- Use `scripts/agent/finalize-task.sh` or `scripts/agent/finalize-task.ps1` after each run.
- If all required gates pass, move the task to `agent/tasks/done/` and write a success summary.
- If any required gate fails, move the task to `agent/tasks/blocked/` and write a failure summary.
- Always remove the queue lock through the finalize script.

## Safety rules

- Do not bypass gates.
- Do not delete or weaken tests to make gates pass.
- Do not modify generated UI delivery artifacts unless the task explicitly requires that workflow.
- Do not add production dependencies without task-card justification.
- If the queue is locked, stop and report the lock instead of editing files.

## Preferred automation entrypoints

- Linux/macOS: `./scripts/agent/run-automation-cycle.sh`
- Windows PowerShell: `./scripts/agent/run-automation-cycle.ps1`

## Preferred Codex app setup

- Use a Codex app Automation that runs in a dedicated worktree.
- Keep the Codex app running and keep the selected project available on disk.
- Point the automation prompt at `.codex/prompts/automation-run-next-task.md`.
