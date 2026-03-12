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


## gateway-server 路径与文件系统配置（跨平台）

gateway-server 启动时会自动检查并创建以下目录，且验证可写：

- `temp_dir`：文件分片临时落盘目录
- `final_dir`：文件组装完成后成品目录
- `audit_dir`：审计日志 JSONL 落盘目录
- `log_dir`：结构化日志文件目录

默认目录（相对 `gateway-server` 运行目录）：

- `./data/temp`
- `./data/final`
- `./data/audit`
- `./data/logs`

可通过环境变量覆盖：

- `GATEWAY_DATA_DIR`（统一根目录，自动派生 temp/final/audit/logs 子目录）
- `GATEWAY_TEMP_DIR`
- `GATEWAY_FINAL_DIR`
- `GATEWAY_AUDIT_DIR`
- `GATEWAY_LOG_DIR`

示例：

```bash
cd gateway-server
GATEWAY_DATA_DIR=./runtime-data go run ./cmd/gateway
```

若目录不可创建或不可写，服务会在启动阶段直接失败并输出可读错误信息，便于运维快速定位。

## 跨平台构建与部署检查

### 默认单文件编译

- Linux/macOS：`./scripts/build.sh`
- Windows（PowerShell）：`./scripts/build.ps1`

默认在 `dist/bin/<os>/<arch>/` 输出当前平台单可执行文件；如需一次构建多平台可使用 `matrix` 模式。


### 后端多架构构建（linux/amd64 + linux/arm64）

```bash
cd gateway-server
make build-linux-amd64
make build-linux-arm64
```

可执行文件输出目录规范：

- `dist/bin/linux/amd64/gateway`
- `dist/bin/linux/arm64/gateway`

### Docker 多架构镜像（buildx）

```bash
cd gateway-server
make docker-buildx IMAGE=your-registry/siptunnel-gateway TAG=v1.0.0 PUSH=true
```

默认（`PUSH=false`）会在本地生成 OCI 归档文件 `dist/images/gateway-<tag>.tar`，便于离线分发与验收。

如果只想在本地聚合后端 Linux 双架构产物：

```bash
cd gateway-server
make release-local
```

### 部署前配置检查（监听端口/媒体端口范围/接收发送角色）

- Linux/macOS：`LISTEN_PORT=18080 MEDIA_PORT_START=20000 MEDIA_PORT_END=20100 NODE_ROLE=receiver ./scripts/preflight.sh`
- Windows（PowerShell）：
  - `$env:LISTEN_PORT='18080'`
  - `$env:MEDIA_PORT_START='20000'`
  - `$env:MEDIA_PORT_END='20100'`
  - `$env:NODE_ROLE='receiver'`
  - `./scripts/preflight.ps1`

完整部署与操作步骤请参考 `docs/operations.md` 与 `deploy/README.md`。

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
