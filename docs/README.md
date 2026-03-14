# SIPTunnel Docs 索引

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

### 对外暴露位置

- 系统信息 API：`GET /api/node/network-status`
  - 返回 `network_mode`、`capability`、`capability_summary`、`transport_plan`。
- 启动摘要 API：`GET /api/startup-summary`
  - 返回 `network_mode`、`capability`、`capability_summary`、`transport_plan`。
- 诊断导出：`GET /api/diagnostics/export`
  - 诊断文件 `01_transport_config.json` 包含 `network_mode`、`capability`、`capability_summary`、`transport_plan`。
