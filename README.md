# SIPTunnel Monorepo

SIPTunnel 是跨安全边界业务交换网关，当前仓库为 monorepo 结构：

- `gateway-server/`：Go 网关服务（SIP/RTP/路由/签名等核心能力）
- `gateway-ui/`：Vue3 运维前端
- `deploy/`：部署相关脚本与清单（预留）
- `scripts/`：仓库级开发脚本（启动/测试/格式化/lint）

## 目录结构

```text
.
├── gateway-server
│   ├── cmd/gateway
│   ├── internal
│   ├── configs
│   ├── scripts
│   ├── Makefile
│   └── Dockerfile
├── gateway-ui
├── scripts
└── deploy
```

## 如何启动

### 一键本地启动（推荐）

```bash
./scripts/dev.sh
```

该命令会先启动后端 `gateway-server`，再启动前端 `gateway-ui` 开发服务器。

### 分别启动

后端：

```bash
cd gateway-server
go run ./cmd/gateway
```

前端：

```bash
cd gateway-ui
npm install
npm run dev
```

默认地址：

- 后端健康检查：`http://127.0.0.1:18080/healthz`
- 前端 Dashboard：`http://127.0.0.1:5173/dashboard`

## 如何测试

### 一键测试

```bash
./scripts/test.sh
```

### 分模块测试

```bash
cd gateway-server && go test ./...
cd gateway-ui && npm run test
```

当前测试覆盖包含：

- 后端：协议编解码、签名、RTP 分片重组（含缺片/乱序）、HTTP 映射、限流
- 前端：关键页面渲染、状态管理、关键组件交互

## 如何查看 Dashboard

1. 启动服务（`./scripts/dev.sh` 或分别启动）。
2. 浏览器访问：`http://127.0.0.1:5173/dashboard`。
3. 页面展示关键指标（成功率、失败率、并发、RTP 丢片率、限流命中）与近期任务趋势图。

## 如何配置路由和限流

### 路由模板（api_code -> HTTP）

- 示例配置：`gateway-server/configs/httpinvoke_routes.example.yaml`
- 设计约束：必须通过 `api_code` 命中白名单路由，禁止任意目标透传。

建议流程：

1. 复制示例文件并按业务新增 `api_code`。
2. 配置目标服务地址、方法、路径、超时、重试。
3. 配置 `header_mapping/body_mapping` 完成字段映射。
4. 启动后通过接口请求验证路由是否按模板执行。

### 限流策略

- 后端限流器：`gateway-server/internal/service/rate_limiter.go`
- 典型参数：`rps`（速率）+ `burst`（桶容量）

可在接入层按 `api_code`、来源系统或节点维度装配不同实例，以实现细粒度限流策略。

## 开发辅助脚本

```bash
./scripts/dev.sh      # 本地启动
./scripts/test.sh     # 本地测试
./scripts/format.sh   # 格式化（Go + 前端）
./scripts/lint.sh     # lint（gofmt 检查 + eslint）
```

## CI

新增 GitHub Actions：`.github/workflows/ci.yml`，默认执行：

- 后端：`go test ./...` + `gofmt` 格式检查
- 前端：`npm run lint` + `npm run build`
