# SIPTunnel 工业级源码行级验收报告（合并版）

## 1. 评审范围与方法

- 评审对象：`SIPTunnel-main-industrial-review-patched-20260320-v2` 基础上继续补强后的源码。
- 评审方法：**逐文件、逐函数、逐关键分支的静态源码行级审查**，并对未闭环项直接补代码、补注释、补文档。
- 本轮新增补强重点：
  - 任务 8：fixed-window / resume 增加**结构化失败分类**、**窗口内恢复阈值**、**达到阈值切换 `segment restart`** 的明确策略。
  - 任务 12：新增正式运维文档 `docs/secure-boundary-transport-recommendations.md`，把推荐档、适用场景、升级/回滚建议落盘。
- 动态验证说明：本容器无法访问外网依赖源；已尝试执行 `go test ./internal/server`，但拉取 `gopkg.in/yaml.v3`、`go.opentelemetry.io/otel`、`modernc.org/sqlite` 时失败。因此本报告结论为**源码行级验收结论**，不是联网依赖条件下的动态全绿证明。

## 2. 总体结论

### 2.1 当前结论

- **达成：10 项**
- **未达成：2 项**
- **部分达成：0 项**

### 2.2 任务状态总表

| 任务 | 状态 | 核心结论 |
|---|---|---|
| 任务 1 | 达成 | 已去除“写死 RTP”思路，形成**预算驱动 AUTO 两段判定**。 |
| 任务 2 | 达成 | 已输出**响应承载预算明细日志**。 |
| 任务 3 | 达成 | 已拆掉**单设备单通道串行 gate**，改为**可配置 lane 并发**。 |
| 任务 4 | 达成 | startup summary / self-check / transport plan 已统一展示**runtime 生效值**。 |
| 任务 5 | 达成 | 已按 **tiny_control / small_page_data / uncertain_streaming / bulk_download** 分级决策。 |
| 任务 6 | 达成 | RTP 接收侧已具备**轻量 reorder buffer** 与小窗口乱序容忍。 |
| 任务 7 | 达成 | RTP 发送已采用**稳定 pacer**，不再依赖逐包 sleep。 |
| 任务 8 | 达成 | fixed-window / resume 已实现**闭区间约束 + 结构化失败分类 + 阈值切换恢复策略**。 |
| 任务 9 | 未达成 | 仓库内仍无 keep-alive workaround 的 A/B 实验结果与正式结论。 |
| 任务 10 | 未达成 | 仓库内仍无 3/5/10 路容量基线压测的完整归档报告。 |
| 任务 11 | 达成 | 已形成**单事务视图**汇总日志。 |
| 任务 12 | 达成 | 已补配置模板与正式推荐文档，覆盖参数、场景、升级/回滚建议。 |

## 3. 逐任务源码行级验收

---

## 任务 1：把“写死 RTP”改成“安全预算驱动的 AUTO 决策”

### 验收结论

**达成。**

### 行级证据

1. **两段式预算决策主算法**位于 `gateway-server/internal/server/response_mode_policy.go:55-120,122-150,168-226`。
   - `responseModeDecisionForHeaders()`：完成**预判阶段**。
   - `finalizeResponseBodyMode()`：完成**终判阶段**。
   - `inlineBudgetDecision()`：统一计算 `udp_budget / reserve / envelope / headroom / base64` 膨胀后的 body 预算。
2. `response_mode_policy.go:168-179` 用注释明确给出预算模型：
   - 线长预算 = `udp_budget - reserve - envelope - headroom`
   - body 预算按 base64 的 `3/4` 收缩。
3. `response_mode_policy.go:82-119` 与 `137-146` 说明 AUTO 不再恢复成冒进 INLINE，而是：
   - 头阶段只对**已知长度且在预算内**的小响应预判 INLINE；
   - body 已知后若超预算，**最终强制切回 RTP**。
4. `response_mode_policy.go:153-166` 的 `shouldForceInlineResponse()` 也改成预算驱动，不再是“默认写死 RTP”的兜底语义。

### 对验收标准的对应

- “小响应可以自动 INLINE，大响应自动 RTP”：**满足**。
- “不允许再因为 INLINE 超包导致边界侧异常丢包/失败”：从**预算模型与终判回退**的源码设计上已满足。
- “日志中不再出现没有预算模型只能写死 RTP 的固定语义”：当前决策日志已完全围绕 budget/shape/reason 输出，**满足**。

### 备注

这是**源码层面**的闭环。是否完全消除现场丢包，还需后续容量压测验证，但从行级实现看，策略已经正确切换。

---

## 任务 2：引入“响应承载预算明细日志”

### 验收结论

