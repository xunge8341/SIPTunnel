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


## gateway-server SIP 控制面服务骨架

`gateway-server/internal/service/sipcontrol` 新增控制面服务骨架，当前能力：

- 抽象接口：`Receiver`、`Sender`、`Router`、`Handler`，便于后续替换真实 SIP 网络适配。
- Dispatcher 路由：按 `message_type` 分发至 handler。
- 统一处理流程：JSON 解析、签名校验、时间窗校验、统一请求上下文（`request_id/trace_id/session_id`）构建。
- 内置 handler 骨架：`command.create`、`file.create`、`file.retransmit.request`、`task.cancel`，并返回 `command.accepted` / `file.accepted`。
- 预留日志和 metrics 埋点，供观测系统接入。

## gateway-server 任务引擎与持久化

`gateway-server/internal/service/taskengine` 与 `gateway-server/internal/repository` 新增任务域能力：

- 双状态机：分别支持命令任务、文件任务的状态流转校验。
- 仓储接口：统一定义 `CreateTask`、`UpdateTaskStatus`、`GetTaskByID`、`ListTasks`、`SaveTaskEvent`。
- 内存实现：用于开发与单元测试场景。
- SQL/SQLite 实现骨架：支持后续接入真实数据库驱动。
- 基础重试与死信池：失败后进入重试等待，超过最大尝试次数进入死信队列，并支持重放。
- 迁移脚本骨架：`gateway-server/migrations/0001_task_engine.up.sql` / `.down.sql`。

## RTP 文件面应用层协议

`gateway-server/internal/protocol/rtpfile` 提供 RTP 文件传输应用层协议库：

- 二进制定长主头（magic/version/header_length/flags/transfer_id/request_id/trace_id/chunk 元数据/摘要/时间戳）
- 可选 TLV 扩展段（type/length/value），解码时可跳过未知 type
- `MarshalBinary()` / `UnmarshalBinary()` 编解码及头长校验
- 分片与重组工具（按 chunk size 切片、生成每片头、计算 chunk/file digest、支持乱序与重复片）

## gateway-server A 网 HTTP 落地执行模块

`gateway-server/internal/service/httpinvoke` 提供基于 `api_code` 路由模板的 HTTP 执行能力，避免透传任意目标地址：

- 路由白名单：仅允许命中配置的 `api_code`，未知编码会被拦截。
- 配置加载：支持从 YAML 加载 `api_code/target_service/target_host/target_port/http_method/http_path/content_type/timeout_ms/retry_times/header_mapping/body_mapping`。
- 参数映射：使用点路径（如 `body.order_id`）从入参映射到目标 Header 与 JSON Body。
- 统一 Header 注入：`X-Request-ID`、`X-Trace-ID`、`X-Session-ID`、`X-Transfer-ID`、`X-Api-Code`、`X-Source-System`、`X-Idempotent-Key`。
- 调用控制：支持超时控制与重试（429/5xx/504）。
- 结果码映射：HTTP 状态码映射为统一 `result_code`（如 `OK`、`UPSTREAM_TIMEOUT`、`UPSTREAM_RATE_LIMIT`、`UPSTREAM_SERVER_ERROR`）。

示例路由配置见：`gateway-server/configs/httpinvoke_routes.example.yaml`。
