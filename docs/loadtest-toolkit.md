# 统一压测工具集（loadtest）

本仓库新增统一压测工具，覆盖以下链路：

- `sip-command-create`：SIP `command.create`
- `sip-status-receipt`：SIP 状态回执链路（`command.create` + `task.status`）
- `rtp-udp-upload`：RTP 文件上传（UDP）
- `rtp-tcp-upload`：RTP 文件上传（TCP）
- `http-invoke`：A 网 HTTP invoke

同时支持**压测-诊断联动**：压测开始、压测中、压测结束自动抓取运维诊断快照，输出可直接给运维和研发联查。

## 目录结构

- `gateway-server/cmd/loadtest/`：CLI 入口
- `gateway-server/loadtest/`：压测执行器、统计与结果写入
- `scripts/loadtest/run.sh`：一键执行脚本

## 参数说明

`go run ./gateway-server/cmd/loadtest --help`

核心参数：

- `-concurrency`：并发数
- `-qps`：全局 QPS（0 表示不限速）
- `-file-size`：压测文件大小（字节）
- `-chunk-size`：RTP 分片大小（字节）
- `-transfer-mode`：`udp|tcp|mixed`
- `-duration`：压测时长
- `-sip-address`：SIP 压测地址
- `-rtp-address`：RTP 压测地址
- `-http-url`：A 网 invoke URL
- `-gateway-base-url`：网关管理面地址（用于自动采集 `/api/node/network/status` 与 `/api/diagnostics/export`）
- `-diag-interval`：压测中诊断采样间隔（`0` 表示仅采集首尾快照）

## 快速开始

```bash
./scripts/loadtest/run.sh
```

或直接执行：

```bash
cd gateway-server
go run ./cmd/loadtest \
  -targets "sip-command-create,sip-status-receipt,rtp-udp-upload,rtp-tcp-upload,http-invoke" \
  -concurrency 50 \
  -qps 500 \
  -file-size 2097152 \
  -transfer-mode mixed \
  -duration 60s \
  -gateway-base-url http://127.0.0.1:18080 \
  -diag-interval 15s
```

## 输出结果（规范目录）

每次执行会在 `output-dir/<run_id>/` 下生成：

- `results.jsonl`：逐请求明细（适合自动分析）
- `summary.json`：聚合统计 + 诊断元信息
- `report.md`：面向运维/研发的人类可读报告
- `diagnostics/`：诊断采样目录
  - `preflight_network_status.json`
  - `preflight_diagnostics_export.json`
  - `preflight_ops_summary.txt`
  - `during_<HHMMSS>_*.json|txt`（压测中周期采样）
  - `postrun_network_status.json`
  - `postrun_diagnostics_export.json`
  - `postrun_ops_summary.txt`

## 报告内容说明

`report.md` 包含：

- 基本参数（目标、并发、QPS、时长）
- 吞吐（按 target）
- 成功率（按 target）
- 延迟分位（P50/P95/P99）
- 关键错误（error_types）
- 诊断快照文件链接（network status / diagnostics export / summary）

`*_ops_summary.txt` 提取关键信息，重点覆盖：

- transport 连接统计（`02_connection_stats_snapshot.json`）
- 端口池状态（`03_port_pool_status.json`）
- 最近错误摘要（`04_transport_error_summary.json`）

## 如何用压测结果调限流和端口池

推荐运维与研发按以下流程联动：

1. **看成功率 + 关键错误**
   - 若出现较多 `timeout`/`connection_refused`，先排 transport 连接上限与后端可用性。
   - 若 `rtp port pool exhausted` 或端口分配失败上升，优先扩 `rtp_port_pool_size`。

2. **看诊断快照中的连接与端口池趋势**
   - 对比 `preflight` 与 `postrun`：
     - `current_connections`/`rtp_tcp_sessions_current` 长时间贴近上限，需上调 `max_connections` 或降低并发。
     - `rtp_port_alloc_fail_total` 增长，说明端口池不足或回收不及时。

3. **按 capacity 规则生成建议，再回填配置**

```bash
./scripts/loadtest/capacity.sh \
  gateway-server/loadtest/results/<run_id>/summary.json \
  <当前command并发上限> <当前file并发上限> <当前端口池> <当前max_conn> <当前rps> <当前burst>
```

4. **二次压测验证**
   - 目标：成功率回升、P95 降低、`04_transport_error_summary` 错误项收敛。

## 容量评估（基于压测结果）

详细规则见 `docs/capacity.md`。