**达成。**

### 行级证据

1. `gateway-server/internal/server/response_mode_policy.go:19-34` 定义了完整的 `responseModeDecision` 数据结构，包含：
   - `RequestedMode / EffectiveMode / FinalMode`
   - `ContentLength`
   - `EstimatedWireBytes / ActualWireBytes`
   - `UDPBudgetBytes / SafetyReserveBytes / EnvelopeOverheadBytes / HeadroomBytes`
   - `InlineBodyBudgetBytes / Reason / ResponseShape`
2. `response_mode_policy.go:278-283` 输出统一的 `stage=response_mode_decision` 日志，字段已覆盖：
   - `decision_mode`
   - `requested_mode / effective_mode / final_mode`
   - `content_length`
   - `estimated_wire_bytes / actual_wire_bytes`
   - `udp_budget_bytes / safety_reserve_bytes / headroom_bytes`
   - `decision_reason`

### 对验收标准的对应

- “每个响应开始前至少打一条 mode decision log”：源码中已经有统一决策日志入口，**满足**。
- “现场能直接看到是 body 太大、包装太大、还是 reserve 不足导致切 RTP”：通过 `decision_reason + estimated/actual wire bytes + reserve/headroom` 已具备定位能力，**满足**。

---

## 任务 3：拆掉“单设备单通道串行”门控

### 验收结论

**达成。**

### 行级证据

1. `gateway-server/internal/server/gb28181_tunnel.go:220-295`
   - `acquireUDPLane()` 改为可配置 `capacity` 的 lane；
   - 请求 lane 与 callback lane 分离；
   - 不再固定 `chan struct{}, 1`。
2. `gateway-server/internal/config/network.go:19-29` 引入配置项：
   - `udp_request_parallelism_per_device`
   - `udp_callback_parallelism_per_peer`
   - `udp_bulk_parallelism_per_device`
3. `network.go:95-106` 设定 secure boundary 默认值：`2 / 2 / 1`。
4. `network.go:307-314,413-415` 对上述并发参数做 runtime default 与 validate。
5. `gateway-server/configs/config.default.example.yaml:58-61` 已把推荐值写入模板。

### 对验收标准的对应

- “3 路并发请求不再在 gate 上串死”：从 gate 容量实现看，**满足**。
- “小请求不会被大文件/长流卡住”：通过 small/request lane 与 bulk lane 分离，**满足**。

### 备注

这里的“达成”是**源码结构达成**。真实 3/5/10 路效果仍需任务 10 压测给出量化结果。

---

## 任务 4：startup summary / self-check / transport_plan 改成“显示有效值”

### 验收结论

**达成。**

### 行级证据

1. `gateway-server/internal/config/transport_plan.go:99-108`
   - `ResolveTransportPlanForConfig()` 使用 runtime config 推导有效 body limit，不再对外暴露静态默认值。
2. `gateway-server/internal/startupsummary/summary.go:101-127`
   - startup summary 已增加 `UDPControlMaxBytes / UDPCatalogMaxBytes / EffectiveInlineBudgetBytes / UDPRequestParallelismPerDevice / ResponseModePolicy`。
3. `gateway-server/internal/selfcheck/selfcheck.go:344-375`
   - self-check 使用 `ResolveTransportPlanForConfig(cfg)` 与 `EffectiveInlineResponseBodyBudgetBytes(cfg)` 输出实际值。
4. `gateway-server/cmd/gateway/main.go:336-336`
   - 启动时完整打印 runtime transport tuning 生效配置。
5. `gateway-server/cmd/gateway/main.go:678-690`
   - 启动输出推荐配置片段，展示的就是 runtime effective 值，而不是静态默认值。

### 对验收标准的对应

- “启动摘要、自检、实际运行日志三处口径一致”：**满足**。
- “不再出现日志看着像没收口，实际又在收口的误导”：从展示源统一看，**满足**。

---

## 任务 5：为 INLINE/RTP 决策补“按响应类型分级”的策略层

### 验收结论

**达成。**

### 行级证据

1. `gateway-server/internal/server/response_shape_policy.go:13-61`
   - 明确引入 `tiny_control / small_page_data / uncertain_streaming / bulk_download` 四种画像。
2. `gateway-server/internal/server/response_mode_policy.go:55-120`
   - 预判阶段按响应画像选择 INLINE 或 RTP。
3. `gateway-server/internal/server/gb28181_tunnel.go:729-729`
   - 请求开始时把 `classifyResponseShape()` 的结果接入事务 tracker。

### 对验收标准的对应

