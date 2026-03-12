# 长稳测试（Long-run / Soak）指南

本文档提供 SIPTunnel 长稳测试能力，覆盖 1h / 6h / 24h 模式与本地 smoke 短时模式，重点关注：

- goroutine 泄漏
- FD 泄漏与连接回收失败
- 堆内存与缓冲区异常增长
- 持续业务链路下错误率波动

## 1. 测试链路覆盖

长稳测试由 `gateway-server/tests/longrun/soak_test.go` 驱动，持续执行三类行为：

1. 持续 `command.create` 控制消息处理
2. 持续 `file.create` + RTP(TCP framing) 小块传输
3. 周期性 TCP 会话重建（建立/关闭），验证连接回收

## 2. 监控指标

测试周期内会周期采样并输出：

- `goroutines`
- `fds`
- `heap_alloc_bytes`
- `heap_inuse_bytes`
- `connections`（RTP TCP 当前会话）
- `active_tasks`
- `operations_total`
- `errors_total`
- `error_rate`

输出文件：

- `tests/longrun/output/longrun-<mode>-<timestamp>.jsonl`：逐点采样
- `tests/longrun/output/longrun-<mode>-<timestamp>.md`：摘要报告

## 3. 运行方式

### 3.1 本地短时（推荐先跑）

```bash
./scripts/longrun/smoke.sh
```

默认会将 `smoke` 模式缩短到 90s，适合开发机快速验证。

### 3.2 指定标准模式（1h / 6h / 24h）

```bash
./scripts/longrun/run.sh 1h
./scripts/longrun/run.sh 6h
./scripts/longrun/run.sh 24h
```

### 3.3 自定义时长（本地排障）

```bash
LONGRUN_DURATION=10m LONGRUN_SAMPLE_INTERVAL=5s ./scripts/longrun/run.sh smoke
```

## 4. CI smoke 建议

CI 建议使用 `smoke` 档位，避免长时间占用 runner：

```bash
./scripts/longrun/smoke.sh
```

建议将报告目录作为制品上传：

- `gateway-server/tests/longrun/output/*.jsonl`
- `gateway-server/tests/longrun/output/*.md`

## 5. 推荐观测命令与阈值

> 下面命令用于“测试进程运行期间”旁路观测，便于定位泄漏与回收问题。

### 5.1 goroutine 与内存趋势

```bash
watch -n 5 "ps -o pid,rss,vsz,etime,cmd -C go"
```

阈值建议：

- 1h 档位 RSS 增长 < 256MB
- 6h 档位 RSS 增长 < 512MB
- 24h 档位 RSS 增长 < 1GB

### 5.2 FD 与连接观测

```bash
PID=$(pgrep -f 'go test ./tests/longrun' | head -n 1)
watch -n 5 "ls /proc/$PID/fd | wc -l; ss -tanp | rg $PID"
```

阈值建议：

- 稳态 FD 增量不应持续单调上升
- 测试结束后连接应回落到基线（`connections_recovered=true`）

### 5.3 错误率与活跃任务

```bash
tail -f gateway-server/tests/longrun/output/*.jsonl | jq '{ts:.timestamp,err:.error_rate,active:.active_tasks,conn:.connections}'
```

阈值建议：

- smoke 错误率 <= 5%
- 1h/6h 错误率 <= 3%
- 24h 错误率 <= 2%

## 6. 报告解读重点

摘要文件重点字段：

- `Leak suspected`：综合 goroutine / FD / 内存增长判定
- `Connections recovered`：连接是否可回收
- `Buffer growth suspect`：`HeapInuse` 峰值是否明显偏离基线

若 `Leak suspected=true` 或 `Buffer growth suspect=true`，优先排查：

1. goroutine 生命周期（ticker、channel、session read loop 是否退出）
2. TCP session 的 `Close` 覆盖率
3. payload/缓冲区切片是否长期持有
4. 错误重试路径是否产生“失败堆积”
