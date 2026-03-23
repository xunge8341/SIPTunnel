# SIPTunnel 行级源码复核与决策闭环报告（2026-03-20）

## 1. 评审组织与结论

本次按“架构策略 / 数据面与 RTP / 配置治理 / 工程可维护性”四个视角进行交叉复核，结论如下：

1. **总体策略主干已落地**：控制面 lane 分流、播放主路径与 fixed-window 兜底、泛型下载 segmented-primary、公平分享与最小日志集，代码主干与既有策略说明一致。
2. **下载公平分享主体已落地**：下载公平分享单位已从 segment child 提升为外层 transfer，segment child 速率按 transfer 总额二次分摊，`min_per_transfer` 也已按总预算覆盖条件启用。
3. **仍存在两个需要补齐的工程级缺口**：
   - 目标级熔断判断会被旧 transfer state 误伤，导致 `breakerOpenForTarget()` 的语义偏离“目标态熔断”。
   - 配置元数据 / 生成配置 / 示例配置之间存在默认值漂移，且 `config.sip-udp.example.yaml` 在本轮前存在缩进失真，影响配置可信度与可维护性。
4. **上述缺口已在本轮一并落地修复**，并补充了针对性测试与配置对齐。

---

## 2. 对照策略逐项复核

### 2.1 控制面 lane 策略

复核结果：**已落地**。

- `internal/server/response_mode_policy.go` 仍按 `small / playback / bulk_open / segment` 四类 lane 分流。
- `segmentChildParallelism()` 与 `classifyUDPRequestLane()` 的职责边界清晰，符合“控制面先分流，再按 profile 放大/限制 segment child 并发”的策略。

### 2.2 大响应主路径与兜底策略

复核结果：**已落地**。

- `copyForwardResponseAdaptive()` 先走主策略，再根据失败类型决定是否进入 fixed-window fallback。
- `buildFixedWindowPlan()` / `buildRemainingFixedWindowPlan()` / `copyForwardResponseWithFixedWindow()` 形成完整的切段兜底闭环。
- open-ended Range 改写、播放热点、自适应 delivery 决策链没有发现与既有策略冲突的分叉。

### 2.3 泛型下载公平分享策略

复核结果：**主体已落地，并在本轮补强边界实现**。

已确认落地的部分：

- `downloadTransferIDHeader` 已打通上级请求、内部分段子请求和下级 RTP 发送链路。
- `genericDownloadController.acquire()` 已按 `(device, target, transferID)` 聚合 transfer，并将 `effectiveTransferBPS` 按 `activeSegmentsTransfer` 二次分摊为 child 速率。
- `generic_download_rate_limit` 日志已输出 transfer 粒度与 child 粒度的关键字段。

本轮新增补强的部分：

- 将**目标级熔断态**从“扫描所有 state”改为“只读目标聚合态”，避免旧 transfer 开闸残留误伤新请求。
- 新 transfer 在 `acquire()` 时会同时感知目标聚合 breaker，从而使**新事务**也能立即受到目标级熔断降速约束。

### 2.4 默认值、注释、配置模板一致性

复核结果：**本轮前存在明显漂移，本轮已补齐**。

发现的漂移项：

- `generic_segmented_primary_threshold_bytes`
- `generic_prefetch_segments`
- `boundary_segment_concurrency`
- `boundary_http_segment_concurrency`
- `standard_segment_concurrency`

这些参数在 `DefaultTransportTuningConfig()`、`internal/configdoc/metadata.go`、`docs/generated/config-params.md`、`configs/generated/*.yaml`、`configs/*.yaml` 之间并不一致；如果不修，会造成“代码实际默认值”和“运维看到的默认值”长期分叉。

---

## 3. 本轮落地修复（带行级锚点）

### 3.1 修复：目标级熔断被旧 transfer state 误伤

**问题判断**

原逻辑中，`breakerOpenForTarget()` 会遍历同一 target 的所有 state。由于 transfer state 以 `(device,target,transferID)` 为 key，旧 transfer 的开闸记录即使已经不应该继续影响目标级决策，仍可能让后续 `breakerOpenForTarget()` 返回 true。

**风险**

- 自适应层会被误导为“目标仍处于熔断态”。
- 新 transfer 的并发与预取会被持续压低。
- 目标级 breaker 与 transfer 级 breaker 语义混杂，日志难以解释。

**已落地方案**

1. 引入 `genericDownloadTargetStateKey()`，将目标级 breaker 与 transfer state 的定位方式显式区分。
2. 新增 `targetBreakerOpenLocked()`，只读取目标聚合态，而不扫描所有 transfer state。
3. `acquire()` 中的 `breakerOpen` 由“仅看 transfer 态”调整为“transfer 态 OR 目标聚合态”。
4. `breakerOpenForTarget()` 改为只走目标聚合态路径。

**源码锚点**

- `internal/server/generic_download_control.go:41`
- `internal/server/generic_download_control.go:193-211`
- `internal/server/generic_download_control.go:244`
- `internal/server/generic_download_control.go:362-393`

### 3.2 修复：state 生命周期不闭环，存在 stale state 堆积

**问题判断**

原逻辑会持续往 `states` 中写入 transfer 级 state，但成功完成、breaker 已失效或永不再访问的旧 state 缺少系统化清理路径。由于下载 transferID 默认取 requestID，这类 state 容易随请求数线性累积。

**风险**

- 内存与 map 扫描成本持续增长。
- stale state 会放大误判面，尤其在目标级 breaker 语义本就混杂时更危险。

**已落地方案**

