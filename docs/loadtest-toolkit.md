# 统一压测工具集（loadtest）

本仓库新增统一压测工具，覆盖以下链路：

- `sip-command-create`：SIP `command.create`
- `sip-status-receipt`：SIP 状态回执链路（`command.create` + `task.status`）
- `rtp-udp-upload`：RTP 文件上传（UDP）
- `rtp-tcp-upload`：RTP 文件上传（TCP）
- `http-invoke`：A 网 HTTP invoke

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
  -duration 60s
```

## 输出结果

每次执行会在 `output-dir/<run_id>/` 下生成：

- `results.jsonl`：逐请求明细（适合自动分析）
- `summary.json`：聚合统计（适合人读 + 系统对比）

聚合指标包含：

- 吞吐（`throughput_qps`）
- 成功率（`success_rate`）
- P50/P95/P99 延迟
- 错误类型分布（`error_types`）

## 对比建议

可将多次 `summary.json` 汇总到统一仓库，按以下维度做回归对比：

- `target`
- `concurrency`
- `configured_qps`
- `file-size`
- `transfer-mode`
- `p95_ms`
- `success_rate`
