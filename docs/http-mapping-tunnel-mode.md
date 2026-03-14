# HTTP 映射隧道模式说明（主线）

本文档用于统一主线认知：SIPTunnel 当前不是“api_code 模板调用系统”，而是**受全局网络模式约束的 HTTP 映射隧道系统**。


## 0. 主链路网络模型（接收端/发送端）

HTTP 映射隧道模式采用明确的双节点协作模型：

- **接收端（SIP 下级域）**：监听本端入口端口；收到 HTTP 请求后向发送端发起 Invite。
- **发送端（SIP 上级域）**：接收 Invite 后访问对端目标 `IP:Port`；再通过 SIP/RTP 把响应返回接收端。
- **映射方向**：支持单向映射与双向映射。
- **能力来源**：链路基础能力来自 GB/T 28181 视频点流承载模型。

## 1. 两种产品模式的边界

### 1.1 命令网关模式

- 主要职责：SIP 命令下发、状态回传、任务审计。
- 数据重心：命令任务与控制流。
- 典型场景：设备控制、状态同步、编排触发。

### 1.2 HTTP 映射隧道模式（当前主线）

- 主要职责：把本端入口请求按映射规则转发到对端目标。
- 数据重心：本端节点、对端节点、网络模式、能力矩阵、隧道映射。
- 典型场景：跨安全边界 HTTP 访问、受控映射转发、能力受限场景下的稳定运行。

> 兼容说明：历史 `route/api_code/template` 仍可在迁移链路与兼容 API 中出现，但仅作为历史模型中的兼容术语（deprecated 术语）。

## 2. 网络模式 -> 能力矩阵 -> 隧道映射

主线采用三层约束：

1. **网络模式（全局）**：定义链路方向与承载边界。
2. **能力矩阵（全局推导）**：由后端根据网络模式导出支持/不支持能力。
3. **隧道映射（逐条配置）**：仅配置业务映射关系（本端入口 -> 对端目标），不能突破全局能力边界。

### 2.1 不同网络模式可用的 HTTP 能力

| 网络模式 | 小请求体 | 大请求体 | 大响应体 | 流式响应 | 双向隧道请求（CONNECT/TRACE） |
| --- | --- | --- | --- | --- | --- |
| `A_TO_B_SIP__B_TO_A_RTP` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `A_B_BIDIR_SIP__BIDIR_RTP` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `A_B_BIDIR_SIP__B_TO_A_RTP` | ✅ | ❌ | ✅ | ✅ | ❌ |
| `RESERVED_*` | ❌ | ❌ | ❌ | ❌ | ❌ |

## 3. 为什么 transport 策略由全局决定

`TunnelTransportPlan` 由后端按 `NetworkMode + Capability` 统一推导，属于**系统级策略**，不是映射级参数。原因如下：

- 避免同一节点内出现相互冲突的 transport 决策。
- 避免 UI 与后端各自硬编码导致策略漂移。
- 便于审计、诊断与自检统一解释。
- 保障安全默认：受限网络模式下自动收敛到保守策略。

## 4. 为什么映射页不逐条配置 transport

映射页负责“业务路由语义”，而非“底层承载策略”——即只配置：

- 本端入口（IP/Port/BasePath）
- 对端目标（IP/Port/BasePath）
- 方法白名单与超时/体积上限

不提供逐条 transport 配置的原因：

- 逐条 transport 会绕开全局能力矩阵，破坏一致性。
- 运维误配置概率显著上升，增加排障复杂度。
- 与“网络模式统一治理”目标冲突。

## 5. 旧术语映射（兼容视图）

- `route`（旧）-> 隧道映射（主线）
- `api_code`（旧）-> 兼容索引字段（控制面保留）
- `template`（旧）-> 历史配置结构（迁移期可读）

建议：新增页面、文档、注释统一使用“隧道映射/本端入口/对端目标/网络模式/能力矩阵”术语。


## 6. 映射状态与诊断术语（前后端统一）

### 6.1 映射链路状态

| 字段 | 中文展示 | 说明 |
| --- | --- | --- |
| `disabled` | 未启用 | 规则未启用，不参与链路监听。 |
| `listening` | 监听中 | 本端入口已监听，等待业务连接。 |
| `connected` | 已连接 | 映射链路可用，可稳定收发。 |
| `interrupted` / `abnormal` | 异常 | 运行中中断或健康探测失败。 |
| `start_failed` | 启动失败 | 启动监听失败（端口冲突/权限/配置错误）。 |

### 6.2 规则测试结果

- 信令请求：`成功 / 失败`
- 响应通道：`正常 / 异常`
- 注册状态：`正常 / 未注册`

规则测试失败时必须返回并展示：

- `failure_reason`（异常原因）
- `suggested_action`（建议动作）

要求：后端直接返回中文诊断语义，前端仅做展示，不再额外翻译成另一套术语。

## 7. 运行时转发策略分层（当前 direct + 未来 SIP/RTP）

为避免把当前 HTTP direct 转发写成“不可演进”的路径，映射运行时采用监听层与转发策略解耦：

- `mapping runtime listener`：仅负责本端 `IP:Port` 监听、连接接入、状态机维护与审计。
- `forward strategy`：负责请求准备与执行；接口拆分为 `PrepareForward` 与 `ExecuteForward`。
- `direct HTTP forwarder`（当前落地）：
  - `PrepareForward` 完成方法校验、请求体限制、路径映射与转发头构建。
  - `ExecuteForward` 执行 HTTP 直连请求并保留超时/响应体限制控制。
- `future SIP/RTP tunnel forwarder`（预留）：
  - 在同一策略接口下扩展 SIP 元信息、RTP 大载荷、流式响应回传。

该分层确保：当前版本可直接运行，同时不破坏后续 Invite + SIP/RTP 承载主链路的接入方式。

## 8. 新增：映射 ID 自增与配置持久化

- 映射创建时 `mapping_id` 支持由后端自动分配（数值自增），前端创建表单不再要求手填。
- 自增游标与映射列表一起持久化在 `data/final/tunnel_mappings.json` 的 `cursor` 字段。
- 启动时自动加载映射与游标；若文件损坏，服务会在初始化时输出 `decode mapping store` 错误日志，便于快速恢复。
- 运行时瞬态状态（如 active forwarding 计数）不会落盘。

## 9. 新增：心跳与会话保活

- 心跳调度由 `tunnel_session_manager` 统一执行，支持：
  - `heartbeat_interval_sec`
  - `register_retry_count`
  - `register_retry_interval_sec`
- 连续心跳失败会累计 `consecutive_heartbeat_timeout`，并驱动重注册。
- UI 在“隧道配置”页展示：
  - 心跳状态
  - 最近心跳时间
  - 连续超时计数
  - 手动“发送一次心跳”

## 10. 新增：SIP/RTP / 映射运行日志要求

- 会话日志：
  - `REGISTER`（注册成功/失败/401）
  - `MESSAGE`（心跳成功/超时）
  - 包含 `session_id`、状态码/结果、耗时。
- 映射运行日志：
  - 每次转发生成 `request_id`
  - 日志包含 `mapping_id`、upstream/downstream 方向、错误原因、耗时。
- 映射状态机统一收敛为：`listening / forwarding / degraded / interrupted`（含 disabled、start_failed）。

## 11. 映射“有监听无响应”修复说明

当前 runtime 已补齐转发闭环：

`HTTP request -> mapping runtime -> forward target -> return response`

并在失败分支给出可观测错误（prepare / upstream forward / downstream writeback）。