1. 在 state 中增加 `LastTouchedAt`，在 controller 中增加 `lastPruneAt`。
2. 抽出 `deleteIfIdleAndInactiveLocked()`，对“无活跃 segment、无打开 breaker、无失败历史”的 state 立即删除。
3. 抽出 `pruneIdleStatesLocked()`，按分钟粒度做一次统一清理，删除已失效且空闲的 state。
4. `release()` 对未开闸的失败 transfer 在收尾时直接删除，避免无意义保留一次性 transfer 记录。

**源码锚点**

- `internal/server/generic_download_control.go:23`
- `internal/server/generic_download_control.go:36`
- `internal/server/generic_download_control.go:157-190`
- `internal/server/generic_download_control.go:297-359`

### 3.3 修复：配置元数据、生成配置、示例配置与真实默认值漂移

**问题判断**

经逐项比对，`DefaultTransportTuningConfig()` 与配置元数据/生成文件存在 5 处真实默认值不一致，属于“代码策略已变、配置基线未跟上”的典型漂移。

**已落地方案**

统一以下默认值与说明：

- `generic_segmented_primary_threshold_bytes = 8388608`
- `generic_prefetch_segments = 0`
- `boundary_segment_concurrency = 4`
- `boundary_http_segment_concurrency = 4`
- `standard_segment_concurrency = 4`

**源码与配置锚点**

- `internal/configdoc/metadata.go:96-97`
- `internal/configdoc/metadata.go:125`
- `internal/configdoc/metadata.go:133`
- `internal/configdoc/metadata.go:137`
- `configs/config.default.example.yaml:66,87,117,129,135`
- `configs/config.yaml:85,106,136,148,154`
- `configs/config.sip-udp.example.yaml:63,84,114,126,132`
- `configs/generated/config.example.generated.yaml:134,138,250,282,298`
- `docs/generated/config-params.md:66-67,95,103,107`

### 3.4 修复：`config.sip-udp.example.yaml` 缩进失真

**问题判断**

该文件在本轮前，`transport_tuning` 段内多处键缩进层级错乱，会让 YAML 结构偏离预期，存在误导使用者甚至直接导致解析异常的风险。

**已落地方案**

- 按 `config.default.example.yaml` 的最新 `transport_tuning` 块整体重排。
- 重新校验 `config.sip-udp.example.yaml` 的 YAML 可解析性。

**源码锚点**

- `configs/config.sip-udp.example.yaml:39-135`

---

## 4. 测试与验证

### 4.1 本轮新增/补强测试

新增了两个更贴近本轮修复目标的单测：

1. `TestGenericDownloadControllerTargetBreakerShapesNewTransfers`
   - 证明目标级 breaker 打开后，新 transfer 会立即被降速。
2. `TestGenericDownloadControllerBreakerOpenForTargetIgnoresStaleTransferState`
   - 证明 `breakerOpenForTarget()` 不再被旧 transfer state 误伤。

**源码锚点**

- `internal/server/generic_download_control_test.go:61-78`
- `internal/server/generic_download_control_test.go:80-93`

### 4.2 已完成的本地验证

- 已对修改过的 Go 源文件执行 `gofmt`
- 已使用 Python 对以下 YAML 做结构化解析验证：
  - `configs/config.yaml`
  - `configs/config.default.example.yaml`
  - `configs/config.sip-udp.example.yaml`
  - `configs/generated/config.example.generated.yaml`
  - `configs/generated/config.dev.template.yaml`
  - `configs/generated/config.prod.template.yaml`
  - `configs/generated/config.test.template.yaml`

### 4.3 当前环境下未完成项

- `go test ./internal/server -count=1` 未能完成。
- 原因不是用例失败，而是当前执行环境无法访问 `proxy.golang.org` 拉取依赖，属于**环境性阻塞**，本报告不虚报“测试全部通过”。

---

## 5. Microsoft 风格与可维护性处理

本轮不是只修逻辑，还同步做了以下工程化整理：

1. **职责显式化**：把“目标态 breaker”“transfer 态 breaker”“state 清理”拆成具名 helper，避免一个函数同时承载统计、清理、判定三类责任。
2. **语义前置**：关键 helper 命名更接近业务语义，例如 `genericDownloadTargetStateKey()`、`targetBreakerOpenLocked()`、`deleteIfIdleAndInactiveLocked()`。
3. **最小必要注释**：保留策略性注释，避免啰嗦型注释掩盖真正关键的边界条件。
4. **配置闭环**：默认值变更不只改代码，而是同时改 metadata / generated config / example config / 参数文档，避免后续再出现“代码与文档两套真相”。

---

## 6. 专家组最终结论

### 已确认落地

- 控制面 lane 分流
- 大响应主路径 + fallback fixed-window
- 泛型下载按外层 transfer 公平分享
- child 速率按 transfer 总额二次分摊
- `min_per_transfer` 不再打穿总预算
- 下载关键日志字段已打全

### 本轮额外补齐并已落地

- 目标级 breaker 不再被 stale transfer state 误伤
- 新 transfer 能立即感知目标聚合 breaker
- state 生命周期完成“创建—使用—清理”闭环
- 配置基线、参数文档、示例 YAML、生成模板全部对齐
- `config.sip-udp.example.yaml` 缩进错误已修复并验证可解析

### 仍建议下一轮继续推进（本轮未声称已落地）

- 将 `gap_tolerated` / 长期低吞吐 / 持续 pacing 压降等**软拥塞信号**纳入 breaker 或 adaptive profile 触发条件；这是已有验收报告里明确提示的下一步方向，具备充分现场依据，但不应在没有额外现场样本前仓促硬编码。

