# 设计说明

## 1. 控制面（SIP）

`gateway-server/internal/control/message.go`

- `SIPBusinessMessage` 通过 JSON Body 承载完整业务参数（payload/meta/audit）。
- Header 只镜像索引字段，避免双写一致性问题扩散。

## 2. 文件面（RTP）

`gateway-server/internal/rtp/header.go`

- 固定 32 字节主头：Magic、版本、分片序号、总分片数、payload 长度、TLV 长度。
- 扩展字段通过 TLV 表达，便于未来增加签名、压缩、业务标签。
- `Reassembler` 按 `message_id` 聚合分片，支持重复片去重（补片/重传）。

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
