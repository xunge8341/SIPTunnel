# 性能诊断手册（运维 + 研发联合排障）

本文档说明如何在 SIPTunnel 中安全启用 pprof、在压测期间采样，以及如何把采样结果转成火焰图分析输入。

## 1. 能力概览

当前支持 profile 类型：

- CPU：`/debug/pprof/profile?seconds=<n>`
- heap：`/debug/pprof/heap`
- goroutine：`/debug/pprof/goroutine`
- block：`/debug/pprof/block`
- mutex：`/debug/pprof/mutex`

对应脚本：

- 采集：`scripts/perf/collect_pprof.sh`
- 导出火焰图输入：`scripts/perf/export_flame_input.sh`

## 2. 安全开启与关闭（生产建议）

> 默认关闭。只有满足「显式开启 + 令牌鉴权 + CIDR 白名单」才会启动 pprof。

### 2.1 环境变量

- `GATEWAY_PPROF_ENABLED`：是否启用（默认 `false`）
- `GATEWAY_PPROF_LISTEN_ADDR`：监听地址（默认 `127.0.0.1:6060`）
- `GATEWAY_PPROF_AUTH_TOKEN`：访问令牌（启用时必填）
- `GATEWAY_PPROF_ALLOWED_CIDRS`：允许访问 CIDR（逗号分隔，默认 `127.0.0.1/32,::1/128`）
- `GATEWAY_PPROF_BLOCK_PROFILE_RATE`：block 采样率（默认 `0`，即关闭）
- `GATEWAY_PPROF_MUTEX_PROFILE_FRACTION`：mutex 采样比例（默认 `0`，即关闭）

### 2.2 推荐开关流程

1. **运维**先临时注入令牌与白名单（建议仅堡垒机出口 IP /32）。
2. **运维**将 `GATEWAY_PPROF_ENABLED=true`，并仅监听在管理网或 loopback。
3. **研发**在压测窗口内采样（建议 1~3 分钟内完成）。
4. **运维**立即回收：
   - 关闭开关 `GATEWAY_PPROF_ENABLED=false`（或移除该环境变量）
   - 轮换 `GATEWAY_PPROF_AUTH_TOKEN`
   - 清理临时放开的 CIDR

### 2.3 systemd 示例

```bash
sudo systemctl edit siptunnel-gateway
# 在 override 中添加：
# Environment="GATEWAY_PPROF_ENABLED=true"
# Environment="GATEWAY_PPROF_LISTEN_ADDR=127.0.0.1:6060"
# Environment="GATEWAY_PPROF_AUTH_TOKEN=<随机长 token>"
# Environment="GATEWAY_PPROF_ALLOWED_CIDRS=127.0.0.1/32,10.20.30.40/32"
# Environment="GATEWAY_PPROF_BLOCK_PROFILE_RATE=1000"
# Environment="GATEWAY_PPROF_MUTEX_PROFILE_FRACTION=10"

sudo systemctl daemon-reload
sudo systemctl restart siptunnel-gateway
```

## 3. 压测期间采样

建议与 `scripts/loadtest/run.sh` 配合，在稳态阶段采样。

### 3.1 一次性采集 CPU/heap/goroutine/block/mutex

```bash
./scripts/perf/collect_pprof.sh \
  --base-url http://127.0.0.1:6060 \
  --token "$GATEWAY_PPROF_AUTH_TOKEN" \
  --duration 30 \
  --out-dir ./artifacts/pprof/loadtest-$(date +%Y%m%d-%H%M%S)
```

建议：

- CPU 建议 `30~60s`。
- block/mutex 仅在怀疑锁竞争或阻塞时开启非 0 采样率，避免长期额外开销。
- 同一轮压测至少采两次：稳态中段 + 尾段。

## 4. 热点分析流程

### 4.1 快速看热点函数

```bash
go tool pprof -top ./artifacts/pprof/<run>/cpu.pb.gz
```

### 4.2 交互式定位调用链

```bash
go tool pprof ./artifacts/pprof/<run>/cpu.pb.gz
# 在交互界面中执行：
# top
# list <func>
# web
```

### 4.3 导出火焰图输入

```bash
./scripts/perf/export_flame_input.sh --profile ./artifacts/pprof/<run>/cpu.pb.gz
```

- 若环境已安装 `inferno-collapse-pprof`：输出 `.folded`，可直接喂给 `flamegraph.pl`。
- 未安装 inferno：输出 `.pb`（pprof protobuf 中间格式），可导入支持 pprof protobuf 的可视化工具。

## 5. 联合排障分工建议

- 运维：负责窗口审批、权限收敛、开关时长控制与结果归档。
- 研发：负责采样策略、热点判读、代码层优化建议。
- 双方共识：
  - 任何生产采样必须带工单编号与时间窗。
  - 采样结束后立即关闭开关并轮换 token。
  - 报告中至少包含：压测参数、采样时间点、TopN 热点、优化建议与回归验证计划。

## 6. 诊断包导出（CLI / API / UI 一致）

诊断包统一命名规范：

- 输出目录：`diag_{nodeId}_{YYYYMMDDTHHmmssZ}[_req_{request_id}][_trace_{trace_id}]_{jobId}`
- 文件名：`diag_{nodeId}_{YYYYMMDDTHHmmssZ}[_req_{request_id}][_trace_{trace_id}]_{jobId}.zip`

> 说明：`request_id` / `trace_id` 为可选，未指定时不拼接该段；命名中的标识符会归一化为 `[a-zA-Z0-9_]` 以避免路径注入。

### 6.1 定向导出

- CLI：`gatewayctl diag export --request-id <id> --trace-id <id> --out diagnostics.json`
- API：`GET /api/diagnostics/export?request_id=<id>&trace_id=<id>`
- UI：节点状态页「导出诊断入口」支持填写 `request_id` / `trace_id` 进行定向导出。

### 6.2 诊断包文件说明

- `README.md`：目录导航、导出过滤条件（request_id/trace_id）与脱敏提示。
- `01_transport_config.json`：当前 SIP/RTP transport 配置快照。
- `02_connection_stats_snapshot.json`：SIP/RTP 连接计数与错误计数快照。
- `03_port_pool_status.json`：RTP 端口池容量、占用、分配失败计数。
- `04_transport_error_summary.json`：最近 transport 绑定/网络错误摘要。
- `05_task_failure_summary.json`：最近失败任务摘要（包含 request/trace 过滤结果，错误字段脱敏）。
- `06_rate_limit_hit_summary.json`：最近 rate limit 命中摘要（支持按 request/trace 定位）。
- `07_profile_entry.json`：pprof 采集入口信息（仅给入口与启用状态，不暴露 token）。

### 6.3 脱敏约定

- 不写出完整敏感值（如错误中的 token、密钥、鉴权串）；只保留可定位前缀。
- profile 仅输出 `enabled/listen_address/profile_url`，不输出认证凭据。
