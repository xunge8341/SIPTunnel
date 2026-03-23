# HTTP over 28181 下载链路整改与源码级核对报告（2026-03-23 r5）

## 1. 本轮整改目标

针对生产新日志继续存在的“文件下载速度上不去”问题，本轮不再停留在分析，直接落实以下整改：

1. 恢复发送侧对源站的 HTTP keep-alive 复用，避免 2MiB 分段频繁重连上游。
2. 修正 generic download 对 open-ended Range 的过度保守策略，避免始终锁死 `segment_concurrency=1`。
3. 把下载链路的关键日志补齐到“下次日志回传就能直接判死因”的程度。
4. 同步调整默认配置模板，避免新包仍带出保守旧值。

---

## 2. 生产日志触发整改的直接证据

### 2.1 keep-alive 被自动关闭
生产日志已经出现：

- `http mitigation enabled scope=mapping-forward-client ... keep_alives=false source=auto reason=windows_go1.26_connreader_crash_workaround`

这意味着发送侧对源站的 HTTP Transport 被自动禁用连接复用，会放大 2MiB 分段场景的连接开销。

### 2.2 generic download 运行态被锁在单并发顺序模式
生产日志已经出现：

- `generic_download_segment_concurrency=1`
- `mode=sequential`
- `adaptive_profile=degraded`

说明当前链路已经不是“错误并桶导致 11/12/13 路平摊”，而是新的瓶颈暴露为：

- 运行态锁定单并发顺序分段
- 上游连接复用关闭
- 发送侧关键观测日志不足

### 2.3 接收侧 RTP 吞吐长期在约 5Mbps
生产日志已有：

- `body_bitrate_bps=5149544`
- `body_bitrate_bps=4595232`
- `body_bitrate_bps=4812736`

说明当前主要问题已经转向“发送侧拉源/发包链路未把配置目标真正跑出来”。

---

## 3. 本轮源码整改项（逐项落地）

### 整改项 A：发送侧 `mapping-forward-client` 默认恢复 keep-alive 复用

**整改文件**

- `gateway-server/internal/server/http_runtime_mitigation.go:39-50`

**整改内容**

在 `runtimeHTTPKeepAlivePolicy(scope string)` 中，对 `mapping-forward-client` 增加自动豁免：

- 当 Windows + Go1.26 的自动 workaround 触发时，`gateway-http` 等 server scope 仍可维持现有保护；
- 但 `mapping-forward-client` 默认改为：
  - `Disable=false`
  - `Source=auto_exempt`
  - `Reason=preserve_download_source_connection_reuse`

**整改目的**

避免 generic download 的 2MiB 分段每次都重新建上游连接，优先恢复源站连接复用能力。

---

### 整改项 B：open-ended generic download 不再一刀切锁死单并发

**整改文件**

- `gateway-server/internal/server/adaptive_delivery.go:175-212`
- `gateway-server/internal/server/mapping_runtime_delivery.go:366-404`
- `gateway-server/internal/server/mapping_runtime_delivery.go:598-603`

**整改内容**

1. 在 `adaptiveDeliveryDecision` 中新增 `ConcurrencyReason`。
2. 对 generic download + open-ended Range：
   - 保留 conservative window；
   - 但把 `SegmentConcurrency` 从硬编码 `1` 改为：
     - `min(max(1, genericDownloadSegmentConcurrency()), 2)`
   - 默认配置改为 `generic_download_segment_concurrency: 2`。
3. 在 `delivery_strategy_selected / delivery_strategy_switch / segment_plan` 日志中打印：
   - `concurrency_reason`

**整改目的**

让 open-ended 大文件下载维持“低并发保守模式”，而不是“永久单并发顺序模式”。

---

### 整改项 C：generic download 限速日志补出 `transfer_id_source`

**整改文件**

- `gateway-server/internal/server/generic_download_control.go:130-158`
- `gateway-server/internal/server/generic_download_control.go:313-320`

**整改内容**

1. 新增 `normalizeGenericDownloadTransferInfo()`，同时计算：
   - `transfer_id`
   - `transfer_id_source`
2. 让 generic download context 与 lease 都保留 `transfer_id_source`。
3. 让 `mapping-runtime stage=generic_download_rate_limit` 直接输出：
   - `transfer_id_source=download_header|target_fallback`

**整改目的**

后续生产日志可直接确认：

- 是不是再次退回了 `target_fallback`
- 是否仍在正确按下载事务做公平分享

---

### 整改项 D：发送侧 `rtp_ps_sent` 成功摘要改为默认打印

**整改文件**

- `gateway-server/internal/server/gb28181_media_sender.go:182-259`

**整改内容**

将 `gb28181 media stage=rtp_ps_sent` 从 verbose-only 改为直接 `log.Printf(...)`，并补出：

