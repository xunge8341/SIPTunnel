# 设计说明

## 1. 控制面（SIP）

`gateway-server/internal/control/message.go`

- `SIPBusinessMessage` 通过 JSON Body 承载完整业务参数（payload/meta/audit）。
- Header 只镜像索引字段，避免双写一致性问题扩散。

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

## 4. A 网 HTTP 执行策略

`gateway-server/internal/router/template_router.go`

- 仅允许预定义 `api_code` 映射模板。
- 未知 `api_code` 直接拒绝，避免任意 HTTP 透传风险。

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
- 返回体统一为 `{code,message,data}`，任务与审计列表统一携带分页结构。
- 运维写操作（重试、取消、更新限流、更新路由）会写入审计日志，支持 `request_id/trace_id` 检索。
- OpenAPI 描述见 `gateway-server/docs/openapi-ops.yaml`。

## 8. gateway-ui 页面与数据适配

`gateway-ui/src/views` + `gateway-ui/src/api`

- Dashboard 增加成功率、失败率、并发、RTP 丢片率、限流命中次数与最近任务趋势图。
- 命令任务/文件任务页面提供筛选区、`request_id/trace_id` 查询、状态标签、详情跳转。
- 任务详情页整合基础信息、状态流转时间线、SIP 事件、RTP 分片统计、HTTP 执行结果、审计记录片段。
- API 采用双模式适配：`VITE_API_MODE=real` 走真实接口，默认走 mock 数据，便于联调与独立开发。
- 类型集中在 `src/types/gateway.ts`，避免页面层与接口字段耦合。
