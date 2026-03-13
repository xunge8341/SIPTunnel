# SIPTunnel Monorepo

SIPTunnel 是跨安全边界业务交换网关，当前仓库为 monorepo 结构：

- `gateway-server/`：Go 网关服务（SIP/RTP/签名验签/防重放/任务状态机/HTTP 映射/审计与可观测）
- `gateway-ui/`：Vue3 运维前端（Dashboard、任务、网络配置、配置治理、路由、限流、审计）
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
- `GET/PUT /api/network/config`
- `GET /api/config-governance`
- `POST /api/config-governance/rollback`
- `GET /api/config-governance/export?target=current|pending`

运维联动文档（页面/API/CLI 统一入口）：

- Runbook（启动停止、发布回滚、链路自检、端口/transport/TCP/RTP 排障、压测前准备）：`docs/runbook.md`
- 值班手册（告警、排查顺序、升级路径、研发介入阈值）：`docs/oncall-handbook.md`
- API 清单（OpenAPI）：`gateway-server/docs/openapi-ops.yaml`


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


## gateway-server 配置查找优先级（启动加载）

`gateway-server` 启动时会按以下顺序查找配置文件，命中即使用：

1. CLI 参数 `--config <path>`
2. 环境变量 `GATEWAY_CONFIG`
3. 可执行文件目录下 `configs/config.yaml`
4. 可执行文件目录下 `config.yaml`
5. 当前工作目录下 `configs/config.yaml`
6. 当前工作目录下 `config.yaml`

若以上路径均不存在，程序会明确记录日志并进入**默认配置生成逻辑**（`config source=default_generated`），不再仅输出“静默回退”语义，便于运维快速判断是否误配路径。

启动日志会打印两项关键字段：

- `config path`：最终使用的配置路径（若走默认生成则为空）。
- `config source`：配置来源（`cli/env/exe_dir/cwd/default_generated`）。

示例：

```bash
cd gateway-server
go run ./cmd/gateway --config ./configs/config.yaml
```

## SIP/RTP 独立网络配置模型

`gateway-server/configs/config.yaml` 提供了 `network.sip` 与 `network.rtp` 两套完全独立的配置段：

- SIP：`enabled/listen_ip/listen_port/transport/advertise_ip/domain/max_message_bytes/read_timeout_ms/write_timeout_ms/idle_timeout_ms/tcp_keepalive_enabled/tcp_keepalive_interval_ms/tcp_read_buffer_bytes/tcp_write_buffer_bytes/max_connections`
- RTP：`enabled/listen_ip/advertise_ip/port_start/port_end/transport/max_packet_bytes/tcp_read_timeout_ms/tcp_write_timeout_ms/tcp_keepalive_enabled/max_tcp_sessions/max_inflight_transfers/receive_buffer_bytes/transfer_timeout_ms/retransmit_max_rounds`

默认值策略：首期默认 `SIP=TCP`、`RTP=UDP`；缺省字段由默认值注入器补齐，再执行分模块校验（含范围校验与端口冲突校验）。

RTP `transport` 说明：

- `UDP`（生产默认）：当前正式上线能力，作为文件面默认实现。
- `TCP`（受控发布）：已支持长度前缀帧封装（4-byte big-endian frame length）并复用 RTP 应用层头/分片/摘要/组装逻辑，可用于联调验证。
  - 适用场景：跨网链路质量波动、需要避免 UDP 丢包影响联调。
  - 限制：当前仍以联调验证为目标，生产默认与推荐模式仍为 UDP。

SIP `transport` 使用建议：

- `TCP`（默认）：适合控制消息体较大、链路稳定且需要降低分片风险的场景。
- `UDP`：适合低时延、轻量控制消息场景；建议 `sip.max_message_bytes <= 1300`。若超过该值，系统会在自检中输出 `sip.udp_message_size_risk` 告警，并在启动日志中提示风险。

观测暴露：

