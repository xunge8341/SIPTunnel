# SIPTunnel Monorepo

SIPTunnel 是跨安全边界业务交换网关，当前仓库为 monorepo 结构：

- `gateway-server/`：Go 网关服务（SIP/RTP/签名验签/防重放/任务状态机/HTTP 映射/审计与可观测）
- `gateway-ui/`：Vue3 运维前端（Dashboard、任务、路由、限流、审计）
- `deploy/`：部署相关脚本与清单（预留）
- `scripts/`：仓库级开发脚本（启动/测试/格式化/lint）

## 关键能力与约束落实

- SIP 控制面：JSON Body 承载完整业务字段，Header 仅镜像索引字段（request/trace/session/api_code/message_type/source_system）。
- RTP 文件面：固定主头 + TLV 扩展协议结构在后端独立模块实现，业务代码不拼裸字节。
- 签名验签：通过 `Signer` 接口注入，当前 HMAC-SHA256，保留 `SM3_HMAC` 升级位。
- 防重放：基于 `request_id + nonce` 的接收防重放窗口。
- HTTP 执行：仅支持 `api_code -> route template` 受控映射，不支持任意透传。
- 生产基线：限流、审计日志、trace 字段透传和结构化日志。

## 如何启动

### 一键本地启动（推荐）

```bash
./scripts/dev.sh
```

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

## 前端联调模式

前端默认使用 mock 数据，联调时请切换 real 模式：

```bash
cd gateway-ui
VITE_API_MODE=real VITE_API_BASE_URL=http://127.0.0.1:18080/api npm run dev
```

页面将直接调用后端运维接口：

- `GET/PUT /api/limits`
- `GET/PUT /api/routes`
- `GET /api/tasks`
- `GET /api/tasks/{id}`
- `GET /api/audits`

## 如何测试

```bash
cd gateway-server && go test ./...
cd gateway-ui && npm run test -- --run
```

## 运维页面覆盖

- Dashboard：成功率/失败率/并发等指标总览
- 命令任务与文件任务：过滤、分页、详情跳转
- 任务详情：基础信息、状态流转、SIP/RTP/HTTP执行结果
- 限流策略：在线查看/更新全局限流
- 路由配置：按 api_code 编辑映射路由
- 审计日志：查询与详情查看
