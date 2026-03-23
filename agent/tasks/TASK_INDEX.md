# 任务卡索引

以下任务卡按建议执行顺序整理，默认位于 `agent/tasks/backlog/`。

## P0（先做）

1. `SEC-SSRF-0001`：Target Sandbox + Request Sanitizer
2. `NET-MTU-0002`：控制面 MTU 高危退化
3. `GOV-HYST-0004`：迟滞/冷却状态机
4. `REL-WIN-0012`：Windows + Go1.25 原生门禁闭环

## P1（主链增强）

5. `ARC-LARGE-0005`：大响应 orchestrator / executor / observer 拆分
6. `OBS-GEN-0006`：Generic Download 事务摘要 + breaker 口径拆分
7. `OBS-RTP-0007`：`rtp_ps_summary` + device/call summary
8. `OBS-STARTUP-0008`：活跃策略快照结构化导出
9. `OPS-DIAG-0009`：诊断导出补链路摘要页
10. `STD-ERROR-0010`：failure reason 字典集中化
11. `NET-META-0003`：元数据旁带机制骨架

## P2（工程边界与长期优化）

12. `OPS-HANDLER-0011`：管理面 handler 继续按职责拆目录

## 依赖提示

- `NET-META-0003` 依赖 `NET-MTU-0002`
- `OBS-GEN-0006` 依赖 `ARC-LARGE-0005`
- `OBS-STARTUP-0008` 建议在 `GOV-HYST-0004` 之后推进
- `OPS-DIAG-0009` 建议在 `OBS-STARTUP-0008`、`OBS-GEN-0006`、`OBS-RTP-0007` 后推进

## 建议执行方式

- 单张任务卡执行：
  - Linux：`./scripts/agent/run-task.sh agent/tasks/backlog/<TASK>.json`
  - Windows：`./scripts/agent/run-task.ps1 -TaskFile agent/tasks/backlog/<TASK>.json`
- 任务推进时，建议将单张卡移动到 `agent/tasks/active/`，完成后移动到 `done/`，失败阻塞移到 `blocked/`。
