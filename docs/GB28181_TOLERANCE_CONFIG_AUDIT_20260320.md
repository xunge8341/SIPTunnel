# GB28181 容错加固与配置覆盖审计（2026-03-20）

## 这次修复/补强了什么

1. 修复了实际编译错误：`internal/server/response_mode_policy.go` 缺失 `hasRTPDestination()`。
2. 补齐了一个真正的配置覆盖缺口：`boundary_range_response_start_wait_ms` 之前只进了配置和启动日志，没有进入运行时等待决策；现在已经进入 `dynamicRelayBodyWait()`，对内部分段 Range 拉取和外部 `Range` 请求都会生效。
3. 补强了链路日志覆盖：
   - `transport tuning applied` 启动日志现在输出完整 tuning 集合。
   - `gb28181 relay stage=response_wait_policy` 输出普通响应/Range 响应实际等待策略。
   - `gb28181 media stage=rtp_ps_policy` 输出 RTP 长流当前容错策略。
   - `mapping-runtime stage=segment_plan` / `resume_plan` 输出窗口、阈值、重试、等待等关键策略。

## 关于“乱序窗口/丢包容忍度”到底是什么意思

`boundary_rtp_reorder_window_packets` 和 `boundary_rtp_loss_tolerance_packets` 的语义是：

- **不是**允许静默丢掉这些 RTP 负载字节继续拼接正文；
- **而是**允许接收端在一定窗口内等待迟到包，暂时不把一个短时 gap 立刻判成失败；
- 如果迟到包一直补不回来，且超过 `boundary_rtp_gap_timeout_ms`，才进入恢复链路。

所以：

- 对 `Web 播放 HTTP MP4` 这种场景，**放大窗口/容忍度是合理的**；
- 对要求严格字节完整性的普通大文件下载，**仍然不能无限放大**，否则只会把失败推迟得更晚。

推荐起步值：

- 边界较稳：`8 / 2 / 1500ms`
- 边界较差但主要是 MP4 播放：`12 / 4 / 2000ms`
- 极弱网且确认不是通用文件下载主场景：`16 / 6 / 2500ms`

## 本次核查后的配置覆盖结论

### 已确认覆盖到运行链 + 日志

- `udp_control_max_bytes`
- `udp_catalog_max_bytes`
- `inline_response_udp_budget_bytes`
- `inline_response_safety_reserve_bytes`
- `inline_response_envelope_overhead_bytes`
- `inline_response_headroom_percent`
- `udp_request_parallelism_per_device`
- `udp_callback_parallelism_per_peer`
- `udp_bulk_parallelism_per_device`
- `boundary_rtp_payload_bytes`
- `boundary_rtp_bitrate_bps`
- `boundary_rtp_min_spacing_us`
- `boundary_rtp_reorder_window_packets`
- `boundary_rtp_loss_tolerance_packets`
- `boundary_rtp_gap_timeout_ms`
- `boundary_fixed_window_bytes`
- `boundary_fixed_window_threshold_bytes`
- `boundary_segment_concurrency`
- `boundary_segment_retries`
- `boundary_resume_max_attempts`
- `boundary_resume_per_range_retries`
- `boundary_response_start_wait_ms`
- `boundary_range_response_start_wait_ms`
- `boundary_http_window_bytes`
- `boundary_http_window_threshold_bytes`
- `boundary_http_segment_concurrency`
- `boundary_http_segment_retries`
- `standard_window_bytes`
- `standard_window_threshold_bytes`
- `standard_segment_concurrency`
- `standard_segment_retries`

### 之前存在、现已补齐的覆盖缺口

- `boundary_range_response_start_wait_ms`
  - 之前：只有配置和启动日志，运行时未使用。
  - 现在：进入 `dynamicRelayBodyWait()`，并且在 `response_wait_policy`、`segment_plan` 日志里可见。

## 这轮审计后，仍建议继续增加的容错点

### P1：优先补

1. **按内容类型区分恢复策略**
   - `video/*`、`audio/*`、`.mp4` 等可容忍更大的乱序窗口与 gap timeout；
   - 非媒体大文件仍保守，避免把失败掩盖成更晚的失败。

2. **对端/设备级退避**
   - 同一设备连续出现 `response_start_timeout`、`rtp_gap_timeout`、`resume_plan` 连续失败时，自动降低并发、放大等待时间。

3. **更明确的“浏览器 Range 播放”日志标签**
   - 在 `response_mode_decision`、`segment_plan`、`resume_plan` 中增加 `range_playback=true/false`，便于现场一眼区分 MP4 播放与普通下载。

### P2：建议补

4. **按 call/session 维度汇总容错统计**
   - gap 次数、reorder 恢复次数、resume 次数、最终是否成功。

5. **超时后的策略回退**
   - 同一 Range 连续多次 `rtp_gap_timeout` 后，自动退到更保守的窗口/并发档位。

## 非现场环境可以验证什么

### 可以在实验室/预发/CI 做的

1. **配置正确性**
   - YAML 解析
   - 默认值注入
   - 边界校验
   - 推荐预算公式校验

2. **响应模式决策**
   - 小 JSON / 小错误页是否走 INLINE
   - 大响应 / Range / 流式响应是否走 RTP

3. **RTP 乱序/等待行为**
   - 乱序窗口内恢复
   - 超过窗口但仍在 loss tolerance 内时只缓冲不失败
   - 超时后触发 `rtp_gap_timeout`

4. **fixed-window / resume**
   - Range 是否严格闭区间
   - `Content-Range` 越界是否被拒绝
   - retry/backoff 是否按配置执行

### 仍然必须现场验证的

1. 真正的安全边界设备对 UDP 报文的处理
2. 专网内实际 MTU / 分片 / NAT / 会话老化行为
3. 海康/大华/SRS/现场上游服务在 GB28181 + RTP 模式下的组合兼容性
4. 多路并发下的真实吞吐拐点

## 这次修改后，现场最少需要关注的日志

- `transport tuning applied`
- `gb28181 relay stage=response_wait_policy`
- `gb28181 inbound stage=response_mode_decision`
- `gb28181 media stage=rtp_ps_policy`
- `gb28181 media stage=rtp_ps_reorder_buffered`
- `gb28181 media stage=rtp_ps_gap_tolerated`
- `gb28181 media stage=rtp_ps_gap_timeout`
- `mapping-runtime stage=segment_plan`
- `mapping-runtime stage=resume_plan`

## 对现场配置的建议

如果当前主要场景是 Web 播放 HTTP MP4，且现场操作能力有限，建议先只调整这三项：

```yaml
transport_tuning:
  boundary_rtp_reorder_window_packets: 12
  boundary_rtp_loss_tolerance_packets: 4
  boundary_rtp_gap_timeout_ms: 2000
```

其他项先保持默认，不建议现场同时改太多变量。
