# SIPTunnel Docs 索引

## 产品模式边界（命令网关模式 vs HTTP 映射隧道模式）

### 命令网关模式

- 核心目标：在控制面完成命令下发、状态回传与审计闭环。
- 关键对象：命令任务、状态事件、签名验签、防重放。
- HTTP 角色：可选执行面能力，不是唯一主轴。

### HTTP 映射隧道模式（当前主线）

- 核心目标：在全局**网络模式**约束下，为业务提供稳定可治理的 HTTP 映射通道。
- 关键对象：本端节点、对端节点、能力矩阵、隧道映射（本端入口 -> 对端目标）。
- HTTP 角色：主线能力；transport 策略由后端按网络模式统一推导。

> 兼容术语说明：`route` / `api_code` / `template` 仅作为历史模型兼容词保留，不再用于主线叙述。

## 网络模式与能力矩阵

SIPTunnel 将“链路方向性/承载能力”统一建模为 `NetworkMode`，并通过后端统一函数 `DeriveCapability(mode)` 推导 HTTP 能力边界，避免 UI、handler、transport 分散硬编码。

### NetworkMode 定义

- `A_TO_B_SIP__B_TO_A_RTP`
  - 含义：A->B 仅 SIP 小报文，B->A 通过 RTP 回传大载荷。
  - 典型场景：小请求 + 大响应。
- `A_B_BIDIR_SIP__BIDIR_RTP`
  - 含义：A/B 双向 SIP + 双向 RTP。
  - 典型场景：双向大载荷、透明代理/隧道。
- `A_B_BIDIR_SIP__B_TO_A_RTP`
  - 含义：A/B 双向 SIP，RTP 仅 B->A。
  - 典型场景：双向控制 + 下行大响应，上行大上传受限。
- `RESERVED_*`
  - 预留模式，能力默认全部降级为不支持（安全默认）。

### Capability 字段

- `supports_small_request_body`
- `supports_large_request_body`
- `supports_large_response_body`
- `supports_streaming_response`
- `supports_bidirectional_http_tunnel`
- `supports_transparent_http_proxy`

### 模式与能力边界

| NetworkMode | small request | large request | large response | streaming response | bidir tunnel | transparent proxy |
| --- | --- | --- | --- | --- | --- | --- |
| `A_TO_B_SIP__B_TO_A_RTP` | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ |
| `A_B_BIDIR_SIP__BIDIR_RTP` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `A_B_BIDIR_SIP__B_TO_A_RTP` | ✅ | ❌ | ✅ | ✅ | ❌ | ❌ |
| `RESERVED_*` | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |


### TunnelTransportPlan（全局承载策略）

`TunnelTransportPlan` 由后端统一函数 `ResolveTransportPlan(mode, capability)` 根据 `NetworkMode` 自动推导，属于**全局能力**，不是逐条映射配置项。

字段：

- `request_meta_transport`
- `request_body_transport`
- `response_meta_transport`
- `response_body_transport`
- `request_body_size_limit`
- `response_body_size_limit`
- `notes`
- `warnings`

设计原则：

- 运维无需在每条映射配置 transport 字段，避免配置漂移与策略冲突。
- transport 决策只在后端集中推导，不在 UI/handler 中散落硬编码。
- UI 应将该计划用于只读展示和诊断解释，不提供逐条编辑入口。


### TunnelMapping 与 Capability 约束

`TunnelMapping` 在保存/更新时会执行统一能力校验（后端集中校验，不依赖 UI 提示）：

- `max_request_body_bytes`：当 `supports_large_request_body=false` 时，不可超过 `1 MiB`（hard error）。
- `max_response_body_bytes`：当 `supports_large_response_body=false` 时，不可超过 `1 MiB`（hard error）。
- `allowed_methods`：默认由系统写入 `[*]`（全部允许）；在受限模式（`supports_bidirectional_http_tunnel=false`）下，若显式传入 `CONNECT/TRACE` 仍会触发 hard error，`PUT/PATCH/DELETE` 给出 warning。
- `require_streaming_response`：当映射要求流式响应且 `supports_streaming_response=false` 时，禁止保存（hard error）。

其中 hard error 会阻止保存；warning 允许保存并通过 API 返回。

> 当前产品主流程为“本端与对端一对一映射（single-binding）”：映射规则仅配置本端入口与对端目标，`peer_node_id` 不在 UI 表单展示，由后端自动绑定唯一启用对端；若无对端或存在多对端冲突，API 会返回 `PEER_BINDING_INVALID`。

> 前端映射编辑抽屉采用单列纵向布局，字段顺序固定为：本端入口 IP、本端入口端口、对端目标 IP、对端目标端口、请求超时、响应超时、请求体大小上限、响应体大小上限、启用状态、备注（其余兼容字段按需补充）。
> 请求动作类型对应的命令/文件传输链路由系统自动决策，映射页不提供手工切换入口。

