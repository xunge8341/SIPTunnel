# SIPTunnel Monorepo

SIPTunnel 是跨安全边界业务交换网关，当前仓库已初始化为 monorepo 结构：

- `gateway-server/`：Go 网关服务（SIP/RTP/路由/签名等核心能力）
- `gateway-ui/`：Vue3 运维前端
- `docs/`：设计与使用文档
- `deploy/`：部署相关脚本与清单（预留）

## 目录结构

```text
.
├── gateway-server
│   ├── cmd/gateway
│   ├── internal
│   ├── pkg
│   ├── configs
│   ├── scripts
│   ├── Makefile
│   └── Dockerfile
├── gateway-ui
├── docs
└── deploy
```

## 启动方式

### 后端（gateway-server）

```bash
cd gateway-server
go test ./...
go run ./cmd/gateway
```

默认监听 `:18080`，健康检查：

```bash
curl http://127.0.0.1:18080/healthz
```

### 前端（gateway-ui）

```bash
cd gateway-ui
npm install
npm run dev
```

默认开发地址：`http://127.0.0.1:5173`。

## SIP 控制面协议模型

`gateway-server/internal/protocol/sip` 提供控制面协议模型与 JSON 编解码层，当前实现包含：

- 统一公共头字段校验（必填、协议版本、时间窗）
- 八类控制消息（`command.create`、`command.accepted`、`file.create`、`file.accepted`、`task.status`、`file.retransmit.request`、`task.result`、`task.cancel`）
- SIP Header 镜像字段生成（`X-Request-ID`、`X-Trace-ID`、`X-Session-ID`、`X-Api-Code`、`X-Message-Type`、`X-Source-System`）

该包仅负责协议模型、JSON 编解码和校验，不包含 SIP 网络收发逻辑。

## RTP 文件面应用层协议

`gateway-server/internal/protocol/rtpfile` 提供 RTP 文件传输应用层协议库：

- 二进制定长主头（magic/version/header_length/flags/transfer_id/request_id/trace_id/chunk 元数据/摘要/时间戳）
- 可选 TLV 扩展段（type/length/value），解码时可跳过未知 type
- `MarshalBinary()` / `UnmarshalBinary()` 编解码及头长校验
- 分片与重组工具（按 chunk size 切片、生成每片头、计算 chunk/file digest、支持乱序与重复片）
