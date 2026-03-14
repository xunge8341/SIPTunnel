# SIPTunnel Monorepo

SIPTunnel 是跨安全边界业务交换网关，当前仓库为 monorepo 结构：

- `gateway-server/`：Go 网关服务（SIP/RTP/签名验签/防重放/任务状态机/HTTP 映射/审计与可观测）
- `gateway-ui/`：Vue3 运维前端（Dashboard、任务、网络配置、本端节点配置、对端节点配置、配置治理、隧道映射、限流、审计）
- `deploy/`：部署相关脚本与清单（预留）
- `scripts/`：仓库级开发脚本（启动/测试/格式化/lint）

## 产品模式与术语基线（主线）

SIPTunnel 同时支持两种产品模式，文档与 UI 必须明确区分：

1. **命令网关模式**：以 SIP 控制命令分发与状态同步为核心。
2. **HTTP 映射隧道模式（当前主线）**：以“受全局网络模式约束的 HTTP 映射转发”为核心。

当前主线统一术语：**本端节点 / 对端节点 / 网络模式 / 能力矩阵 / 隧道映射 / 本端入口 / 对端目标**。

> 兼容说明：`route` / `api_code` / `template` 属于历史模型（兼容术语），仅在迁移或兼容接口（如 `/api/routes`）中出现，不再作为默认产品术语。

## 关键能力与约束落实

- SIP 控制面：JSON Body 承载完整业务字段，Header 仅镜像索引字段（request/trace/session/api_code/message_type/source_system）。
- RTP 文件面：固定主头 + TLV 扩展协议结构在后端独立模块实现，业务代码不拼裸字节。
- 签名验签：通过 `Signer` 接口注入，当前 HMAC-SHA256，保留 `SM3_HMAC` 升级位。
- 防重放：基于 `request_id + nonce` 的接收防重放窗口。
- HTTP 执行：主模型为“隧道映射（本端入口 -> 对端目标）”；`api_code -> route template` 仅保留为历史兼容术语，不支持任意透传。
- 生产基线：限流、审计日志、trace 字段透传和结构化日志。
- 网络模式能力矩阵：`NetworkMode -> Capability` 由后端统一推导，覆盖系统信息 API、启动摘要与诊断导出（见 `docs/README.md#网络模式与能力矩阵`）。
- 映射能力联动校验：`TunnelMapping` 保存/更新会按当前 `NetworkMode/Capability` 校验 `max_request_body_bytes`、`max_response_body_bytes`、`allowed_methods` 与 `require_streaming_response`，并在 API/selfcheck/诊断暴露 warnings 或 errors。

## 如何启动

### 一键本地启动（推荐）

```bash
./scripts/dev.sh
```

该脚本会同时启动后端与前端（real API 模式）。若只需要单独运行前端，可使用：

```bash
# mock 模式（可选）
./scripts/ui-dev.sh

# real 模式（对接本地后端）
./scripts/ui-dev.sh real
```

```powershell
# mock 模式（可选）
.\scripts\ui-dev.ps1

# real 模式（对接本地后端）
.\scripts\ui-dev.ps1 -Mode real
```

### 分别启动

后端：

```bash
cd gateway-server
go run ./cmd/gateway
```

前端：

```bash
./scripts/ui-dev.sh
```

前端编译与预览：

```bash
./scripts/ui-build.sh
./scripts/ui-preview.sh
```

```powershell
.\scripts\ui-build.ps1
.\scripts\ui-preview.ps1
```

默认地址：

- 后端健康检查：`http://127.0.0.1:18080/healthz`
- 前端 Dashboard：`http://127.0.0.1:5173/dashboard`

## Windows 快速启动

面向首启体验，建议将 `gateway.exe`、`configs/`、`data/`、`logs/` 放在同一安装目录下。

```powershell
Set-Location C:\SIPTunnel
.\gateway.exe --config .\configs\config.yaml
```

Windows 下配置查找优先顺序同 Linux，但会优先尝试 **exe 所在目录**（`configs/config.yaml`、`config.yaml`），避免从快捷方式/其他目录启动时相对路径失效。