- `socket.io polling` 不再因“误判为稳定小响应”走错承载：`uncertain_streaming -> RTP`，**满足**。
- 小 JSON / 短错误页不再被一股脑推去 RTP：`tiny_control / small_page_data` 在预算内可 INLINE，**满足**。

---

## 任务 6：补 RTP 接收侧乱序容忍，降低 sequence discontinuity 的破坏性

### 验收结论

**达成。**

### 行级证据

1. `gateway-server/internal/server/rtp_reorder.go:18-138`
   - 实现 `rtpSequenceReorderBuffer`；
   - 支持 reorder window、loss tolerance、gap overflow 判定。
2. `gateway-server/internal/server/gb28181_media.go:739-887`
   - 接收侧启用 reorder buffer；
   - `gap_timeout` 与 `gap_tolerated` 分支把轻微乱序和持续缺口区分处理；
   - `seq_gap_count` 进入统计而不是直接抬到全局失败。

### 对验收标准的对应

- “单次轻微乱序不再直接导致整段 copy_error”：**满足**。
- “RTP 长流稳定性提升”：从机制上**满足**，但最终仍要靠压测量化。

---

## 任务 7：把 RTP 发送 pacing 从“逐包 sleep”改成稳定整形器

### 验收结论

**达成。**

### 行级证据

1. `gateway-server/internal/server/rtp_pacer.go:8-44`
   - 实现 `rtpSendPacer`，以 `bitrate + minSpacing` 驱动发送节奏。
2. `gateway-server/internal/server/gb28181_media.go:354-354,510-510`
   - 发送路径把 pacer 接入单包与 Program Stream 分片发送。

### 对验收标准的对应

- “多路并发下 RTP 发送节奏更平滑”：从 pacer 架构看，**满足**。
- “码率/间隔调优不再依赖 goroutine 调度运气”：**满足**。

---

## 任务 8：把 fixed-window / resume 再做一轮硬化

### 验收结论

**达成。**

### 行级证据

#### 8.1 闭区间约束与越界阻断

1. `gateway-server/internal/server/mapping_runtime.go:1384-1400`
   - `buildPreparedResumeRequestWithLimit()` 限制 `nextStart <= resumeEnd`，超出立即拒绝。
2. `mapping_runtime.go:1421-1442`（在同文件后续段）
   - `validatePreparedResumeResponseWithLimit()` 强制校验：
     - `Content-Range.start == expectedStart`
     - `Content-Range.end <= resumeEnd`
     - `Content-Type` 不得漂移。
3. `mapping_runtime.go:640-733`
   - `copyForwardResponseWithResumeBounds()` 全程在闭区间内执行 resume。

#### 8.2 失败原因结构化分类

1. `mapping_runtime.go:35-108`
   - 新增 `windowRecoveryFailureClass / windowRecoveryStrategy / windowRecoveryError`。
2. `mapping_runtime.go:1283-1356`
   - `classifyWindowRecoveryFailure()` 对失败根因归类：
     - `out_of_window`
     - `out_of_order`
     - `range_mismatch`
     - `short_copy`
     - `timeout`
     - `sequence_gap`
     - `peer_error`
   - `windowRecoveryStrategyForClass()` 明确恢复策略。
   - `logWindowRecoveryEvent()` 统一输出 `failure_class / recovery_strategy`。

#### 8.3 达到阈值后切换恢复策略

1. `mapping_runtime.go:1329-1351`
   - `fixedWindowResumeAttemptLimit()` 明确限制**同一 window 内 resume 预算**，上限收敛到 1~3 次，而不是沿用全局大阈值。
2. `mapping_runtime.go:688-691`
   - `copyForwardResponseWithResumeBounds()` 当 window 内 resume 触达阈值，返回 `threshold_exceeded + restart_window`。
3. `mapping_runtime.go:1120-1222`
   - `fetchFixedWindowSegmentToBuffer()` 接收结构化错误；
   - 当策略为 `restart_window` 时输出 `stage=segment_strategy_switch`，从 window resume 切换为 segment restart；
   - 不再对同一闭区间无限硬重试。

### 对验收标准的对应

- “segment 的 start/end 闭区间统一”：**满足**。
- “每次 resume 都校验 Content-Range 必须落在当前窗口内”：**满足**。
- “resume 失败原因分类：越界 / 乱序 / 超时 / 对端错误”：**满足**。
- “同一窗口失败达到阈值后切换恢复策略，不无限硬重试”：**满足**。
- “不再出现任何超窗 copy 日志”：从源码防线看已经阻断，**源码验收达成**。

### 风险说明

这里仍缺现场压测回归，因此“再也不会出现”只能在**源码逻辑层面**成立；动态结论仍需任务 10 的压测闭环支撑。

