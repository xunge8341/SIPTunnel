# P3 熔断恢复与观察

本阶段补齐三项：

1. 熔断状态显式区分 `open / half_open / closed`。
2. `/api/protection/state` 和 `/metrics` 暴露半开观察数量与熔断条目。
3. 支持手工恢复熔断对象：`POST /api/protection/circuit/recover`。

## 手工恢复

请求体可选：

```json
{ "target": "map-orders" }
```

- 不传 `target`：清理全部熔断条目。
- 传 `target`：按 key / 最近失败原因模糊匹配并清理。

## UI

“告警与保护”页提供“熔断恢复”按钮，操作后会回读当前保护状态。

## 下一批补充

- 支持熔断恢复目标过滤（target 可选，空值恢复全部）。
- `/api/protection/state` 已显式暴露 open / half_open 观察态与条目列表。
- 管理台“告警与保护”页可直接执行恢复动作，并在恢复后刷新运行态。