若找不到配置，`dev/test` 模式会自动生成默认配置并创建目录；并优先选择可用的友好端口（Windows 优先 `18180`，其次 `18080`）。报错信息中会附带 Windows 友好排查建议（包括 PowerShell/CMD 端口排查命令）。

另外，Windows 首次执行 `init-config` 自动生成配置时，SIP 默认端口会按 `59226 -> 15060 -> 25060 -> 35060 -> 5060` 探测可用端口，减少首启即触发 `sip.listen_port_occupancy` 的概率。

推荐首启命令：

```powershell
Set-Location C:\SIPTunnel
.\gateway.exe init-config --config .\configs\config.yaml
.\gateway.exe validate-config -f .\configs\config.yaml
.\gateway.exe --config .\configs\config.yaml
```

Windows 详细运维手册见：`docs/windows-operations.md`。

Windows 交付包组装：

```powershell
.\scripts\package-windows.ps1 -Version v0.1.0
```

## 前端联调模式

前端默认使用真实后端（`VITE_API_MODE=real`）；仅当显式设置 `VITE_API_MODE=mock` 时才走 mock：

```bash
VITE_API_BASE_URL=http://127.0.0.1:18080/api ./scripts/ui-dev.sh real
```

```powershell
$env:VITE_API_BASE_URL='http://127.0.0.1:18080/api'
.\scripts\ui-dev.ps1 -Mode real
```

页面将直接调用后端运维接口：

- `GET/PUT /api/limits`
- `GET/POST /api/mappings`
- `PUT/DELETE /api/mappings/{mapping_id}`
- `GET/PUT /api/routes`（**deprecated**，仅兼容旧版 `OpsRoute/RouteConfig`）
- `GET /api/tasks`
- `GET /api/tasks/{id}`
- `GET /api/audits`
- `GET/PUT /api/network/config`
- `GET /api/config-governance`
- `POST /api/config-governance/rollback`
- `GET /api/config-governance/export?target=current|pending`

### RouteConfig/OpsRoute → TunnelMapping 迁移（兼容过渡）

为避免历史页面/脚本一次性失效，当前版本提供“启动时自动迁移 + 显式迁移工具”双路径：

- 自动迁移：服务启动加载 `data/final/tunnel_mappings.json` 时，若检测到旧格式（`OpsRoute` 或 `RouteConfig`）会自动转换为 `TunnelMapping` 并回写新格式。
- 显式迁移：执行 `go run ./cmd/mapping-migrate --in <legacy.json> --out <tunnel_mappings.json>` 完成离线转换。
- 兼容 API：`GET/PUT /api/routes` 仍保留，但已标记 **deprecated**，仅用于过渡，建议改用 `/api/mappings`。

推荐迁移步骤：

1. 导出旧数据
   - 旧运维接口导出：`curl -s http://127.0.0.1:18080/api/routes > legacy-routes.json`
   - 旧配置文件导出：备份历史 `httpinvoke_routes*.yaml/json`。
2. 转换/导入新模型
   - 离线转换：`go run ./cmd/mapping-migrate --in legacy-routes.json --out ./data/final/tunnel_mappings.json`
   - 或直接重启 gateway，让启动时兼容层自动读取并转换。
3. 校验转换结果
   - `curl -s http://127.0.0.1:18080/api/mappings` 确认 `mapping_id/local_base_path/remote_target_*` 等字段已生成。
   - 观察 `tunnel_mappings.json` 已改写为 `{"items":[...]}` 新格式。

字段映射与丢弃规则：

- `OpsRoute.api_code -> TunnelMapping.mapping_id/name`
- `OpsRoute.http_path -> local_base_path/remote_base_path`
- `OpsRoute.http_method -> allowed_methods[0]`
- `RouteConfig.target_service -> peer_node_id`（为空时回退 `legacy-peer`）
- `RouteConfig.target_host/target_port -> remote_target_ip/remote_target_port`（无效值回退 `127.0.0.1:8080`）
- `RouteConfig.timeout_ms -> request_timeout_ms/response_timeout_ms`
- 以下旧字段不再作为主模型持久化：`retry_times/header_mapping/body_mapping/content_type`（迁移后由 TunnelMapping + 业务逻辑统一治理）。

