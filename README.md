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