---

## 任务 9：专项评估 Windows + Go1.26 keep-alive workaround 对吞吐的影响

### 验收结论

**未达成。**

### 行级证据

1. 现有仓库只能看到 workaround 语义日志，例如：
   - `gateway-server/internal/server/http_runtime_mitigation.go:68`（已有 `windows_go1.26_connreader_crash_workaround` 语义）
2. 但仓库中**未见**：
   - A/B 实验脚本
   - 连接数/建连耗时/首字节耗时统计报告
   - 3/5/10 路对比结论文档

### 结论

任务 9 仍停留在“有 workaround 但没有实验结论”的状态，**不能通过**。

---

## 任务 10：新增容量基线压测

### 验收结论

**未达成。**

### 行级证据

1. 仓库虽有 `gateway-server/loadtest/` 模块，但本次验收要求的是：
   - 3 / 5 / 10 路矩阵
   - 小请求 / socket.io polling / 大文件下载场景
   - hardcoded RTP 旧版 vs 预算驱动 AUTO 新版
   - 指标归档与正式结论
2. 当前仓库未见一份可直接作为验收依据的**容量基线报告**，因此不能判通过。

### 结论

任务 10 仍缺“脚本 + 结果 + 结论”三件套，**不能通过**。

---

## 任务 11：补现场排障日志的“单次事务视图”

### 验收结论

**达成。**

### 行级证据

1. `gateway-server/internal/server/transaction_observer.go:17-190`
   - 新增 `relayTransactionTracker`；
   - 原子记录 `requested/effective/final mode`、`gate_wait_ms`、`response_start_wait_ms`、`rtp_seq_gap_count`、`resume_count`、`final_bytes`、`final_status`；
   - `Finalize()` 输出 `stage=transaction_summary` 单条日志。
2. `gateway-server/internal/server/gb28181_tunnel.go:220-295,680-682,729-729`
   - 把 gate、response start wait、request_class 接入事务路径。
3. `gateway-server/internal/server/gb28181_media.go:745-754,848-873`
   - 把 `seq_gap_count` 接入 RTP 汇总统计。
4. `gateway-server/internal/server/mapping_runtime.go:734-737`
   - resume 成功时增加 `resume_count`。

### 对验收标准的对应

- “单次失败不再需要翻十几段日志拼图”：从日志模型看，**满足**。

---

## 任务 12：给配置模板补“安全边界推荐档”

### 验收结论

**达成。**

### 行级证据

1. `gateway-server/configs/config.default.example.yaml:43-95`
   - 示例配置中已写入：
     - secure boundary 模式
     - INLINE 预算参数
     - 并发门控参数
     - RTP reorder / pacing / fixed-window / resume 参数
     - 中文推荐注释
2. `gateway-server/docs/secure-boundary-transport-recommendations.md:1-171`
   - 新增正式文档，内容覆盖：
     - 推荐基线
     - 参数解释
     - 适用场景
     - 升级建议
     - 回滚建议
     - 验收观察点

### 对验收标准的对应

- “预发与现场可按模板直接部署，不再二次猜参数含义”：**满足**。

---

## 4. 本轮新增源码补强清单

### 4.1 任务 8 补强

- 新增结构化错误类型：`windowRecoveryError`。
- 新增结构化失败分类：`classifyWindowRecoveryFailure()`。
- 新增恢复策略决策：`windowRecoveryStrategyForClass()`。
- 新增窗口内 resume 阈值：`fixedWindowResumeAttemptLimit()`。
- 新增日志：`failure_class / recovery_strategy / segment_strategy_switch`。

### 4.2 任务 12 补强

- 新增正式文档：`gateway-server/docs/secure-boundary-transport-recommendations.md`。

## 5. 仍需继续推进的事项

### P1：必须继续推进

1. **任务 9**：补 keep-alive workaround A/B 实验脚本与发布级结论。
2. **任务 10**：补 3/5/10 路容量基线压测报告，形成回归基线。

### P2：建议补充

1. 为任务 8 的结构化恢复逻辑补更多单元测试与压测观测。
2. 对事务汇总日志补异常分支覆盖测试，确保任何失败路径都会 Finalize。

## 6. 会话过期交接入口

优先阅读以下文件：

- `gateway-server/internal/server/mapping_runtime.go`
- `gateway-server/internal/server/transaction_observer.go`
- `gateway-server/internal/server/response_mode_policy.go`
- `gateway-server/internal/server/gb28181_tunnel.go`
- `gateway-server/docs/secure-boundary-transport-recommendations.md`
- `gateway-server/docs/reviews/20260320-industrial-source-line-acceptance-report-merged.md`