弃用建议：

- **当前版本起**：`OpsRoute`、`RouteConfig` 仅作为兼容输入/输出模型，新增能力只在 `TunnelMapping` 维护。
- **建议切换窗口**：建议在下一个小版本迭代周期内完成所有页面、脚本到 `/api/mappings` 的切换。

运维联动文档（页面/API/CLI 统一入口）：

- Runbook（启动停止、发布回滚、链路自检、端口/transport/TCP/RTP 排障、压测前准备）：`docs/runbook.md`
- 值班手册（告警、排查顺序、升级路径、研发介入阈值）：`docs/oncall-handbook.md`
- API 清单（OpenAPI）：`gateway-server/docs/openapi-ops.yaml`



## Embedded UI（后端自宿主）模式

`gateway-server` 支持两种 UI 模式，可通过 `ui.*` 配置切换：

- `ui.enabled`：是否启用 UI 入口。
- `ui.mode`：`external`（前后端分离）或 `embedded`（静态资源嵌入后端二进制）。
- `ui.listen_ip` / `ui.listen_port`：`embedded` 模式下 HTTP 监听地址。
- `ui.base_path`：UI 挂载路径（如 `/`、`/ops`）。

### 开发模式（external，推荐本地联调）

保持前后端分离：

```bash
# 终端 1：启动后端 API
cd gateway-server
go run ./cmd/gateway --config ./configs/config.yaml

# 终端 2：启动前端开发服务器（real 模式）
VITE_API_BASE_URL=http://127.0.0.1:18080/api ./scripts/ui-dev.sh real
```

说明：
- `gateway-server` 只承载 API（`/api/*`）；UI 由 Vite dev server 承载。
- 启动日志会输出统一 `startup summary`（结构化字段可复用到日志/API/UI/诊断导出），包含：
  - `node_id`
  - `config_path` / `config_source`
  - `run_mode` / `auto_generated_config`
  - `config_candidates`（配置自动发现顺序）
  - `ui_mode` / `ui_url`
  - `api_url`
  - `sip_listen(ip/port/transport)`
  - `rtp_listen(ip/port_range/transport)`
  - `storage_dirs`
  - `business_execution`（业务执行层状态）
  - `self_check_summary`
- 可通过 `GET /api/startup-summary` 获取同一份摘要 JSON。

示例输出：

```text
startup summary:
- node_id: gateway-a-01
- config: path=./configs/config.yaml source=cli
- ui: mode=embedded url=http://127.0.0.1:18080/
- api_url: http://127.0.0.1:18080/api
- sip_listen: ip=0.0.0.0 port=5060 transport=TCP
- rtp_listen: ip=0.0.0.0 port_range=20000-20100 transport=UDP
- storage_dirs: temp=./data/temp final=./data/final audit=./data/audit log=./data/logs
- business_execution: state=protocol_only route_count=0 message=协议层可启动，业务执行层未激活（未加载下游 HTTP 隧道映射） impact=仅完成 SIP/RTP 协议交互，不会执行 A 网 HTTP 落地
- self_check_summary: generated_at=2026-01-02T03:04:05Z overall=warn info=7 warn=1 error=0
```

### embedded 模式（单进程打包交付）

1) 构建并同步 UI 静态产物到后端嵌入目录（内部会调用 `./scripts/ui-build.sh`）：

```bash
# Linux / macOS
./scripts/embed-ui.sh
```

```powershell
# Windows PowerShell
.\scripts\embed-ui.ps1
```

2) 在 `gateway-server/configs/config.yaml` 中设置：

```yaml
ui:
  enabled: true
  mode: embedded
  listen_ip: 0.0.0.0
  listen_port: 18080
  base_path: /
```

3) 启动 gateway（建议显式指定配置）：

