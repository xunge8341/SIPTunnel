# 任务卡

任务卡使用 JSON，便于 Linux shell / Windows PowerShell 统一解析。

建议字段：

- `id`：任务唯一编号
- `title`：任务标题
- `goal`：任务目标数组
- `scope.include`：允许改动范围
- `scope.exclude`：禁止改动范围
- `required_gates`：必须执行的门禁数组
- `success_criteria`：成功标准数组
- `priority`：P0/P1/P2
- `risk`：low/medium/high
- `status`：backlog/active/done/blocked

执行入口：

- Linux: `./scripts/agent/run-task.sh <task.json>`
- Windows: `./scripts/agent/run-task.ps1 -TaskFile <task.json>`


推荐先阅读：

- `agent/tasks/TASK_INDEX.md`：任务卡执行顺序与依赖关系
