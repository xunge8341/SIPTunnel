# SIPTunnel 工业级源码行级验收报告 v2

## 结论

- 评审方式：静态源码逐文件/逐关键路径审查 + 本轮继续补强源码。
- 结论：**达成 8 项，部分达成 2 项，未达成 2 项。**
- 本轮继续推进的核心增量：**任务11 单事务视图落地**、**关键算法注释补齐**、**resume/rtp gap 指标接入事务汇总日志**。
- 限制：当前容器无法访问 Go 依赖源，**未完成 go test / go build 联网依赖验证**；本报告结论属于**静态行级审查结论**，不是动态全绿证明。

## 状态汇总

| 状态 | 任务 |
|---|---|
| 达成 | 任务1、任务2、任务3、任务4、任务5、任务6、任务7、任务11 |
| 部分达成 | 任务8、任务12 |
| 未达成 | 任务9、任务10 |

## 逐任务行级确认表

| 项目 | 状态 | 验收点 | 行级证据 | 评审结论 | 缺口/后续 |
|---|---|---|---|---|---|
| 任务1 | 达成 | 去掉 hardcoded RTP，改为安全预算驱动的 AUTO 两段判定 | response_mode_policy.go:55-120, 122-166, 168-219; gb28181_tunnel.go:1527-1587 | 已存在“预判 + 终判”两段式；预算显式计入 UDP budget / reserve / envelope / headroom / base64 膨胀；AUTO 不再等价冒进 INLINE。 | 无阻断缺口。建议后续补预算单测覆盖边界值。 |
| 任务1-日志语义 | 达成 | 不再出现“没有预算模型只能写死 RTP”的固定语义 | response_mode_policy.go:267-283; gb28181_tunnel.go:1537,1587 | 判定日志已由 budget/shape/reason 驱动，语义已从 hardcoded RTP 转为预算模型。 | 建议现场验证旧关键字日志已不再出现。 |
| 任务2 | 达成 | 新增响应承载预算明细日志 | response_mode_policy.go:267-283 | 已输出 decision_mode / requested_mode / effective_mode / final_mode / content_length / estimated_wire_bytes / actual_wire_bytes / udp_budget_bytes / safety_reserve_bytes / headroom_bytes / decision_reason。 | 字段完整度满足需求；可再考虑加入 Content-Type 便于现场筛选。 |
| 任务3-并发门控 | 达成 | 拆掉单设备单通道串行 gate，改为可配置并发 lane | gb28181_tunnel.go:220-295; config/network.go:23-29,100-106,307-314; configs/config.default.example.yaml:53-61 | request gate / callback gate / bulk lane 已分离，且并发度可配置，不再硬编码为 1。 | 仍建议补压测证明 3/5/10 路下 wait p95/p99 改善幅度。 |
| 任务4-有效值展示 | 达成 | startup summary / self-check / transport_plan 输出 runtime 生效值 | startupsummary/summary.go:102-127; selfcheck/selfcheck.go:375; cmd/gateway/main.go:336,677-683; config/transport_plan.go:99-103 | 摘要、自检、运行期 transport tuning 日志均已转向 runtime 生效值与 effective inline budget。 | 建议再补一条回归用例，防止未来又回退到静态默认值。 |
| 任务5-响应类型分级 | 达成 | 按 tiny_control / small_page_data / uncertain_streaming / bulk_download 分级 | response_shape_policy.go:8-61; response_mode_policy.go:63-120; gb28181_tunnel.go:1527-1587 | 已按响应画像分层；socket.io/chunked/SSE 归 uncertain_streaming，bulk 下载默认 RTP，小文本/JSON 可 INLINE。 | 可继续补更多 MIME/path 特征单测。 |
| 任务6-RTP乱序容忍 | 达成 | 接收侧增加 reorder buffer、小窗口乱序容忍、resume 兜底 | rtp_reorder.go:15-138; gb28181_media.go:738-888 | 已有 reorder window + loss tolerance + gap timeout；轻微乱序不直接抬到 resume 层。 | 建议继续补统计图与长期稳定性压测。 |
| 任务7-RTP pacing | 达成 | 从逐包 sleep 改为稳定整形器 | rtp_pacer.go:8-58; gb28181_media.go:354-380,478-530 | 发送侧已使用 rtpSendPacer，根据 bitrate + minSpacing 控节奏；组帧与发送 pacing 已解耦。 | 建议增加 tuning 参数与实测吞吐曲线对应文档。 |
| 任务8-fixed-window/resume 安全边界 | 部分达成 | segment 闭区间统一、resume 必须落在当前窗口内 | mapping_runtime.go:564-632,1095-1115,1188-1234 | 当前实现已对 nextStart/resumeEnd/content-range start/end 做闭区间约束，能阻断超窗恢复。 | 仍缺“窗口失败原因结构化分类 + 阈值切换恢复策略 + 明确 stage 级错误码”，因此只能判部分达成。 |
| 任务9-Windows keep-alive workaround A/B | 未达成 | 输出 workaround 开/关的吞吐实验结论 | http_runtime_mitigation.go:68 仅见 workaround 日志，无 A/B 实验脚本/结果 | 代码能看到 workaround 开关语义，但仓库中未见系统化 A/B 实验框架、统计输出或结论文档。 | 需补实验脚本、指标采集和正式结论。 |
| 任务10-容量基线压测 | 未达成 | 3/5/10 路与多场景矩阵压测 | 未发现成体系压测脚本/报告 | 当前仓库没有可回归的容量基线闭环材料。 | 需补基准脚本、指标面板、结果归档。 |
| 任务11-单次事务视图 | 达成 | 单条事务日志串起 call_id/mapping_id/device_id/request_class/requested/effective/final/gate_wait/response_start_wait/rtp_seq_gap_count/resume_count/final_bytes/final_status | transaction_observer.go:17-210; gb28181_tunnel.go:443-479,692-729,758-781; mapping_runtime.go:630-633; gb28181_media.go:633-754,848-872 | 本轮已新增 transaction_summary 汇总器，并把 gate wait / response start wait / resume_count / rtp_seq_gap_count / final_bytes / final_status 接到同一条日志。 | 建议现场确认所有异常分支都会触发 body close/Finalize；可再加单测。 |
| 任务12-模板与推荐档 | 部分达成 | 配置模板补安全边界推荐档、INLINE 预算参数、并发门控参数、场景说明 | configs/config.default.example.yaml:43-61; cmd/gateway/main.go:677-683 | 示例配置已包含 secure_boundary 推荐值与关键注释，启动输出也会解释建议值。 | docs 下尚缺成体系升级/回滚指引与场景化部署说明，因此只能判部分达成。 |
| 本轮新增-算法注释 | 达成 | 代码中要有明确算法实现注释说明 | response_mode_policy.go:55-62,122-126,168-179,207-209; mapping_runtime.go:564-569,1188-1190,1222-1225; gb28181_tunnel.go:220-224; transaction_observer.go:17-23 | 预算模型、两段决策、resume 边界、lane 设计、事务汇总器都已补中文注释，满足“源码可读、可交接”的工业要求。 | 建议后续保持注释与实现同步更新。 |