- **metrics**：`sip_control_route_total/sip_control_route_duration` 增加 `transport` 标签（`TCP|UDP`）。
- **节点状态 API**：`GET /api/node/network-status` 的 `data.sip` 增加连接级指标（`current_connections/accepted_connections_total/closed_connections_total/read_timeout_total/write_timeout_total/connection_error_total`）以及 TCP 生命周期配置回显字段。
- **RTP 端口池指标**：`rtp_port_pool_total/rtp_port_pool_used/rtp_port_alloc_fail_total` 用于观测文件传输端口池容量、占用和分配失败次数。
- **RTP TCP 传输指标**：`rtp_tcp_sessions_current/rtp_tcp_sessions_total/rtp_tcp_read_errors_total/rtp_tcp_write_errors_total` 用于观测 TCP 会话与 I/O 健康度。
- **日志字段**：SIP 控制面处理日志追加 `transport` 字段，启动日志打印 `sip_transport/rtp_transport`。

配置示例：

- 默认 TCP：`gateway-server/configs/config.yaml`
- UDP 示例：`gateway-server/configs/config.sip-udp.example.yaml`



## 配置参数手册自动生成

为避免参数手册与代码漂移，`gateway-server` 增加了可重复执行的配置文档生成器：

```bash
cd gateway-server
make gen-config-docs
```

生成结果：

- 参数手册：`gateway-server/docs/generated/config-params.md`
- 示例配置：`gateway-server/configs/generated/config.example.generated.yaml`
- 环境模板：
  - `gateway-server/configs/generated/config.dev.template.yaml`
  - `gateway-server/configs/generated/config.test.template.yaml`
  - `gateway-server/configs/generated/config.prod.template.yaml`

手册表格包含以下字段，便于运维审阅和变更评估：

- 参数名
- 类型
- 默认值
- 是否支持热更新
- 风险等级（高风险网络参数会标记为 `⚠️ HIGH-NET`）
- 说明

示例输出片段（Markdown）：

```markdown
| 参数名 | 类型 | 默认值 | 热更新 | 风险等级 | 说明 |
|---|---|---|---|---|---|
| `network.sip.listen_port` | `int` | `5060` | 否 | ⚠️ HIGH-NET | SIP 监听端口。 |
| `network.rtp.transport` | `string` | `UDP` | 否 | ⚠️ HIGH-NET | RTP 传输协议（UDP 生产默认，TCP 可联调验证）。 |
```


## 网络劣化测试框架（netem）

仓库新增了可复用的网络劣化测试框架，用于对 `SIP TCP / RTP UDP / RTP TCP` 在延迟、抖动、丢包、乱序、断连、带宽收缩场景下做一致化验证。

- 场景矩阵：`gateway-server/tests/netem/matrix.json`
- 执行脚本：`scripts/netem/run.sh`
- 报告模板：`gateway-server/tests/netem/report_template.md`
- 详细复现步骤：`docs/network-degradation-testing.md`

快速执行：

```bash
./scripts/netem/run.sh
```

若需接入现有探针，请通过 `NETEM_PROBE_COMMAND` 传入测试命令（输出 JSONL），框架会自动汇总成功率、平均时延、重传率与恢复时间并生成报告。


## 性能基线与关键路径 Benchmark

仓库已为关键路径补齐 benchmark，并提供统一基线记录与对比方法：

- SIP JSON decode/validate
- 签名/验签
- RTP header encode/decode
- 文件分片/文件组装
- HTTP 映射与调用封装

详见文档：`docs/performance-baseline.md`。

CI 同步加入 benchmark smoke（低强度），用于持续校验关键基准可执行性。


## 发布前回归测试套件

统一入口：

```bash
./scripts/regression.sh [local|smoke|full]
```

- `local`：本地可跑简版回归。
- `smoke`：CI 可跑 smoke 版。
- `full`：发布机可跑完整版（含 `go test ./...`）。

详细说明见：`docs/release-regression.md`。

## 上线前检查清单

