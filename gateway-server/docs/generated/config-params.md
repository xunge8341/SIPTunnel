# 配置参数手册（自动生成）

> 由 `go run ./cmd/configdocgen` 生成，请勿手动编辑。

高风险网络参数会标记为 `⚠️ HIGH-NET`，变更前请执行联调与端口占用检查。

| 参数名 | 类型 | 默认值 | 热更新 | 风险等级 | 说明 | 可选/校验值 |
|---|---|---|---|---|---|---|
| `storage.temp_dir` | `string` | `./data/temp` | 否 | LOW | 文件分片临时目录。 |  |
| `storage.final_dir` | `string` | `./data/final` | 否 | LOW | 文件组装完成目录。 |  |
| `storage.audit_dir` | `string` | `./data/audit` | 否 | LOW | 审计日志落盘目录。 |  |
| `storage.log_dir` | `string` | `./data/logs` | 否 | LOW | 运行日志目录。 |  |
| `network.sip.enabled` | `bool` | `true` | 是 | MEDIUM | 启用 SIP 控制面。 |  |
| `network.sip.listen_ip` | `string` | `0.0.0.0` | 否 | ⚠️ HIGH-NET | SIP 监听 IP。 |  |
| `network.sip.listen_port` | `int` | `5060` | 否 | ⚠️ HIGH-NET | SIP 监听端口。 |  |
| `network.sip.transport` | `string` | `TCP` | 否 | ⚠️ HIGH-NET | SIP 传输层协议（TCP/UDP/TLS）。 |  |
| `network.sip.advertise_ip` | `string` | `""` | 否 | MEDIUM | SIP 对端可见地址。 |  |
| `network.sip.domain` | `string` | `""` | 是 | LOW | SIP 域名。 |  |
| `network.sip.max_message_bytes` | `int` | `65535` | 是 | ⚠️ HIGH-NET | SIP 最大报文大小（UDP 超 1300 存在分片风险）。 |  |
| `network.sip.read_timeout_ms` | `int` | `5000` | 是 | MEDIUM | SIP 读超时（毫秒）。 |  |
| `network.sip.write_timeout_ms` | `int` | `5000` | 是 | MEDIUM | SIP 写超时（毫秒）。 |  |
| `network.sip.idle_timeout_ms` | `int` | `60000` | 是 | LOW | SIP 空闲连接超时（毫秒）。 |  |
| `network.sip.tcp_keepalive_enabled` | `bool` | `true` | 是 | LOW | 启用 SIP TCP keepalive。 |  |
| `network.sip.tcp_keepalive_interval_ms` | `int` | `30000` | 是 | LOW | SIP TCP keepalive 间隔（毫秒）。 |  |
| `network.sip.tcp_read_buffer_bytes` | `int` | `65536` | 是 | MEDIUM | SIP TCP 连接读缓冲区大小。 |  |
| `network.sip.tcp_write_buffer_bytes` | `int` | `65536` | 是 | MEDIUM | SIP TCP 连接写缓冲区大小。 |  |
| `network.sip.max_connections` | `int` | `2048` | 是 | ⚠️ HIGH-NET | SIP TCP 最大并发连接数。 |  |
| `network.rtp.enabled` | `bool` | `true` | 是 | MEDIUM | 启用 RTP 文件面。 |  |
| `network.rtp.listen_ip` | `string` | `0.0.0.0` | 否 | ⚠️ HIGH-NET | RTP 监听 IP。 |  |
| `network.rtp.advertise_ip` | `string` | `""` | 否 | MEDIUM | RTP 对端可见地址。 |  |
| `network.rtp.port_start` | `int` | `20000` | 否 | ⚠️ HIGH-NET | RTP 端口池起始端口。 |  |
| `network.rtp.port_end` | `int` | `20999` | 否 | ⚠️ HIGH-NET | RTP 端口池结束端口。 |  |
| `network.rtp.transport` | `string` | `UDP` | 否 | ⚠️ HIGH-NET | RTP 传输协议（UDP 生产默认，TCP 可联调验证）。 |  |
| `network.rtp.max_packet_bytes` | `int` | `1400` | 是 | ⚠️ HIGH-NET | RTP 单包大小。 |  |
| `network.rtp.max_inflight_transfers` | `int` | `64` | 是 | MEDIUM | 并发传输上限。 |  |
| `network.rtp.receive_buffer_bytes` | `int` | `4194304` | 是 | MEDIUM | RTP 接收缓冲区大小。 |  |
| `network.rtp.transfer_timeout_ms` | `int` | `30000` | 是 | MEDIUM | 文件传输超时（毫秒）。 |  |
| `network.rtp.retransmit_max_rounds` | `int` | `3` | 是 | LOW | 重传最大轮次。 |  |
| `network.rtp.tcp_read_timeout_ms` | `int` | `5000` | 是 | MEDIUM | RTP TCP 读超时（毫秒）。 |  |
| `network.rtp.tcp_write_timeout_ms` | `int` | `5000` | 是 | MEDIUM | RTP TCP 写超时（毫秒）。 |  |
| `network.rtp.tcp_keepalive_enabled` | `bool` | `true` | 是 | LOW | 启用 RTP TCP keepalive。 |  |
| `network.rtp.max_tcp_sessions` | `int` | `128` | 是 | ⚠️ HIGH-NET | RTP TCP 最大并发会话数。 |  |
| `media.port_range.start` | `int` | `20000` | 否 | MEDIUM | 部署规划媒体端口起始值。 |  |
| `media.port_range.end` | `int` | `20999` | 否 | MEDIUM | 部署规划媒体端口结束值。 |  |
| `transport_tuning.mode` | `string` | `secure_boundary` | 是 | MEDIUM | 性能优先、弱网兜底的传输策略档位。 | 可选值：secure_boundary / boundary / strict_boundary |
| `transport_tuning.udp_control_max_bytes` | `int` | `1300` | 是 | HIGH | UDP 控制面单报文预算。 | 推荐 1300；范围 [1024,1400]；低于 1249 会重现现场 oversize |
| `transport_tuning.udp_catalog_max_bytes` | `int` | `1300` | 是 | HIGH | 目录/分页查询控制面预算。 | 推荐 1300；范围 [1024,1400] |
| `transport_tuning.inline_response_udp_budget_bytes` | `int` | `1200` | 是 | HIGH | INLINE 响应可用 UDP 预算。 | 推荐 1200；范围 [1024,1400] |
| `transport_tuning.inline_response_safety_reserve_bytes` | `int` | `220` | 是 | MEDIUM | INLINE 安全预留字节。 | 推荐 220；范围 [128,512] |
| `transport_tuning.inline_response_envelope_overhead_bytes` | `int` | `320` | 是 | MEDIUM | INLINE XML/封套开销估算。 | 推荐 320；范围 [128,768] |
| `transport_tuning.inline_response_headroom_ratio` | `float64` | `0.15` | 是 | MEDIUM | INLINE 预算头寸比例。 | 推荐 0.15；范围 [0.05,0.30] |
| `transport_tuning.inline_response_headroom_percent` | `int` | `15` | 是 | LOW | 兼容旧配置的 headroom 百分比。 | 兼容字段；推荐与 ratio 保持一致 |
| `transport_tuning.udp_request_parallelism_per_device` | `int` | `6` | 是 | HIGH | 单设备普通请求并发。 | 推荐 4；范围 [2,16] |
| `transport_tuning.udp_callback_parallelism_per_peer` | `int` | `6` | 是 | HIGH | 单对端 callback 并发。 | 推荐 4；范围 [2,16] |
| `transport_tuning.udp_bulk_parallelism_per_device` | `int` | `4` | 是 | HIGH | 单设备 bulk_open 并发。 | 推荐 4；范围 [2,16] |
| `transport_tuning.udp_segment_parallelism_per_device` | `int` | `8` | 是 | HIGH | 单设备 segment-child 总并发。 | 推荐 8；范围 [2,16]；回环/同源同看场景建议 4~8 |
| `transport_tuning.udp_small_request_max_wait_ms` | `int` | `1500` | 是 | MEDIUM | small lane 最大等待时间。 | 推荐 1500；范围 [500,5000] |
| `transport_tuning.adaptive_playback_hot_window_bytes` | `int64` | `8388608` | 是 | MEDIUM | 热点播放窗口大小，用内存换更少控制面往返。 | 推荐 8MiB；范围 [4MiB,16MiB] |
| `transport_tuning.adaptive_playback_segment_cache_bytes` | `int64` | `536870912` | 是 | MEDIUM | 热点段缓存总容量。 | 推荐 512MiB；范围 [128MiB,1GiB] |
| `transport_tuning.adaptive_playback_segment_cache_ttl_ms` | `int` | `45000` | 是 | LOW | 热点段缓存 TTL。 | 推荐 45000；范围 [10000,300000] |
| `transport_tuning.adaptive_playback_prefetch_segments` | `int` | `1` | 是 | MEDIUM | 热点播放预取段数。 | 推荐 1；范围 [0,2]；稳定态默认不预取，仅热点命中后再开 |
| `transport_tuning.adaptive_primary_segment_after_failures` | `int` | `2` | 是 | MEDIUM | 连续流失败多少次后切到 segmented_primary。 | 推荐 2；范围 [1,4] |
| `transport_tuning.adaptive_loopback_playback_segment_concurrency` | `int` | `2` | 是 | MEDIUM | 本地回环源场景下的稳定态 segment 并发。 | 推荐 2；范围 [1,4]；过低会让播放体感发钝，过高会重新放大回环带宽 |
| `transport_tuning.adaptive_open_ended_range_initial_window_bytes` | `int64` | `8388608` | 是 | MEDIUM | recent probe abort 后，将 bytes=N- 改写成首个有界窗口的大小。 | 推荐 8388608(8MiB)；范围 [1048576,33554432]；用于避免先拉全量再切段 |
| `transport_tuning.boundary_rtp_payload_bytes` | `int` | `1200` | 是 | HIGH | 边界 RTP payload 大小。 | 推荐 1200；范围 [960,1200] |
| `transport_tuning.generic_segmented_primary_threshold_bytes` | `int64` | `8388608` | 是 | MEDIUM | 非音视频大文件进入 segmented-primary 的阈值。 | 推荐 8388608(8MiB)；范围 [8388608,536870912]；更早进入 segmented-primary，避免慢客户端下载长期卡在单流 |
| `transport_tuning.generic_prefetch_segments` | `int` | `0` | 是 | LOW | 非音视频大文件 segmented-primary 的预取段数。 | 推荐 0；范围 [0,2]；默认关闭预取，避免下载侧额外放大回源并发 |
| `transport_tuning.generic_download_window_bytes` | `int64` | `2097152` | 是 | MEDIUM | 非音视频大文件默认分段窗口。 | 推荐 2097152(2MiB)；范围 [1048576,67108864]；优先缩短弱网失败时的回退长度，避免单段过大 |
| `transport_tuning.generic_download_segment_concurrency` | `int` | `2` | 是 | MEDIUM | 单个非音视频下载默认 segment 并发；大文件开区间 Range 在运行时默认维持低并发保守模式。 | 推荐 2；范围 [1,8]；开区间 Range/熔断打开时会自动收敛到低并发或 1 |
| `transport_tuning.generic_download_segment_retries` | `int` | `2` | 是 | LOW | 单个下载 segment 重试次数。 | 推荐 2；范围 [0,6] |
| `transport_tuning.generic_download_resume_max_attempts` | `int` | `6` | 是 | LOW | 单个非音视频下载事务的总恢复次数。 | 推荐 6；范围 [1,12] |
| `transport_tuning.generic_download_resume_per_range_retries` | `int` | `3` | 是 | LOW | 同一下载窗口内的恢复重试次数。 | 推荐 3；范围 [1,6] |
| `transport_tuning.generic_download_penalty_wait_ms` | `int` | `500` | 是 | LOW | 下载类 rtp_sequence_gap 触发后的附加等待。 | 推荐 500；范围 [0,5000]；默认显著小于设备级 10s 罚等 |
| `transport_tuning.generic_download_total_bitrate_bps` | `int` | `33554432` | 是 | MEDIUM | 所有非音视频外层下载事务共享的总发送预算。 | 推荐 33554432；范围 [4194304,134217728]；按外层下载事务而不是按 segment child 公平分享带宽 |
| `transport_tuning.generic_download_min_per_transfer_bitrate_bps` | `int` | `2097152` | 是 | MEDIUM | 单个外层下载事务在公平整形下的最低发送速率；只有总预算足以覆盖所有活跃下载时才会启用。 | 推荐 2097152；范围 [1048576,33554432]；不会再按 segment child 逐个叠加打穿总带宽 |
| `transport_tuning.generic_download_circuit_failure_threshold` | `int` | `3` | 是 | MEDIUM | 下载类连续失败达到多少次后进入熔断降级。 | 推荐 3；范围 [1,10] |
| `transport_tuning.generic_download_circuit_open_ms` | `int` | `30000` | 是 | MEDIUM | 下载类熔断打开时长。 | 推荐 30000；范围 [1000,300000] |
| `transport_tuning.generic_download_rtp_bitrate_bps` | `int` | `8388608` | 是 | MEDIUM | 下载专用 RTP 整形速率上限。 | 推荐 8388608；范围 [2097152,67108864]；现场日志显示实际稳态远低于 16Mbps，默认改得更保守 |
| `transport_tuning.generic_download_rtp_min_spacing_us` | `int` | `650` | 是 | MEDIUM | 下载专用 RTP 包间最小间隔。 | 推荐 650；范围 [100,10000]；适当拉大 spacing，减少大文件下载的瞬时 burst |
| `transport_tuning.generic_download_rtp_socket_buffer_bytes` | `int` | `33554432` | 是 | LOW | 下载专用 RTP 套接字缓冲。 | 推荐 33554432(32MiB)；范围 [1048576,67108864]；用内存换更强乱序吸收 |
| `transport_tuning.generic_download_rtp_reorder_window_packets` | `int` | `512` | 是 | MEDIUM | 下载专用 RTP 乱序窗口。 | 推荐 512；范围 [16,1024] |
| `transport_tuning.generic_download_rtp_loss_tolerance_packets` | `int` | `192` | 是 | MEDIUM | 下载专用 RTP 丢包容忍。 | 推荐 192；范围 [8,512] |
| `transport_tuning.generic_download_rtp_gap_timeout_ms` | `int` | `900` | 是 | MEDIUM | 下载专用 RTP gap 等待超时。 | 推荐 900；范围 [100,5000] |
| `transport_tuning.generic_download_rtp_fec_enabled` | `bool` | `true` | 是 | MEDIUM | 下载专用 RTP 是否启用单丢包恢复型 XOR FEC。 | 推荐 true；关闭时可把 generic_download_rtp_fec_group_packets 设为 0 |
| `transport_tuning.generic_download_rtp_fec_group_packets` | `int` | `8` | 是 | MEDIUM | 下载专用 RTP FEC 分组包数。 | 推荐 8；范围 {0}∪[2,32]；0 表示禁用 FEC，8 约等于 12.5% 额外包开销 |
| `transport_tuning.boundary_rtp_bitrate_bps` | `int` | `16777216` | 是 | MEDIUM | 边界 RTP 发送整形速率。 | 推荐 16777216；范围 [2097152,67108864]；从 legacy 3Mbps profile 切回 boundary-rtp 后，避免默认过猛 |
| `transport_tuning.boundary_rtp_min_spacing_us` | `int` | `250` | 是 | MEDIUM | 边界 RTP 包间最小间隔。 | 推荐 250；范围 [100,5000]；越小越激进，需配合链路质量 |
| `transport_tuning.boundary_rtp_socket_buffer_bytes` | `int` | `16777216` | 是 | LOW | 边界 RTP 套接字读写缓存。 | 推荐 16777216(16MiB)；范围 [1048576,67108864]；用内存换 burst 吞吐与弱网抖动容忍 |
| `transport_tuning.boundary_rtp_reorder_window_packets` | `int` | `128` | 是 | MEDIUM | 边界 RTP 乱序窗口。 | 推荐 128；范围 [16,512] |
| `transport_tuning.boundary_rtp_loss_tolerance_packets` | `int` | `48` | 是 | MEDIUM | 边界 RTP 丢包容忍。 | 推荐 48；范围 [0,256] |
| `transport_tuning.boundary_rtp_gap_timeout_ms` | `int` | `300` | 是 | MEDIUM | 边界 RTP gap 等待超时。 | 推荐 300；范围 [100,30000] |
| `transport_tuning.boundary_rtp_fec_enabled` | `bool` | `true` | 是 | MEDIUM | 边界 RTP 是否启用单丢包恢复型 XOR FEC。 | 推荐 true；关闭时可把 boundary_rtp_fec_group_packets 设为 0 |
| `transport_tuning.boundary_rtp_fec_group_packets` | `int` | `8` | 是 | MEDIUM | 边界 RTP FEC 分组包数。 | 推荐 8；范围 {0}∪[2,32]；0 表示禁用 FEC |
| `transport_tuning.boundary_playback_rtp_reorder_window_packets` | `int` | `192` | 是 | MEDIUM | 播放场景 RTP 乱序窗口。 | 推荐 192；范围 [32,512] |
| `transport_tuning.boundary_playback_rtp_loss_tolerance_packets` | `int` | `64` | 是 | MEDIUM | 播放场景 RTP 丢包容忍。 | 推荐 64；范围 [8,256] |
| `transport_tuning.boundary_playback_rtp_gap_timeout_ms` | `int` | `450` | 是 | MEDIUM | 播放场景 RTP gap 等待超时。 | 推荐 450；范围 [100,30000] |
| `transport_tuning.boundary_playback_rtp_fec_enabled` | `bool` | `true` | 是 | MEDIUM | 播放场景 RTP 是否启用单丢包恢复型 XOR FEC。 | 推荐 true；关闭时可把 boundary_playback_rtp_fec_group_packets 设为 0 |
| `transport_tuning.boundary_playback_rtp_fec_group_packets` | `int` | `8` | 是 | MEDIUM | 播放场景 RTP FEC 分组包数。 | 推荐 8；范围 {0}∪[2,32]；0 表示禁用 FEC |
| `transport_tuning.boundary_fixed_window_bytes` | `int64` | `4194304` | 是 | MEDIUM | 边界 fallback 分段窗口大小。 | 推荐 4MiB；范围 [1MiB,8MiB] |
| `transport_tuning.boundary_fixed_window_threshold_bytes` | `int64` | `268435456` | 是 | MEDIUM | 边界大文件进入 fallback 参考阈值。 | 推荐 256MiB；范围 [64MiB,1GiB] |
| `transport_tuning.boundary_segment_concurrency` | `int` | `4` | 是 | HIGH | 边界 fallback 分段并发。 | 推荐 4；范围 [1,16] |
| `transport_tuning.boundary_segment_retries` | `int` | `1` | 是 | LOW | 边界 fallback 分段重试次数。 | 推荐 1；范围 [0,2] |
| `transport_tuning.boundary_resume_max_attempts` | `int` | `3` | 是 | MEDIUM | 边界流主路径最大 resume 次数。 | 推荐 3；范围 [1,4] |
| `transport_tuning.boundary_resume_per_range_retries` | `int` | `1` | 是 | LOW | 单窗口 range 重试次数。 | 推荐 1；范围 [0,2] |
| `transport_tuning.boundary_response_start_wait_ms` | `int` | `12000` | 是 | MEDIUM | 边界主请求 response_start 等待。 | 推荐 12000；范围 [8000,30000] |
| `transport_tuning.boundary_range_response_start_wait_ms` | `int` | `45000` | 是 | MEDIUM | 边界 range/segment response_start 等待。 | 推荐 45000；范围 [15000,60000] |
| `transport_tuning.boundary_http_window_bytes` | `int64` | `4194304` | 是 | MEDIUM | 边界 HTTP fallback 窗口大小。 | 推荐 4MiB；范围 [1MiB,8MiB] |
| `transport_tuning.boundary_http_window_threshold_bytes` | `int64` | `268435456` | 是 | MEDIUM | 边界 HTTP fallback 参考阈值。 | 推荐 256MiB；范围 [64MiB,1GiB] |
| `transport_tuning.boundary_http_segment_concurrency` | `int` | `4` | 是 | HIGH | 边界 HTTP fallback 并发。 | 推荐 4；范围 [1,16] |
| `transport_tuning.boundary_http_segment_retries` | `int` | `1` | 是 | LOW | 边界 HTTP fallback 重试次数。 | 推荐 1；范围 [0,2] |
| `transport_tuning.standard_window_bytes` | `int64` | `16777216` | 是 | MEDIUM | 普通网络 fallback 窗口大小。 | 推荐 16MiB；范围 [4MiB,32MiB] |
| `transport_tuning.standard_window_threshold_bytes` | `int64` | `268435456` | 是 | MEDIUM | 普通网络 fallback 参考阈值。 | 推荐 256MiB；范围 [64MiB,1GiB] |
| `transport_tuning.standard_segment_concurrency` | `int` | `4` | 是 | HIGH | 普通网络 fallback 分段并发。 | 推荐 4；范围 [1,16] |
| `transport_tuning.standard_segment_retries` | `int` | `1` | 是 | LOW | 普通网络 fallback 重试次数。 | 推荐 1；范围 [0,2] |
| `node.role` | `string` | `receiver` | 否 | MEDIUM | 节点角色（receiver/sender）。 |  |
