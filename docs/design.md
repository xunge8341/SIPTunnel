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

