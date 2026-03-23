Use the `task-card-executor` skill and follow repository `AGENTS.md` rules.

1. Claim exactly one queued task by running the repository queue script.
2. If no task is available or the queue is locked, stop and report that nothing was executed.
3. Read the claimed task card fully.
4. Implement only that one task.
5. Run all required gates.
6. Collect evidence.
7. Finalize the task as done if gates pass, or blocked if any required gate fails.
8. Stop. Do not continue to another task in the same run.