- [ ] 执行 `./scripts/regression.sh local`，本地回归通过。
- [ ] 执行 `./scripts/regression.sh smoke`，CI smoke 回归通过。
- [ ] 发布机执行 `./scripts/regression.sh full`，全量回归通过。
- [ ] 在最新回归报告中确认 command/file/SIP TCP/RTP UDP/RTP TCP（若实现）/配置校验/自检/关键 API smoke 全部 PASS。
- [ ] 确认 `go test ./...` 通过。
- [ ] 检查 `artifacts/regression/` 中 Markdown 与 JSON 报告完整可追溯。

## 性能诊断（pprof）

网关新增了可控 pprof 诊断能力，默认关闭，必须同时满足以下条件才可开启：

- `GATEWAY_PPROF_ENABLED=true`
- 配置访问令牌 `GATEWAY_PPROF_AUTH_TOKEN`
- 配置网段白名单 `GATEWAY_PPROF_ALLOWED_CIDRS`

支持采集 `cpu / heap / goroutine / block / mutex` profile，并提供脚本：

- `scripts/perf/collect_pprof.sh`：采集 profile
- `scripts/perf/export_flame_input.sh`：导出火焰图输入

详细流程（压测采样、热点分析、生产安全开关）见：`docs/performance-diagnostics.md`。

## 长稳测试（1h / 6h / 24h）

仓库新增长稳（soak）测试能力，用于持续验证控制面/文件面链路在长时间运行下的稳定性，重点关注 goroutine、FD、连接回收、内存与缓冲区增长、错误率。

- 测试代码：`gateway-server/tests/longrun/soak_test.go`
- 执行脚本：`scripts/longrun/run.sh`
- CI smoke 脚本：`scripts/longrun/smoke.sh`
- 详细说明与阈值建议：`docs/longrun-testing.md`

快速运行（本地短时）：

```bash
./scripts/longrun/smoke.sh
```

标准模式：

```bash
./scripts/longrun/run.sh 1h
./scripts/longrun/run.sh 6h
./scripts/longrun/run.sh 24h
```

## gateway-server 运维环境自检

gateway-server 启动前会执行环境自检，并提供统一报告对象（可复用于 API/CLI/日志）：

- 分级：`info / warn / error`
- 每项包含：`name/level/message/suggestion`
- 覆盖：
  - `listen_ip` 网卡存在性
  - SIP 监听端口占用
  - RTP 端口范围合法性
  - RTP 传输模式规划提示（UDP 生产默认，TCP 可联调验证）
  - SIP 与 RTP 端口冲突
  - `temp/final/audit` 目录可写性
  - 下游 HTTP 基础可达性（TCP）

可选环境变量：

- `GATEWAY_NETWORK_CONFIG`：网络配置文件路径（默认 `configs/config.yaml`）
- `GATEWAY_HTTPINVOKE_CONFIG`：下游路由配置文件路径（用于可达性检查）

运维 API：

- `GET /api/selfcheck` 返回统一自检报告 JSON。

CLI/日志示例：

```text
self-check generated_at=2026-01-02T03:04:05Z overall=warn info=6 warn=1 error=0
- [WARN] sip.listen_ip: listen_ip=0.0.0.0 为通配地址，无法精确校验网卡存在性 | suggestion: 若需严格约束到指定网卡，请改为明确的本机 IP
- [INFO] sip.listen_port_occupancy: SIP 监听地址 0.0.0.0:5060 可成功绑定 | suggestion: 无需处理
- [INFO] downstream.http_base_reachability: 下游地址 127.0.0.1:19001 TCP 可达 | suggestion: 无需处理
```

API 示例：

```json
{
  "code": "OK",
  "message": "success",
  "data": {
    "generated_at": "2026-01-02T03:04:05Z",
    "overall": "warn",
    "summary": {"info": 6, "warn": 1, "error": 0},
    "items": [
      {
        "name": "sip.listen_ip",
        "level": "warn",
        "message": "listen_ip=0.0.0.0 为通配地址，无法精确校验网卡存在性",
        "suggestion": "若需严格约束到指定网卡，请改为明确的本机 IP"
      }
    ]
  }
}
```


