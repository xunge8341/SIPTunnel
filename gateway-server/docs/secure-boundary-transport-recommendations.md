# Secure Boundary 传输参数推荐档

## 目标

本档用于把 GB28181 隧道的“安全边界”策略从源码经验沉淀为可部署模板，覆盖：

- INLINE / RTP 自动判定预算参数
- UDP request / callback / bulk 并发门控参数
- fixed-window / resume / RTP 乱序容忍参数
- 升级、回滚与灰度建议

## 推荐基线（secure_boundary）

```yaml
network:
  transport_tuning:
    mode: secure_boundary
    udp_control_max_bytes: 1200
    udp_catalog_max_bytes: 1200

    inline_response_udp_budget_bytes: 1200
    inline_response_safety_reserve_bytes: 220
    inline_response_envelope_overhead_bytes: 320
    inline_response_headroom_ratio: 0.15
    inline_response_headroom_percent: 15

    udp_request_parallelism_per_device: 2
    udp_callback_parallelism_per_peer: 2
    udp_bulk_parallelism_per_device: 1
    udp_small_request_max_wait_ms: 1500

    boundary_rtp_payload_bytes: 640
    boundary_rtp_bitrate_bps: 2097152
    boundary_rtp_min_spacing_us: 400
    boundary_rtp_reorder_window_packets: 8
    boundary_rtp_loss_tolerance_packets: 2
    boundary_rtp_gap_timeout_ms: 1500

    boundary_playback_rtp_reorder_window_packets: 12
    boundary_playback_rtp_loss_tolerance_packets: 4
    boundary_playback_rtp_gap_timeout_ms: 2000

    boundary_fixed_window_bytes: 1048576
    boundary_fixed_window_threshold_bytes: 1048576
    boundary_segment_concurrency: 1
    boundary_segment_retries: 8
    boundary_resume_max_attempts: 64
    boundary_resume_per_range_retries: 3

    boundary_response_start_wait_ms: 45000
    boundary_range_response_start_wait_ms: 60000
```

## 参数解释

### 1. INLINE 预算

- `inline_response_udp_budget_bytes`
  - INLINE 响应的总 UDP 预算。
  - 推荐与 `udp_control_max_bytes` 联动，不单独放大。
- `inline_response_safety_reserve_bytes`
  - 预留给 SIP header 波动、设备侧附加字段、边界抖动。
  - 建议 200~300 bytes。
- `inline_response_envelope_overhead_bytes`
  - MANSCDP XML 包装与额外字段的保守估计。
- `inline_response_headroom_ratio`
  - 更精确的生效配置，直接表达需要保留的预算比例。
  - 推荐与 `inline_response_headroom_percent` 保持一致；模板中保留两者是为了兼容旧配置。
- `inline_response_headroom_percent`
  - 在预算可用空间上继续保留的安全余量。
  - secure boundary 建议 15% 起步；网络质量较差可提高到 20%。

### 2. 并发门控

- `udp_request_parallelism_per_device`
  - 单设备控制/小响应 lane 并发。
  - 建议从 2 起步，避免 3 路并发时被 gate 串死。
- `udp_callback_parallelism_per_peer`
  - callback lane 并发，避免单 peer 被 lock 成 1。
- `udp_bulk_parallelism_per_device`
  - bulk / RTP lane 并发。
  - secure boundary 默认 1，优先保证稳定性。
- `udp_small_request_max_wait_ms`
  - small lane 等待上限；超过后宁可快速失败，也不要让小控制流长期饿死在 bulk 队列后面。

### 3. RTP 乱序与 pacing

- `boundary_rtp_reorder_window_packets`
  - 非播放型大响应的乱序容忍窗口。
- `boundary_playback_rtp_reorder_window_packets`
  - 播放类流的更宽容窗口。
- `boundary_rtp_gap_timeout_ms`
  - gap 等待超时。值越大，容忍乱序越强，但尾延迟会上升。
- `boundary_rtp_min_spacing_us`
  - pacing 最小发包间隔，避免 goroutine sleep 抖动把瞬时突发放大。

### 4. fixed-window / resume

- `boundary_fixed_window_bytes`
  - 单个 fixed window segment 的字节窗口大小。
- `boundary_segment_retries`
  - segment restart 最大次数。
- `boundary_resume_max_attempts`
  - 全局 resume 上限。
- `boundary_resume_per_range_retries`
  - 单次 range execute 的重试次数。

当前实现中，**同一 window 内的 resume 会使用更小的窗口内恢复阈值**；达到阈值后切换到 `segment restart`，避免在同一闭区间上无上限硬重试。

## 场景建议

### 场景 A：控制面为主，小 JSON / ack / 错误页较多

建议：

- 保持 `inline_response_headroom_percent=15`
- `udp_request_parallelism_per_device=2`
- `udp_bulk_parallelism_per_device=1`

目标：让 tiny_control / small_page_data 尽量 INLINE，但仍保留安全边界。

### 场景 B：socket.io polling / chunked / 长轮询较多

建议：

- 维持 AUTO 分级，不要强推 INLINE
- `boundary_response_start_wait_ms` 适度放宽
- 保持 `uncertain_streaming -> RTP` 默认策略

目标：避免把未知长度响应误判成稳定小包。

### 场景 C：大文件 / 大列表 / 下载类业务

建议：

- 保持 `boundary_fixed_window_bytes=1MiB`
- `boundary_segment_concurrency=1` 起步
- 先观察 `segment_restart` / `resume_failure` / `transaction_summary` 再决定是否加并发

目标：先保 correctness，再做吞吐调优。

## 升级建议

1. 先上线预算驱动 AUTO 与事务级汇总日志。
2. 观察 `stage=mode_decision`、`stage=resume_failure`、`stage=segment_strategy_switch`、`stage=transaction_summary`。
3. 若 3 路并发仍有 gate wait 堆积，再提升 `udp_request_parallelism_per_device`。
4. 若 RTP 序列 gap 较多，但最终未触发大量 restart，可小步增加 reorder window。

## 回滚建议

出现以下任一情况可回滚到更保守档：

- INLINE 命中率提升后，边界 UDP 丢包显著增加
- `segment_strategy_switch` 和 `resume_failure` 激增
- 设备侧出现大面积 response_start_timeout

回滚顺序建议：

1. 将 `inline_response_headroom_percent` 提高到 20
2. 将 `inline_response_safety_reserve_bytes` 提高到 260~300
3. 将 `udp_request_parallelism_per_device` 降回 1~2 的稳定值
4. 必要时将不确定流量策略强制收回 RTP

## 验收观察点

上线后建议至少检查以下日志字段：

- `decision_mode / requested_mode / effective_mode / final_mode`
- `estimated_wire_bytes / actual_wire_bytes / encoded_body_bytes`
- `udp_budget_bytes / safety_reserve_bytes / sip_header_bytes / manscdp_xml_wrap_bytes / headroom_bytes`
- `failure_class / recovery_strategy`
- `gate_wait_ms / response_start_wait_ms / rtp_seq_gap_count / resume_count / final_status`

这组字段足以支撑：

- 为什么走 INLINE / RTP
- 为什么某个 window 被 restart
- 为什么某次事务最终失败