### 映射能力校验的暴露位置

- `POST /api/mappings`、`PUT /api/mappings/{id}`：成功响应可附带 `warnings`。
- `GET /api/mappings`：返回 `items` 同时附带 `bound_peer`（只读绑定对端）、可选 `binding_error` 与聚合 `warnings`。
- `GET /api/selfcheck`：新增 `mappings_capability_validation` 与 `mapping_peer_binding` 项，级别可能为 `info/warn/error`。
- `GET /api/diagnostics/export`：`01_transport_config.json` 增加 `mappings_capability_validation` 结果，用于离线排障。

### 对外暴露位置

- 系统信息 API：`GET /api/node/network-status`
  - 返回 `network_mode`、`capability`、`capability_summary`、`transport_plan`。
- 系统状态 API：`GET /api/system/status`
  - 返回 `tunnel_status`、`connection_reason`、`network_mode` 与首页能力矩阵字段（`supports_small_request_body`、`supports_large_response_body`、`supports_streaming_response`、`supports_large_file_upload`、`supports_bidirectional_http_tunnel`）。
- 隧道配置 API：`GET /api/tunnel/config`、`POST /api/tunnel/config`
  - 配置 `channel_protocol`、`request_channel`、`response_channel`、`network_mode`。
  - 后端根据 `network_mode` 自动刷新 `capability` 与 `capability_items` 能力矩阵。
  - UI 的 M32 隧道配置页在通道协议/请求通道/响应通道/网络模式任一字段变更时会即时重算并展示能力矩阵。
- 启动摘要 API：`GET /api/startup-summary`
  - 返回 `network_mode`、`capability`、`capability_summary`、`transport_plan`。
  - 返回 `data_sources`，明确 `node_config/peers/mappings/mode/capability` 的来源（文件路径或运行时推导）。
- 诊断导出：`GET /api/diagnostics/export`
  - 诊断文件 `01_transport_config.json` 包含 `network_mode`、`capability`、`capability_summary`、`transport_plan`。
  - `01_transport_config.json.data_sources` 显示 node/peer/mapping 与 mode/capability 的真实数据来源。


## node/peer 与 NetworkMode/Capability 联动校验

后端在统一校验器中对 `本端 node`、`对端 peer`、`current_network_mode`、`current_capability` 进行联动判断，并将结果接入：

- `GET /api/selfcheck`：新增
  - `local_node_config_valid`
  - `peer_node_config_valid`
  - `network_mode_compatibility`
- `GET /api/node`、`GET /api/node/network-status`、`GET /api/startup-summary`：新增
  - `current_network_mode`
  - `current_capability`
  - `compatibility_status`
- `GET /api/diagnostics/export` 的 `01_transport_config.json`：新增上述兼容性字段，便于离线诊断。

### 兼容规则

1. 本端兼容：`local_node.network_mode` 必须等于当前运行 `network_mode`。
2. 对端兼容：`peer.enabled=true` 的 peer，`supported_network_mode` 必须等于当前运行 `network_mode`。
3. 关键字段缺失（如 `peer_name`、`peer_signaling_ip`、`peer_media_ip`）会判定为不兼容并给出修复建议。

### 组合示例

- 兼容：
  - current=`A_TO_B_SIP__B_TO_A_RTP`
  - local.node.network_mode=`A_TO_B_SIP__B_TO_A_RTP`
  - peer.supported_network_mode=`A_TO_B_SIP__B_TO_A_RTP`
- 不兼容：
  - current=`A_TO_B_SIP__B_TO_A_RTP`
  - local.node.network_mode=`A_B_BIDIR_SIP__BIDIR_RTP`
- 不兼容：
  - current=`A_TO_B_SIP__B_TO_A_RTP`
  - peer.enabled=true 且 peer.supported_network_mode=`A_B_BIDIR_SIP__BIDIR_RTP`
- 不兼容（配置缺失）：
  - peer_name 为空，或 peer_signaling_ip/peer_media_ip 为空。

### 排查建议

- 先修复本端 `network_mode` 一致性，再修复 peer 的 `supported_network_mode` 与关键字段。
- 保存配置失败时，优先检查错误码：
  - `NETWORK_MODE_MISMATCH`
  - `PEER_NETWORK_MODE_INCOMPATIBLE`
  - `INVALID_ARGUMENT`
- 修复后立刻复核：`/api/selfcheck`、`/api/startup-summary`、`/api/node/network-status`。

## 推荐先读

- 产品模式与主线术语：`docs/http-mapping-tunnel-mode.md`
- 架构设计总览：`docs/design.md`
- 运维手册：`docs/operations.md`
