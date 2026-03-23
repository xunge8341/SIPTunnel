# agent 层

本目录用于把仓库变成 **LLM/Agent 可持续接手的源码工程控制面**。

目标不是替代现有源码或测试，而是为模型和自动执行器补齐三类最小能力：

1. **任务卡**：把优化目标、作用范围、成功标准、必跑门禁做成机器可读卡片。
2. **统一门禁**：通过 `scripts/agent/run-gates.*` 把现有质量门禁、smoke、longrun 收束为统一入口。
3. **证据归档**：每次执行都把任务快照、门禁结果、日志、摘要归档到统一证据目录。

## 目录说明

- `mission.json`：长期目标与优先级。
- `constraints.json`：硬约束，模型和执行器不得突破。
- `modules.json`：源码模块边界、路径范围和不变量。
- `gates.json`：门禁目录与支持平台说明。
- `tasks/`：任务卡队列。建议通过 `backlog/active/done/blocked` 管理生命周期。
- `evidence/`：执行产物说明与历史归档占位。
- `baselines/`：性能、长稳、弱网等基线快照。
- `outputs/`：补丁、报告等产物占位。

## 推荐使用方式

### Linux

```bash
./scripts/agent/run-task.sh agent/tasks/active/AGENT-BOOTSTRAP-0001.json
```

### Windows PowerShell

```powershell
./scripts/agent/run-task.ps1 -TaskFile agent/tasks/active/AGENT-BOOTSTRAP-0001.json
```

## 执行结果

默认会写入：

- `artifacts/agent/<task-id>/<timestamp>/gate-results.json`
- `artifacts/agent/<task-id>/<timestamp>/gate-results.md`
- `artifacts/agent/<task-id>/<timestamp>/execution-summary.json`
- `artifacts/agent/<task-id>/<timestamp>/summary.md`
- `artifacts/agent/<task-id>/<timestamp>/logs/`
- `artifacts/agent/<task-id>/<timestamp>/config-snapshot/`

## 平台约定

- 统一任务卡格式：JSON
- 统一门禁结果格式：JSON + Markdown
- Linux 使用 shell 入口；Windows 使用 PowerShell 入口
- 能跨平台的门禁必须提供两侧脚本；仅 Linux 可用的门禁需要在 `agent/gates.json` 中明确标注