```bash
cd gateway-server
go run ./cmd/gateway --config ./configs/config.yaml
```

embedded 模式下，网关将统一承载：
- `/api/*`（运维 API）
- `/assets/*`（前端静态资源，带缓存头）
- `/favicon.svg`（favicon）
- `/`（SPA 入口，含 Vue Router fallback）
- UI 404/500 友好页面（最小版）

### external 模式（生产部署建议）

适用于已有前端网关或静态托管能力（Nginx/对象存储/CDN）：

1) `gateway-server` 使用默认 `ui.mode: external` 启动，仅提供 API。
2) `gateway-ui` 构建产物独立部署到静态服务。
3) 将前端 API 基址指向 gateway-server 的 `/api`（可经反向代理暴露）。
4) 关注启动日志中的 `startup summary` 与 external 提示行，确保运维明确“UI 由外部承载”。

### 最小验证

```bash
./scripts/verify-embedded-ui.sh
```

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



## gateway 配置初始化 / 打印 / 校验命令

在 `gateway-server` 目录下可直接使用以下命令（均为**纯工具命令**，在 `main()` 最早阶段分流并直接退出，不会启动服务或触发 startup/self-check）：

```bash
# 生成默认配置（已存在则不覆盖）
go run ./cmd/gateway init-config

# 打印默认配置到 stdout（仅输出内容后退出）
go run ./cmd/gateway print-default-config

# 校验配置文件（仅做文件/格式/字段校验，不依赖实际网络环境）
go run ./cmd/gateway validate-config -f ./configs/config.yaml
```

启动时若找不到配置文件：

- `GATEWAY_MODE=dev/test`：自动生成默认配置并继续启动。
- `GATEWAY_MODE=prod`：生成生产模板后退出，并提示运维先修改再启动。

启动日志会明确输出：

- 是否自动生成配置（`auto_generated=true|false`）
- 配置文件路径（`config_path`）
- 配置来源与自动发现候选（`config_source` + `config_candidates`）
- 下一步建议（`next_step`）

默认目录仍会自动创建并校验可写：`data/temp`、`data/final`、`data/audit`（以及 `data/logs`）。

## 运维 Smoke Test（上线前 / 故障恢复后）

仓库提供了面向运维的一键 smoke test 套件，默认覆盖以下检查项：

- 配置加载校验（`validate-config`）
- 自检接口（`/api/selfcheck`）
- SIP listener 可用性
- RTP listener / 端口池可用性
- UI/API 可访问性（`/healthz` + `/api/startup-summary`，embedded 模式额外探测 UI URL）
- 首启摘要完整性（`run_mode/config_path/config_source`）
- 最小 command 链路（`POST /demo/process`）

### Linux

```bash
./scripts/smoke.sh
```

### Windows (PowerShell)

```powershell
./scripts/smoke.ps1
```

### 常用环境变量

- `SMOKE_START_GATEWAY=true|false`：是否由脚本自动拉起 `gateway-server`（默认 `true`）。
- `SMOKE_BASE_URL`：目标网关地址，默认 `http://127.0.0.1:${GATEWAY_PORT:-18080}`。
- `SMOKE_CONFIG_PATH`：配置文件路径，默认 `gateway-server/configs/config.yaml`。
- `SMOKE_WAIT_SECONDS`：自动拉起网关时等待健康检查的超时时间，默认 25 秒。
- `SMOKE_LOG_FILE`：自动拉起模式下 gateway 日志输出文件。

脚本执行结束会输出统一测试摘要（PASS/FAIL + 每项耗时与详情），适合上线前和故障恢复后的快速巡检。

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
- **系统状态 API**：`GET /api/system/status` 返回 `tunnel_status`、`connection_reason`、`network_mode` 与首页能力矩阵字段（`supports_small_request_body`、`supports_large_response_body`、`supports_streaming_response`、`supports_large_file_upload`、`supports_bidirectional_http_tunnel`）。
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

