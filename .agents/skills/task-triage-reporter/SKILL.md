---
name: task-triage-reporter
description: Use this skill when asked to analyze blocked task cards, summarize why a queued task failed, or prepare the next follow-up task without implementing code changes.
---

# Task triage reporter

Use this skill for blocked work analysis only.

## Workflow

1. Inspect `agent/tasks/blocked/` and the corresponding `artifacts/agent/<task-id>/` evidence.
2. Read the latest `gate-results.json`, `summary.md`, and relevant logs.
3. Produce a concise failure report.
4. If needed, create a follow-up task card in `agent/tasks/backlog/`.

## Rules

- Do not modify production code when using this skill.
- Keep findings grounded in evidence.
- Prefer creating a focused follow-up task over broad catch-all tasks.
