# 设计说明

## 1. 控制面（SIP）

`gateway-server/internal/control/message.go`

- `SIPBusinessMessage` 通过 JSON Body 承载完整业务参数（payload/meta/audit）。
- Header 只镜像索引字段，避免双写一致性问题扩散。
- `task.status` 在异常态（`failed/cancelled/dead_lettered/retry_wait`）必须包含 `status_reason`，用于一线诊断与回溯。

## 2. 文件面（RTP）

`gateway-server/internal/protocol/rtpfile`

- 固定主头 + TLV 扩展段，主头包含 `magic/protocol_version/header_length/flags/transfer_id/request_id/trace_id/chunk_no/chunk_total/chunk_offset/chunk_length/file_size/chunk_digest/file_digest/send_timestamp`。
- TLV 使用 `type/length/value`，解码时跳过未知 type，保证向前兼容。
- 提供 `MarshalBinary()` 与 `UnmarshalBinary()`，并执行 `header_length` 校验。
- `SplitFileToChunks` 与 `Reassembler` 提供分片、摘要、乱序重组与重复片处理能力。

## 3. 安全与算法升级

`gateway-server/internal/security/signer.go`

- `Signer` 接口统一签名能力。
- 首版实现 `HMAC_SHA256`。
- 预留 `SM3_HMAC` 常量作为后续国密升级占位，不破坏调用方接口。

## 4. A 网 HTTP 执行策略（HTTP 映射隧道模式主线）

`gateway-server/internal/router/template_router.go`

- 主线模型为“隧道映射（本端入口 -> 对端目标）”。
- `api_code`/`route template` 为历史兼容术语，仍可作为控制面索引或迁移输入。
- 未知 `api_code` 在兼容路径下直接拒绝，避免任意 HTTP 透传风险。
- transport 决策由全局 `NetworkMode -> Capability -> TunnelTransportPlan` 推导，不在单条映射配置。

## 5. 可靠性基础模块

`gateway-server/internal/service`

- `IdempotencyStore`：请求号去重。
- `RetryPolicy`：指数退避时延。
- `RateLimiter`：令牌桶限流。

## 6. 可观测性与运维

- `gateway-server/internal/observability/telemetry.go`：审计日志入口。
- `gateway-ui/`：运维后台前端骨架（首页展示与状态占位）。

## 7. 运维管理 API

`gateway-server/internal/server/http.go`

- 新增 `/api/tasks`、`/api/limits`、`/api/routes`、`/api/nodes`、`/api/audits` 等运维接口。
- `TunnelMapping` 现为主业务模型，对应 `/api/mappings` CRUD，用于表达“本端入口(local_bind/local_base_path) ↔ 对端目标(remote_target)”语义。
- `OpsRoute/RouteConfig` 降级为兼容层，仅保留 `/api/routes`（deprecated）用于历史前端/脚本平滑迁移。
- 持久化层在启动时支持旧 `OpsRoute/RouteConfig` 数据自动迁移到 `TunnelMapping`，并提供 `cmd/mapping-migrate` 离线转换工具，避免一次性破坏升级。
- 返回体统一为 `{code,message,data}`，任务与审计列表统一携带分页结构。
- 运维写操作（重试、取消、更新限流、更新路由）会写入审计日志，支持 `request_id/trace_id` 检索。
- OpenAPI 描述见 `gateway-server/docs/openapi-ops.yaml`。

## 8. gateway-ui 页面与数据适配

`gateway-ui/src/views` + `gateway-ui/src/api`

- Dashboard 增加成功率、失败率、并发、RTP 丢片率、限流命中次数与最近任务趋势图。
- 命令任务/文件任务页面提供筛选区、`request_id/trace_id` 查询、状态标签、详情跳转。
- 任务详情页整合基础信息、状态流转时间线、SIP 事件、RTP 分片统计、HTTP 执行结果、审计记录片段。
- API 采用双模式适配：默认 `VITE_API_MODE=real` 走真实接口；仅当显式设置 `VITE_API_MODE=mock` 时走 mock 数据，避免主路径误用伪数据。
- 类型集中在 `src/types/gateway.ts`，避免页面层与接口字段耦合。


