# SIPTunnel

跨安全边界业务交换网关（SIP 控制面 + RTP 文件面 + A 网 HTTP 落地执行）。

## 架构要点

- SIP 控制面：使用 JSON Body 承载完整业务字段，Header 仅镜像 `trace_id/request_id/api_code` 索引。
- RTP 文件面：`二进制定长主头(32B) + 可选 TLV 扩展段`，支持大载荷补片重组。
- A 网执行：仅允许 `api_code -> HTTP 路由模板` 的受控映射，不支持任意透传。
- 安全签名：当前实现 `HMAC-SHA256`，接口预留 `SM3-HMAC` 算法位。
- 可靠性：提供幂等、补片重组、重试退避、限流基础模块。
- 可观测性：日志/审计接口与运维后台前端骨架。

## 目录

- `cmd/gateway`：网关启动入口
- `internal/control`：SIP 控制面协议封装
- `internal/rtp`：RTP 二进制主头/TLV 编解码
- `internal/security`：签名抽象与 HMAC 实现
- `internal/router`：受控 HTTP 模板路由
- `internal/service`：幂等、补片、重试、限流等服务
- `internal/observability`：审计日志抽象
- `web/`：Vue3 + TS + Vite + Ant Design Vue 运维后台骨架
- `docs/`：协议与运行文档

## 快速运行

```bash
go test ./...
go run ./cmd/gateway
```

## 前端开发

```bash
cd web
npm install
npm run dev
```

## 当前已实现能力（首版）

1. SIP Envelope 编解码。
2. RTP 主头/TLV 编解码。
3. HMAC 签名与验签。
4. API Code 路由模板控制。
5. 幂等判重、补片重组、指数退避、令牌桶限流。
6. 运维后台骨架（状态概览、告警占位、链路追踪占位）。

详细见 `docs/design.md`。