- `transfer_id`
- `transfer_id_source`
- `effective_transfer_bitrate_bps`
- `effective_segment_bitrate_bps`
- limiter 相关统计

**整改目的**

不再要求现场额外打开 `GATEWAY_GB28181_VERBOSE_LOG=true` 才能看到发送侧真实发速。

---

### 整改项 E：新增上游读取观测日志 `upstream_body_read_summary`

**整改文件**

- `gateway-server/internal/server/reader_observer.go`（新文件）
- `gateway-server/internal/server/gb28181_tunnel.go:1712-1733`

**整改内容**

新增 `observedReadCloser`，对发送侧上游 body 读取打点并输出：

- `upstream_body_bytes`
- `upstream_read_elapsed_ms`
- `upstream_body_bytes_per_sec`
- `upstream_body_bitrate_bps`
- `upstream_read_calls`
- `upstream_read_block_ms_total`
- `upstream_read_block_ms_max`
- `source=upstream_body|buffered_body`

**整改目的**

下次日志可直接区分：

- 是上游 HTTP body 自己就只有约 5Mbps
- 还是 RTP sender 把上游更高吞吐压成了约 5Mbps

---

### 整改项 F：请求头压缩日志补出 `kept_headers`

**整改文件**

- `gateway-server/internal/server/gb28181_transport_helpers.go:299-315`
- `gateway-server/internal/server/gb28181_tunnel.go:616-664`

**整改内容**

1. 新增 `headerKeySummary()`。
2. 在以下日志中新增 `kept_headers=`：
   - `request_headers_compacted`
   - `request_control_budget_rescue`
   - `request_control_severe_budget_rescue`

**整改目的**

后续现场不需要再靠猜测，就能直接确认：

- `x-siptunnel-download-transfer-id`
- `range`
- `x-siptunnel-download-profile`

是否真的被保住了。

---

## 4. 本轮配置调整（已直接改入源码包）

### 4.1 运行默认值调整

**整改文件**

- `gateway-server/internal/config/network_defaults.go:35-45`

**调整项**

- `GenericDownloadSegmentConcurrency: 1 -> 2`

### 4.2 现场配置模板调整

**整改文件**

- `gateway-server/configs/config.yaml:86-89`
- `gateway-server/configs/config.default.example.yaml`
- `gateway-server/configs/config.sip-udp.example.yaml`
- `gateway-server/configs/generated/config.dev.template.yaml`
- `gateway-server/configs/generated/config.prod.template.yaml`
- `gateway-server/configs/generated/config.test.template.yaml`
- `gateway-server/configs/generated/config.example.generated.yaml`

**调整项**

- `transport_tuning.generic_download_segment_concurrency: 1 -> 2`

### 4.3 配置文档说明同步调整

**整改文件**

- `gateway-server/internal/configdoc/metadata.go:99`
- `gateway-server/docs/generated/config-params.md:69`

**调整项**

- 默认值从 `1` 改为 `2`
- 描述从“自动降到单并发”调整为“默认维持低并发保守模式”

---

## 5. 源码级测试与核对结果

### 5.1 已完成

1. **静态语法格式校核**
   - 已对修改过的 Go 文件执行 `gofmt -w`
   - 说明本轮修改至少满足 Go 语法层面的格式化校核

2. **静态整改点核对**
   - 产出：`/mnt/data/static_validation_r5.json`
   - 校核项包括：
     - mapping-forward-client keep-alive 自动豁免存在
     - generic download 默认并发已调到 2
     - open-ended low-parallel 逻辑存在
     - `concurrency_reason` 日志存在
     - `transfer_id_source` 日志存在
     - `kept_headers` 日志存在
     - `upstream_body_read_summary` 日志存在
     - `rtp_ps_sent` 默认打印存在
     - 相关测试文本已同步更新

3. **测试代码同步**
   - `gateway-server/internal/server/http_runtime_mitigation_test.go:50-67`
   - `gateway-server/internal/server/adaptive_delivery_test.go:81-108`
   - `gateway-server/internal/startupsummary/summary_test.go`

### 5.2 未能在当前离线容器内完成的项目（如实说明）

1. **原生 `go test` 未跑通**
   - 原因：`go.mod` 要求 `go 1.25.0`
   - 当前容器只有 `go 1.23.2`
   - 工具链自动下载被离线环境阻断

2. **记录文件**
   - `go_test_attempt_r5.txt`

**结论**

- 本轮整改代码、配置、日志点位已全部落地；
- 静态核对通过；
- 原生单元测试受限于离线 Go toolchain，未在当前容器内完成，这是环境限制，不作虚假宣称。

---

## 6. 本轮交付内容

1. 修复后的源码包（含代码与配置）
2. 本整改核对报告
3. `go test` 尝试输出
4. 静态核对结果 JSON