## 9. 本端节点 / 对端节点建模与持久化

`gateway-server/internal/nodeconfig` + `gateway-server/internal/repository/file/node_config_store.go`

- `LocalNodeConfig`：描述本端节点（node_id/node_name/node_role）与本端网络绑定（network_mode/sip_listen/rtp_listen）。
- `PeerNodeConfig`：描述对端节点（peer_node_id/peer_name）与对端信令/媒体地址段，并通过 `supported_network_mode` 声明兼容网络模式。
- `network_mode` 挂在 `LocalNodeConfig` 上，作为“本端当前生效网络能力边界”；Peer 记录其可兼容模式用于后续匹配。
- 提供真实文件持久化（`data/final/node_config.json`），重启后配置可恢复。
- API：
  - `GET/PUT /api/node`
  - `GET/POST /api/peers`
  - `PUT/DELETE /api/peers/{peer_node_id}`
- `TunnelMapping` 在当前主流程按单对单模型运行：映射编辑不暴露 `peer_node_id`，运行时由后端绑定唯一启用 peer（内部仍保留 `peer_node_id` 字段以兼容未来多对端扩展）。


## 术语纠偏（主线）

- 架构文档默认采用：本端节点、对端节点、网络模式、能力矩阵、隧道映射、本端入口、对端目标。
- `route/api_code/template` 仅在兼容 API、历史配置和迁移工具中出现，统一视为历史模型（兼容术语 / deprecated）。
- 运行链路应理解为“接收端（SIP 下级域）监听本端入口并发起 Invite，发送端（SIP 上级域）访问对端目标并回传”。

## 10. 大响应策略收口（当前有效）

`gateway-server/internal/server`

- 大响应交付策略只保留四个运行态决策：`stream_primary`、`range_primary`、`adaptive_segmented_primary`、`fallback_segmented`。
- 对非音视频大文件的开区间 Range（如 `bytes=0-`）不再沿用播放式 `stream_primary`；运行时会优先切到 `adaptive_segmented_primary` 的保守下载路径。
- 分段 profile 只保留一个统一选择入口：显式 child-profile > `generic-rtp` > `boundary-rtp` > `boundary-http` > `standard-http`（兼容 fallback）。
- `standard-http` 仅作为非 secure-boundary 的兼容兜底，不得在控制台、文档或日志中冒充当前主线能力。
- 目录资源只表达 `MANUAL / UNEXPOSED` 事实；历史自动暴露/自动映射不再作为当前运行态事实来源。

## 11. 后台链路时序图（当前有效）

- 后台真实执行链路、时序图与优化建议统一见：`docs/BACKEND_CHAIN_SEQUENCE_AND_OPTIMIZATION_20260321.md`
- 该文档只描述**当前源码真实生效路径**，用于排查“设计策略 vs 代码实现”是否偏离。
- 新的链路调整若影响启动、映射、大响应、RTP、GB28181、观测中的任一主链路，必须同步更新该文档。

## 12. 启动摘要与失败原因字典（当前有效）

- 启动日志除了 transport tuning，还必须输出一条 `active_strategy_snapshot`，把当前真实生效的：
  - `response_mode_policy`
  - `large_response_delivery_family`
  - `segmented_profile_selector`
  - `boundary / playback / generic_download` 的 RTP 发送与容忍画像
  - `generic_download_circuit_policy`
  固化进启动摘要，避免现场靠读多处代码猜“现在到底跑的是哪套策略”。
- `rtp_sequence_gap`、`rtp_gap_timeout`、`timeout`、`connection_reset`、`broken_pipe`、`unexpected_eof` 这些跨链路失败原因，必须收口到共享字典；
  不允许 generic download、window recovery、device penalty、日志总结各自维护一套近义词。
- 像“端口占用判断”这类跨入口基础规则，必须走共享工具，不允许 `cmd/` 与 `internal/server/` 各写一份字符串匹配。
