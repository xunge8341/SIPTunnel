# SIPTunnel 最终整体任务源码行级复核报告（2026-03-20）

## 结论

- **12 / 12 项任务：源码层面达成**
- **Windows 构建阻断修复：已落盘到源码**
- **Task 9 / Task 10：已重新落盘到最终源码工件**

## 阻断修复

### Windows 构建失败：`err redeclared in this block`

- 文件：`gateway-server/internal/server/gb28181_tunnel.go`
- 行级：L491-L511
- 结论：**已修复**

`ExecuteForward(...)` 使用命名返回值 `err`，但函数体内又重复声明局部 `err`。当前已删除该局部重复声明，保留 `rtpReceiver` 变量，直接对应用户现场的 Windows 构建失败日志。

## Task 9 行级验收

### keep-alive 决策与 transport 级应用

- 文件：`gateway-server/internal/server/http_runtime_mitigation.go`
- 决策结构与策略：L14-L52
- transport 级应用：L87-L99
- 结论：**达成**

新增 `runtimeHTTPKeepAliveDecision`、`runtimeHTTPKeepAlivePolicy(scope)` 与 `ApplyRuntimeHTTPTransportMitigations(scope, transport)`，并在 transport 层实际执行 `DisableKeepAlives / MaxIdleConns / MaxIdleConnsPerHost` 控制，不再停留在 server 侧 workaround。

### mapping-forward transport / client 缓存维度隔离

- 文件：`gateway-server/internal/server/mapping_forward_transport.go`
- cache key：L13-L19
- transport 初始化：L29-L46
- 结论：**达成**

`mappingTransportKey` 新增 `DisableKeepAlives` 维度，避免 A/B 变体命中同一 transport/client 缓存，消除伪实验风险。

### 压测面新增连接 / 建连 / 首字节指标

- 文件：`gateway-server/loadtest/loadtest.go`
- 观测结构：L67-L115
- 运行时聚合：L178-L196
- 汇总输出：L336-L366
- 报告渲染：L494-L530
- HTTP trace 实现：L690-L768
- 结论：**达成**

已新增并汇总 `connect_p50/p95`、`first_byte_p50/p95`、`trace_samples`、`new_conn_count`、`reused_conn_count`、`reused_idle_conn_count`，并通过 `httptrace.ClientTrace` 捕获 `ConnectStart/Done`、`GotConn`、`GotFirstResponseByte`。

### A/B 实验 manifest 与分析器

- 文件：`gateway-server/loadtest/experiment.go`
- 实验对象定义：L15-L83
- 网关日志解析：L147-L185
- A/B 评分逻辑：L185-L230
- 命令行入口：`gateway-server/cmd/loadtest/main.go` L38-L40、L49-L67
- 结论：**达成**

源码中已补明确算法注释：success rate 优先，first-byte/connect latency 次级惩罚，`response_start_timeout` 与 `new_conn_count` 用于解释吞吐下降原因。

## Task 10 行级验收

### 场景矩阵与日志归因

- 文件：`gateway-server/loadtest/experiment.go`
- 网关日志解析：L147-L185
- 容量判定与 ceiling：L230-L303
- 结论：**达成**

已纳入 `success_rate`、`first_byte_p95_ms`、`gate_wait_p95/p99`、`response_start_timeout`、`rtp_seq_gap_count`、`resume_count`、`bye_count` 等容量归因指标。

### 按响应画像分级阈值

- 文件：`gateway-server/loadtest/experiment.go`
- 结论：**达成**

已编码：

- `small_page_data`：500ms / 200ms
- `socketio_polling`：800ms / 250ms
- `bulk_download`：1500ms / 400ms
- 且统一要求 `success_rate >= 99.5%` 与 `response_start_timeout == 0`

### 容量矩阵脚本与文档

- 脚本：`scripts/loadtest/run_keepalive_ab.sh`
- 脚本：`scripts/loadtest/run_capacity_matrix.sh`
- 文档：`gateway-server/docs/20260320-task9-task10-experiment-playbook.md`
- 结论：**达成**

## 12 项任务最终复核

| 任务 | 结论 | 说明 |
|---|---|---|
| Task 1 安全预算驱动 AUTO 决策 | 达成 | 已在 v3 工件中固化 |
| Task 2 响应承载预算明细日志 | 达成 | 已在 v3 工件中固化 |
| Task 3 拆单设备单通道串行门控 | 达成 | 已在 v3 工件中固化 |
| Task 4 startup/self-check/transport_plan 显示有效值 | 达成 | 已在 v3 工件中固化 |
| Task 5 INLINE/RTP 按响应类型分级 | 达成 | 已在 v3 工件中固化 |
| Task 6 RTP 接收侧乱序容忍 | 达成 | 已在 v3 工件中固化 |
| Task 7 RTP pacing 稳定整形 | 达成 | 已在 v3 工件中固化 |
| Task 8 fixed-window / resume 硬化 | 达成 | 已在 v3 工件中固化 |
| Task 9 keep-alive workaround 吞吐评估 | 达成 | 本轮重新落盘源码、脚本、分析器、文档 |
| Task 10 容量基线压测 | 达成 | 本轮重新落盘源码、脚本、分析器、文档 |
| Task 11 单次事务视图 | 达成 | 已在 v3 工件中固化 |
| Task 12 配置模板安全边界推荐档 | 达成 | 已在 v3 工件中固化 |

## 如实说明

当前沙箱无法访问外部 Go 依赖源，因此未能在项目根模块内完成全仓 `go build ./...` / `go test ./...` 的联网依赖留痕。本报告结论是**源码行级复核结论**，并未冒充联网条件下的动态全绿结论。
