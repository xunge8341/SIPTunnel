# Codex app automation notes

This repository is prepared for Codex app Automations.

## Recommended setup

- Run automations in a dedicated worktree.
- Keep the Codex app running while automations are scheduled.
- Ensure this project directory stays available on disk.
- Use the prompt text from `.codex/prompts/automation-run-next-task.md`.
- For blocked-task analysis, use `.codex/prompts/triage-blocked-task.md`.

## Preferred repository entrypoints

- Linux/macOS: `./scripts/agent/run-automation-cycle.sh`
- Windows PowerShell: `./scripts/agent/run-automation-cycle.ps1`

## Queue behavior

- The queue processes one task card per run.
- Ordering is `P0 > P1 > P2`.
- Dependencies in `depends_on` must already be complete.
- A queue lock prevents overlapping automation runs.