## 本轮继续推进的源码补强

1. **单事务视图**：新增 `gateway-server/internal/server/transaction_observer.go`，把 `call_id / mapping_id / device_id / request_class / requested_mode / effective_mode / final_mode / gate_wait_ms / response_start_wait_ms / rtp_seq_gap_count / resume_count / final_bytes / final_status` 汇总到 `stage=transaction_summary` 单条日志。
2. **resume 指标接线**：在 `mapping_runtime.go:630-633` 将成功 resume 次数接入 tracker。
3. **RTP seq gap 指标接线**：在 `gb28181_media.go:745-754,851,864,872` 增补 `seq_gap_count` 统计与输出。
4. **算法注释**：对预算决策、终判回退、UDP lane、resume 闭区间约束等关键逻辑补上显式中文注释，便于跨会话交接。

## 高风险未闭环项

- **任务8**：虽然闭区间约束已经到位，但窗口级失败原因仍未结构化到统一枚举，且没有达到阈值后的恢复策略切换。
- **任务9**：没有 A/B 实验，不足以判断 keep-alive workaround 是否构成吞吐瓶颈。
- **任务10**：没有容量基线压测矩阵与归档结果，无法工业化回归。
- **任务12**：模板有了，文档闭环还不够，尤其缺升级/回滚手册。

## 会话过期交接建议

- 优先继续补 **任务8** 的“结构化失败原因 + 阈值切换策略 + 单测”。
- 然后补 **任务9/10** 的实验脚本与报告产物。
- 当前最关键的跨会话入口文件：
  - `gateway-server/internal/server/transaction_observer.go`
  - `gateway-server/internal/server/gb28181_tunnel.go`
  - `gateway-server/internal/server/mapping_runtime.go`
  - `gateway-server/internal/server/gb28181_media.go`
  - `gateway-server/docs/reviews/20260320-industrial-line-review-v2.md`