## gatewayctl 轻量 CLI 运维工具

`gateway-server` 新增了 `gatewayctl`，用于复用现有运维 API 与配置校验能力，支持文本与 JSON 双输出。

CLI 对应操作文档：

- 日常处置动作：`docs/runbook.md`
- 值班升级规范：`docs/oncall-handbook.md`

编译与运行：

```bash
cd gateway-server
make build-gatewayctl
# 或直接运行
go run ./cmd/gatewayctl --help
```

常用命令示例：

```bash
# 1) 校验网络配置（复用 network 配置默认值注入 + Validate 逻辑）
go run ./cmd/gatewayctl config validate -f ./configs/config.yaml

# 2) 查询节点网络状态（复用 /api/node/network-status）
go run ./cmd/gatewayctl node inspect

# 3) 按 request_id 查询任务（复用 /api/tasks 过滤）
go run ./cmd/gatewayctl task query --request-id req-20260312-001

# 4) 导出诊断快照（聚合 healthz/selfcheck/node/limits/routes）
go run ./cmd/gatewayctl diag export --out ./diagnostics.json

# 5) JSON 输出（机器可解析）
go run ./cmd/gatewayctl --output json node inspect
```

可选全局参数：

- `--server`：指定网关地址（默认 `http://127.0.0.1:18080`）
- `--output, -o`：输出格式 `text|json`（默认 `text`）
- `--timeout`：API 请求超时（默认 `5s`）

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

## 生产交付脚本（安装/升级/回滚）

仓库已提供 Linux/systemd 交付脚本与模板：

- 安装前检查：`deploy/scripts/precheck.sh`
- 安装：`deploy/scripts/install.sh`
- 升级：`deploy/scripts/upgrade.sh`
- 回滚：`deploy/scripts/rollback.sh`
- systemd unit 模板：`deploy/systemd/siptunnel-gateway.service`

示例：

```bash
./deploy/scripts/precheck.sh all
RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/install.sh
RELEASE_FILE=./dist/gateway-linux-amd64 ./deploy/scripts/upgrade.sh
./deploy/scripts/rollback.sh
```

## 如何测试

```bash
cd gateway-server && go test ./...
cd gateway-ui && npm run test -- --run
```

## CI/CD 质量门禁

- 主线 CI（单元测试/协议编解码/e2e smoke/网络配置校验/benchmark smoke）：`.github/workflows/ci.yml`
- 发布前门禁（回归套件/自检/诊断采样）与夜间重压任务：`.github/workflows/pre-release.yml`
- 详细说明：`docs/ci.md`

## 运维页面覆盖

- Dashboard：成功率/失败率/并发等指标总览（值班动作见 `docs/oncall-handbook.md`）
- 命令任务与文件任务：过滤、分页、详情跳转（故障处置见 `docs/runbook.md`）
- 任务详情：基础信息、状态流转、SIP/RTP/HTTP执行结果（诊断导出见 `gatewayctl diag export`）
- 限流策略：在线查看/更新全局限流（变更前后请执行 `docs/runbook.md` 的链路自检）
- 路由配置：按 api_code 编辑映射路由（发布/回滚流程见 `docs/operations.md`）
- 审计日志：查询与详情查看（升级研发前需附带审计与诊断信息）

## 统一压测工具集（loadtest）

仓库新增 Go 实现的统一压测工具，支持 SIP command.create、SIP 状态回执链路、RTP UDP/TCP 文件上传、A 网 HTTP invoke，并输出 JSONL + JSON 聚合报告，便于人读和自动分析。

- CLI: `gateway-server/cmd/loadtest`
- 核心实现: `gateway-server/loadtest`
- 一键脚本: `scripts/loadtest/run.sh`
- 文档: `docs/loadtest-toolkit.md`

快速开始：

```bash
./scripts/loadtest/run.sh
```