- 启动策略：`run_mode=prod` 时若 `overall=error` 则阻断启动；`run_mode=dev/test` 时允许降级启动（便于先进入 UI/API 做远程排障），但仍会完整输出 error 项。
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
- `GATEWAY_SELFCHECK_SUGGEST_FREE_PORT=true|false`：是否输出“建议可用端口”提示（默认：dev/test 自动开启，prod 关闭）

运维 API：

- `GET /api/selfcheck` 返回统一自检报告 JSON。

CLI/日志示例：

```text
self-check generated_at=2026-01-02T03:04:05Z overall=warn info=6 warn=1 error=0
- [WARN] sip.listen_ip: listen_ip=0.0.0.0 为通配地址，无法精确校验网卡存在性 | suggestion: 若需严格约束到指定网卡，请改为明确的本机 IP
- [INFO] sip.listen_port_occupancy: SIP 监听地址 0.0.0.0:5060 可成功绑定 | suggestion: 无需处理
- [ERROR] sip.listen_port_occupancy: SIP 端口检查失败（TCP 127.0.0.1:5060）：bind: address already in use；疑似占用进程=nginx(pid=234) | suggestion: Linux 排查可执行：ss -ltnp；lsof -i :5060；开发模式建议先切换 sip.listen_port=25060 进行快速联调（变更后请重启并复核 /api/selfcheck）；生产模式默认不自动改端口，请先释放冲突端口或人工修改 sip.listen_port
- [WARN] downstream.http_base_reachability: 未配置下游 HTTP 路由：当前处于协议层可启动、业务执行层未激活状态（仅跳过可达性检查） | suggestion: 请加载最小 httpinvoke 路由配置以激活业务执行层
```


最小业务路由配置示例（用于激活业务执行层）：

```yaml
routes:
  - api_code: api.health.ping
    target_service: local-service
    target_host: 127.0.0.1
    target_port: 19001
    http_method: POST
    http_path: /v1/ping
    content_type: application/json
    timeout_ms: 1000
    retry_times: 0
    body_mapping:
      request_id: body.request_id
```

将该文件路径写入 `GATEWAY_HTTPINVOKE_CONFIG` 后重启，可将系统状态从“协议层可启动、业务执行层未激活”切换为“业务执行层已激活”。

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

# 4) 导出诊断快照（聚合 startup-summary/healthz/selfcheck/node/limits/routes）
go run ./cmd/gatewayctl diag export --out ./diagnostics.json

# 5) JSON 输出（机器可解析）
go run ./cmd/gatewayctl --output json node inspect

# 6) 一键链路测试（SIP 控制 / RTP 端口池 / HTTP mock/downstream）
go run ./cmd/gatewayctl link test
```

链路测试说明（不影响真实业务）：

- SIP 控制链路：仅做 TCP 握手或 UDP 监听状态校验，不发送业务 SIP Body。
- RTP 端口池：只读取当前端口池统计，不分配业务传输任务。
- HTTP 下游：默认探测 `http://127.0.0.1:18080/healthz`；可通过 `GATEWAY_LINK_TEST_HTTP_TARGET` 指向专用 mock/downstream 健康探针。
- 统一输出：`passed/failed`、子项结果、`request_id/trace_id`、总耗时，适合运维快速判断。

对应 API：

- `POST /api/ops/link-test`：执行一次链路测试并返回报告。
- `GET /api/ops/link-test`：查询最近一次链路测试结果（供 UI 展示）。

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

> 端口冲突时，脚本会输出一线可直接执行的诊断命令，并在开发联调时可选输出推荐空闲端口（`AUTO_FIX_PORTS=true`，仅建议，不自动改配置）。

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
- 隧道映射：按 TunnelMapping 编辑核心业务映射（不在页面逐条编辑 transport，发布/回滚流程见 `docs/operations.md`）
- 本端节点配置：集中维护 `node_id/node_name/node_role/network_mode` 与 SIP/RTP 监听参数，并展示当前 NetworkMode/Capability 摘要。
- 对端节点配置：维护 peer signaling/media 地址范围、`supported_network_mode` 与启停状态，支持增删改查。
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
