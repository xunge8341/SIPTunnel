package configdoc

import (
	"fmt"
	"strconv"
	"strings"
)

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

type ParamMeta struct {
	Name        string
	Type        string
	Default     any
	HotReload   bool
	Risk        RiskLevel
	Description string
	Validation  string
	NetworkKey  bool
}

type Profile string

const (
	ProfileExample Profile = "example"
	ProfileDev     Profile = "dev"
	ProfileTest    Profile = "test"
	ProfileProd    Profile = "prod"
)

func Catalog() []ParamMeta {
	return []ParamMeta{
		{Name: "storage.temp_dir", Type: "string", Default: "./data/temp", HotReload: false, Risk: RiskLow, Description: "文件分片临时目录。"},
		{Name: "storage.final_dir", Type: "string", Default: "./data/final", HotReload: false, Risk: RiskLow, Description: "文件组装完成目录。"},
		{Name: "storage.audit_dir", Type: "string", Default: "./data/audit", HotReload: false, Risk: RiskLow, Description: "审计日志落盘目录。"},
		{Name: "storage.log_dir", Type: "string", Default: "./data/logs", HotReload: false, Risk: RiskLow, Description: "运行日志目录。"},
		{Name: "network.sip.enabled", Type: "bool", Default: true, HotReload: true, Risk: RiskMedium, Description: "启用 SIP 控制面。", NetworkKey: true},
		{Name: "network.sip.listen_ip", Type: "string", Default: "0.0.0.0", HotReload: false, Risk: RiskHigh, Description: "SIP 监听 IP。", NetworkKey: true},
		{Name: "network.sip.listen_port", Type: "int", Default: 5060, HotReload: false, Risk: RiskHigh, Description: "SIP 监听端口。", NetworkKey: true},
		{Name: "network.sip.transport", Type: "string", Default: "TCP", HotReload: false, Risk: RiskHigh, Description: "SIP 传输层协议（TCP/UDP/TLS）。", NetworkKey: true},
		{Name: "network.sip.advertise_ip", Type: "string", Default: "", HotReload: false, Risk: RiskMedium, Description: "SIP 对端可见地址。", NetworkKey: true},
		{Name: "network.sip.domain", Type: "string", Default: "", HotReload: true, Risk: RiskLow, Description: "SIP 域名。"},
		{Name: "network.sip.max_message_bytes", Type: "int", Default: 45535, HotReload: true, Risk: RiskHigh, Description: "SIP 最大报文大小（UDP 超 1300 存在分片风险）。", NetworkKey: true},
		{Name: "network.sip.read_timeout_ms", Type: "int", Default: 5000, HotReload: true, Risk: RiskMedium, Description: "SIP 读超时（毫秒）。", NetworkKey: true},
		{Name: "network.sip.write_timeout_ms", Type: "int", Default: 5000, HotReload: true, Risk: RiskMedium, Description: "SIP 写超时（毫秒）。", NetworkKey: true},
		{Name: "network.sip.idle_timeout_ms", Type: "int", Default: 40000, HotReload: true, Risk: RiskLow, Description: "SIP 空闲连接超时（毫秒）。", NetworkKey: true},
		{Name: "network.sip.tcp_keepalive_enabled", Type: "bool", Default: true, HotReload: true, Risk: RiskLow, Description: "启用 SIP TCP keepalive。", NetworkKey: true},
		{Name: "network.sip.tcp_keepalive_interval_ms", Type: "int", Default: 30000, HotReload: true, Risk: RiskLow, Description: "SIP TCP keepalive 间隔（毫秒）。", NetworkKey: true},
		{Name: "network.sip.tcp_read_buffer_bytes", Type: "int", Default: 45536, HotReload: true, Risk: RiskMedium, Description: "SIP TCP 连接读缓冲区大小。", NetworkKey: true},
		{Name: "network.sip.tcp_write_buffer_bytes", Type: "int", Default: 45536, HotReload: true, Risk: RiskMedium, Description: "SIP TCP 连接写缓冲区大小。", NetworkKey: true},
		{Name: "network.sip.max_connections", Type: "int", Default: 1048, HotReload: true, Risk: RiskHigh, Description: "SIP TCP 最大并发连接数。", NetworkKey: true},
		{Name: "network.rtp.enabled", Type: "bool", Default: true, HotReload: true, Risk: RiskMedium, Description: "启用 RTP 文件面。", NetworkKey: true},
		{Name: "network.rtp.listen_ip", Type: "string", Default: "0.0.0.0", HotReload: false, Risk: RiskHigh, Description: "RTP 监听 IP。", NetworkKey: true},
		{Name: "network.rtp.advertise_ip", Type: "string", Default: "", HotReload: false, Risk: RiskMedium, Description: "RTP 对端可见地址。", NetworkKey: true},
		{Name: "network.rtp.port_start", Type: "int", Default: 10000, HotReload: false, Risk: RiskHigh, Description: "RTP 端口池起始端口。", NetworkKey: true},
		{Name: "network.rtp.port_end", Type: "int", Default: 10999, HotReload: false, Risk: RiskHigh, Description: "RTP 端口池结束端口。", NetworkKey: true},
		{Name: "network.rtp.transport", Type: "string", Default: "UDP", HotReload: false, Risk: RiskHigh, Description: "RTP 传输协议（UDP 生产默认，TCP 可联调验证）。", NetworkKey: true},
		{Name: "network.rtp.max_packet_bytes", Type: "int", Default: 1400, HotReload: true, Risk: RiskHigh, Description: "RTP 单包大小。", NetworkKey: true},
		{Name: "network.rtp.max_inflight_transfers", Type: "int", Default: 44, HotReload: true, Risk: RiskMedium, Description: "并发传输上限。"},
		{Name: "network.rtp.receive_buffer_bytes", Type: "int", Default: 4194304, HotReload: true, Risk: RiskMedium, Description: "RTP 接收缓冲区大小。", NetworkKey: true},
		{Name: "network.rtp.transfer_timeout_ms", Type: "int", Default: 30000, HotReload: true, Risk: RiskMedium, Description: "文件传输超时（毫秒）。", NetworkKey: true},
		{Name: "network.rtp.retransmit_max_rounds", Type: "int", Default: 3, HotReload: true, Risk: RiskLow, Description: "重传最大轮次。"},
		{Name: "network.rtp.tcp_read_timeout_ms", Type: "int", Default: 5000, HotReload: true, Risk: RiskMedium, Description: "RTP TCP 读超时（毫秒）。", NetworkKey: true},
		{Name: "network.rtp.tcp_write_timeout_ms", Type: "int", Default: 5000, HotReload: true, Risk: RiskMedium, Description: "RTP TCP 写超时（毫秒）。", NetworkKey: true},
		{Name: "network.rtp.tcp_keepalive_enabled", Type: "bool", Default: true, HotReload: true, Risk: RiskLow, Description: "启用 RTP TCP keepalive。", NetworkKey: true},
		{Name: "network.rtp.max_tcp_sessions", Type: "int", Default: 128, HotReload: true, Risk: RiskHigh, Description: "RTP TCP 最大并发会话数。", NetworkKey: true},
		{Name: "media.port_range.start", Type: "int", Default: 10000, HotReload: false, Risk: RiskMedium, Description: "部署规划媒体端口起始值。", NetworkKey: true},
		{Name: "media.port_range.end", Type: "int", Default: 10999, HotReload: false, Risk: RiskMedium, Description: "部署规划媒体端口结束值。", NetworkKey: true},
		{Name: "transport_tuning.mode", Type: "string", Default: "secure_boundary", HotReload: true, Risk: RiskMedium, Description: "性能优先、弱网兜底的传输策略档位。", Validation: "可选值：secure_boundary / boundary / strict_boundary"},
		{Name: "transport_tuning.udp_control_max_bytes", Type: "int", Default: 1300, HotReload: true, Risk: RiskHigh, Description: "UDP 控制面单报文预算。", Validation: "推荐 1300；范围 [1024,1400]；低于 1249 会重现现场 oversize"},
		{Name: "transport_tuning.udp_catalog_max_bytes", Type: "int", Default: 1300, HotReload: true, Risk: RiskHigh, Description: "目录/分页查询控制面预算。", Validation: "推荐 1300；范围 [1024,1400]"},
		{Name: "transport_tuning.inline_response_udp_budget_bytes", Type: "int", Default: 1200, HotReload: true, Risk: RiskHigh, Description: "INLINE 响应可用 UDP 预算。", Validation: "推荐 1200；范围 [1024,1400]"},
		{Name: "transport_tuning.inline_response_safety_reserve_bytes", Type: "int", Default: 120, HotReload: true, Risk: RiskMedium, Description: "INLINE 安全预留字节。", Validation: "推荐 220；范围 [128,512]"},
		{Name: "transport_tuning.inline_response_envelope_overhead_bytes", Type: "int", Default: 320, HotReload: true, Risk: RiskMedium, Description: "INLINE XML/封套开销估算。", Validation: "推荐 320；范围 [128,768]"},
		{Name: "transport_tuning.inline_response_headroom_ratio", Type: "float64", Default: 0.15, HotReload: true, Risk: RiskMedium, Description: "INLINE 预算头寸比例。", Validation: "推荐 0.15；范围 [0.05,0.30]"},
		{Name: "transport_tuning.inline_response_headroom_percent", Type: "int", Default: 15, HotReload: true, Risk: RiskLow, Description: "兼容旧配置的 headroom 百分比。", Validation: "兼容字段；推荐与 ratio 保持一致"},
		{Name: "transport_tuning.udp_request_parallelism_per_device", Type: "int", Default: 4, HotReload: true, Risk: RiskHigh, Description: "单设备普通请求并发。", Validation: "推荐 4；范围 [2,16]"},
		{Name: "transport_tuning.udp_callback_parallelism_per_peer", Type: "int", Default: 4, HotReload: true, Risk: RiskHigh, Description: "单对端 callback 并发。", Validation: "推荐 4；范围 [2,16]"},
		{Name: "transport_tuning.udp_bulk_parallelism_per_device", Type: "int", Default: 4, HotReload: true, Risk: RiskHigh, Description: "单设备 bulk_open 并发。", Validation: "推荐 4；范围 [2,16]"},
		{Name: "transport_tuning.udp_segment_parallelism_per_device", Type: "int", Default: 8, HotReload: true, Risk: RiskHigh, Description: "单设备 segment-child 总并发。", Validation: "推荐 8；范围 [2,16]；回环/同源同看场景建议 4~8"},
		{Name: "transport_tuning.udp_small_request_max_wait_ms", Type: "int", Default: 1500, HotReload: true, Risk: RiskMedium, Description: "small lane 最大等待时间。", Validation: "推荐 1500；范围 [500,5000]"},
		{Name: "transport_tuning.adaptive_playback_hot_window_bytes", Type: "int64", Default: 8388608, HotReload: true, Risk: RiskMedium, Description: "热点播放窗口大小，用内存换更少控制面往返。", Validation: "推荐 8MiB；范围 [4MiB,16MiB]"},
		{Name: "transport_tuning.adaptive_playback_segment_cache_bytes", Type: "int64", Default: 536870912, HotReload: true, Risk: RiskMedium, Description: "热点段缓存总容量。", Validation: "推荐 512MiB；范围 [128MiB,1GiB]"},
		{Name: "transport_tuning.adaptive_playback_segment_cache_ttl_ms", Type: "int", Default: 45000, HotReload: true, Risk: RiskLow, Description: "热点段缓存 TTL。", Validation: "推荐 45000；范围 [10000,300000]"},
		{Name: "transport_tuning.adaptive_playback_prefetch_segments", Type: "int", Default: 1, HotReload: true, Risk: RiskMedium, Description: "热点播放预取段数。", Validation: "推荐 1；范围 [0,2]；稳定态默认不预取，仅热点命中后再开"},
		{Name: "transport_tuning.adaptive_primary_segment_after_failures", Type: "int", Default: 1, HotReload: true, Risk: RiskMedium, Description: "连续流失败多少次后切到 segmented_primary。", Validation: "推荐 2；范围 [1,4]"},
		{Name: "transport_tuning.adaptive_loopback_playback_segment_concurrency", Type: "int", Default: 1, HotReload: true, Risk: RiskMedium, Description: "本地回环源场景下的稳定态 segment 并发。", Validation: "推荐 2；范围 [1,4]；过低会让播放体感发钝，过高会重新放大回环带宽"},
		{Name: "transport_tuning.adaptive_open_ended_range_initial_window_bytes", Type: "int64", Default: 8388608, HotReload: true, Risk: RiskMedium, Description: "recent probe abort 后，将 bytes=N- 改写成首个有界窗口的大小。", Validation: "推荐 8388608(8MiB)；范围 [1048576,33554432]；用于避免先拉全量再切段"},
		{Name: "transport_tuning.boundary_rtp_payload_bytes", Type: "int", Default: 1200, HotReload: true, Risk: RiskHigh, Description: "边界 RTP payload 大小。", Validation: "推荐 1200；范围 [960,1200]"},
		{Name: "transport_tuning.generic_segmented_primary_threshold_bytes", Type: "int64", Default: 8388608, HotReload: true, Risk: RiskMedium, Description: "非音视频大文件进入 segmented-primary 的阈值。", Validation: "推荐 8388608(8MiB)；范围 [8388608,536870912]；更早进入 segmented-primary，避免慢客户端下载长期卡在单流"},
		{Name: "transport_tuning.generic_prefetch_segments", Type: "int", Default: 0, HotReload: true, Risk: RiskLow, Description: "非音视频大文件 segmented-primary 的预取段数。", Validation: "推荐 0；范围 [0,2]；默认关闭预取，避免下载侧额外放大回源并发"},
		{Name: "transport_tuning.generic_download_window_bytes", Type: "int64", Default: 2097152, HotReload: true, Risk: RiskMedium, Description: "非音视频大文件默认分段窗口。", Validation: "推荐 2097152(2MiB)；范围 [1048576,67108864]；用于有界 range / fallback 场景"},
		{Name: "transport_tuning.generic_download_open_ended_window_bytes", Type: "int64", Default: 8388608, HotReload: true, Risk: RiskMedium, Description: "非音视频大文件开区间 Range 的 bulk 窗口。", Validation: "推荐 8388608(8MiB)；范围 [2097152,67108864]；优先减少 2MiB 小段事务开销"},
		{Name: "transport_tuning.generic_download_segment_concurrency", Type: "int", Default: 1, HotReload: true, Risk: RiskMedium, Description: "单个非音视频下载在 fixed-window 兜底阶段的默认 segment 并发；stream-first 主路径优先连续流，异常再切单段恢复。", Validation: "推荐 1；范围 [1,8]；默认作为 stream-first 的恢复窗口并发，不建议在未验证前抬高"},
		{Name: "transport_tuning.generic_download_same_transfer_split_enabled", Type: "bool", Default: false, HotReload: true, Risk: RiskMedium, Description: "是否把同一外层下载事务内的多个 segment 再继续平分带宽。", Validation: "推荐 false；bulk download 默认不再做段内平分，保留多线程下载提速意义"},
		{Name: "transport_tuning.generic_download_source_constrained_auto_singleflight_enabled", Type: "bool", Default: false, HotReload: true, Risk: RiskMedium, Description: "是否根据 sender 侧观测速率自动退回 singleflight。", Validation: "推荐 false；避免把小窗口/会话开销误判成源站受限"},
		{Name: "transport_tuning.generic_download_segment_retries", Type: "int", Default: 1, HotReload: true, Risk: RiskLow, Description: "单个下载 segment 重试次数。", Validation: "推荐 2；范围 [0,6]"},
		{Name: "transport_tuning.generic_download_resume_max_attempts", Type: "int", Default: 4, HotReload: true, Risk: RiskLow, Description: "单个非音视频下载事务的总恢复次数。", Validation: "推荐 6；范围 [1,12]"},
		{Name: "transport_tuning.generic_download_resume_per_range_retries", Type: "int", Default: 3, HotReload: true, Risk: RiskLow, Description: "同一下载窗口内的恢复重试次数。", Validation: "推荐 3；范围 [1,6]"},
		{Name: "transport_tuning.generic_download_penalty_wait_ms", Type: "int", Default: 500, HotReload: true, Risk: RiskLow, Description: "下载类 rtp_sequence_gap 触发后的附加等待。", Validation: "推荐 500；范围 [0,5000]；默认显著小于设备级 10s 罚等"},
		{Name: "transport_tuning.generic_download_total_bitrate_bps", Type: "int", Default: 33554432, HotReload: true, Risk: RiskMedium, Description: "所有非音视频外层下载事务共享的总发送预算。", Validation: "推荐 33554432；范围 [4194304,134217728]；按外层下载事务而不是按 segment child 公平分享带宽"},
		{Name: "transport_tuning.generic_download_min_per_transfer_bitrate_bps", Type: "int", Default: 1097152, HotReload: true, Risk: RiskMedium, Description: "单个外层下载事务在公平整形下的最低发送速率；只有总预算足以覆盖所有活跃下载时才会启用。", Validation: "推荐 2097152；范围 [1048576,33554432]；不会再按 segment child 逐个叠加打穿总带宽"},
		{Name: "transport_tuning.generic_download_circuit_failure_threshold", Type: "int", Default: 3, HotReload: true, Risk: RiskMedium, Description: "下载类连续失败达到多少次后进入熔断降级。", Validation: "推荐 3；范围 [1,10]"},
		{Name: "transport_tuning.generic_download_circuit_open_ms", Type: "int", Default: 30000, HotReload: true, Risk: RiskMedium, Description: "下载类熔断打开时长。", Validation: "推荐 30000；范围 [1000,300000]"},
		{Name: "transport_tuning.generic_download_rtp_bitrate_bps", Type: "int", Default: 8388608, HotReload: true, Risk: RiskMedium, Description: "下载专用 RTP 整形速率上限。", Validation: "推荐 8388608；范围 [2097152,67108864]；现场日志显示实际稳态远低于 16Mbps，默认改得更保守"},
		{Name: "transport_tuning.generic_download_rtp_min_spacing_us", Type: "int", Default: 450, HotReload: true, Risk: RiskMedium, Description: "下载专用 RTP 包间最小间隔。", Validation: "推荐 650；范围 [100,10000]；适当拉大 spacing，减少大文件下载的瞬时 burst"},
		{Name: "transport_tuning.generic_download_rtp_socket_buffer_bytes", Type: "int", Default: 33554432, HotReload: true, Risk: RiskLow, Description: "下载专用 RTP 套接字缓冲。", Validation: "推荐 33554432(32MiB)；范围 [1048576,67108864]；用内存换更强乱序吸收"},
		{Name: "transport_tuning.generic_download_rtp_reorder_window_packets", Type: "int", Default: 512, HotReload: true, Risk: RiskMedium, Description: "下载专用 RTP 乱序窗口。", Validation: "推荐 512；范围 [16,1024]"},
		{Name: "transport_tuning.generic_download_rtp_loss_tolerance_packets", Type: "int", Default: 192, HotReload: true, Risk: RiskMedium, Description: "下载专用 RTP 丢包容忍。", Validation: "推荐 192；范围 [8,512]"},
		{Name: "transport_tuning.generic_download_rtp_gap_timeout_ms", Type: "int", Default: 900, HotReload: true, Risk: RiskMedium, Description: "下载专用 RTP gap 等待超时。", Validation: "推荐 900；范围 [100,5000]"},
		{Name: "transport_tuning.generic_download_rtp_fec_enabled", Type: "bool", Default: true, HotReload: true, Risk: RiskMedium, Description: "下载专用 RTP 是否启用单丢包恢复型 XOR FEC。", Validation: "推荐 true；关闭时可把 generic_download_rtp_fec_group_packets 设为 0"},
		{Name: "transport_tuning.generic_download_rtp_fec_group_packets", Type: "int", Default: 8, HotReload: true, Risk: RiskMedium, Description: "下载专用 RTP FEC 分组包数。", Validation: "推荐 8；范围 {0}∪[2,32]；0 表示禁用 FEC，8 约等于 12.5% 额外包开销"},
		{Name: "transport_tuning.boundary_rtp_bitrate_bps", Type: "int", Default: 16777216, HotReload: true, Risk: RiskMedium, Description: "边界 RTP 发送整形速率。", Validation: "推荐 16777216；范围 [2097152,67108864]；从 legacy 3Mbps profile 切回 boundary-rtp 后，避免默认过猛"},
		{Name: "transport_tuning.boundary_rtp_min_spacing_us", Type: "int", Default: 150, HotReload: true, Risk: RiskMedium, Description: "边界 RTP 包间最小间隔。", Validation: "推荐 250；范围 [100,5000]；越小越激进，需配合链路质量"},
		{Name: "transport_tuning.boundary_rtp_socket_buffer_bytes", Type: "int", Default: 16777216, HotReload: true, Risk: RiskLow, Description: "边界 RTP 套接字读写缓存。", Validation: "推荐 16777216(16MiB)；范围 [1048576,67108864]；用内存换 burst 吞吐与弱网抖动容忍"},
		{Name: "transport_tuning.boundary_rtp_reorder_window_packets", Type: "int", Default: 128, HotReload: true, Risk: RiskMedium, Description: "边界 RTP 乱序窗口。", Validation: "推荐 128；范围 [16,512]"},
		{Name: "transport_tuning.boundary_rtp_loss_tolerance_packets", Type: "int", Default: 48, HotReload: true, Risk: RiskMedium, Description: "边界 RTP 丢包容忍。", Validation: "推荐 48；范围 [0,256]"},
		{Name: "transport_tuning.boundary_rtp_gap_timeout_ms", Type: "int", Default: 300, HotReload: true, Risk: RiskMedium, Description: "边界 RTP gap 等待超时。", Validation: "推荐 300；范围 [100,30000]"},
		{Name: "transport_tuning.boundary_rtp_fec_enabled", Type: "bool", Default: true, HotReload: true, Risk: RiskMedium, Description: "边界 RTP 是否启用单丢包恢复型 XOR FEC。", Validation: "推荐 true；关闭时可把 boundary_rtp_fec_group_packets 设为 0"},
		{Name: "transport_tuning.boundary_rtp_fec_group_packets", Type: "int", Default: 8, HotReload: true, Risk: RiskMedium, Description: "边界 RTP FEC 分组包数。", Validation: "推荐 8；范围 {0}∪[2,32]；0 表示禁用 FEC"},
		{Name: "transport_tuning.boundary_playback_rtp_reorder_window_packets", Type: "int", Default: 192, HotReload: true, Risk: RiskMedium, Description: "播放场景 RTP 乱序窗口。", Validation: "推荐 192；范围 [32,512]"},
		{Name: "transport_tuning.boundary_playback_rtp_loss_tolerance_packets", Type: "int", Default: 64, HotReload: true, Risk: RiskMedium, Description: "播放场景 RTP 丢包容忍。", Validation: "推荐 64；范围 [8,256]"},
		{Name: "transport_tuning.boundary_playback_rtp_gap_timeout_ms", Type: "int", Default: 450, HotReload: true, Risk: RiskMedium, Description: "播放场景 RTP gap 等待超时。", Validation: "推荐 450；范围 [100,30000]"},
		{Name: "transport_tuning.boundary_playback_rtp_fec_enabled", Type: "bool", Default: true, HotReload: true, Risk: RiskMedium, Description: "播放场景 RTP 是否启用单丢包恢复型 XOR FEC。", Validation: "推荐 true；关闭时可把 boundary_playback_rtp_fec_group_packets 设为 0"},
		{Name: "transport_tuning.boundary_playback_rtp_fec_group_packets", Type: "int", Default: 8, HotReload: true, Risk: RiskMedium, Description: "播放场景 RTP FEC 分组包数。", Validation: "推荐 8；范围 {0}∪[2,32]；0 表示禁用 FEC"},
		{Name: "transport_tuning.boundary_fixed_window_bytes", Type: "int64", Default: 4194304, HotReload: true, Risk: RiskMedium, Description: "边界 fallback 分段窗口大小。", Validation: "推荐 4MiB；范围 [1MiB,8MiB]"},
		{Name: "transport_tuning.boundary_fixed_window_threshold_bytes", Type: "int64", Default: 168435456, HotReload: true, Risk: RiskMedium, Description: "边界大文件进入 fallback 参考阈值。", Validation: "推荐 256MiB；范围 [64MiB,1GiB]"},
		{Name: "transport_tuning.boundary_segment_concurrency", Type: "int", Default: 4, HotReload: true, Risk: RiskHigh, Description: "边界 fallback 分段并发。", Validation: "推荐 4；范围 [1,16]"},
		{Name: "transport_tuning.boundary_segment_retries", Type: "int", Default: 1, HotReload: true, Risk: RiskLow, Description: "边界 fallback 分段重试次数。", Validation: "推荐 1；范围 [0,2]"},
		{Name: "transport_tuning.boundary_resume_max_attempts", Type: "int", Default: 3, HotReload: true, Risk: RiskMedium, Description: "边界流主路径最大 resume 次数。", Validation: "推荐 3；范围 [1,4]"},
		{Name: "transport_tuning.boundary_resume_per_range_retries", Type: "int", Default: 1, HotReload: true, Risk: RiskLow, Description: "单窗口 range 重试次数。", Validation: "推荐 1；范围 [0,2]"},
		{Name: "transport_tuning.boundary_response_start_wait_ms", Type: "int", Default: 12000, HotReload: true, Risk: RiskMedium, Description: "边界主请求 response_start 等待。", Validation: "推荐 12000；范围 [8000,30000]"},
		{Name: "transport_tuning.boundary_range_response_start_wait_ms", Type: "int", Default: 45000, HotReload: true, Risk: RiskMedium, Description: "边界 range/segment response_start 等待。", Validation: "推荐 45000；范围 [15000,60000]"},
		{Name: "transport_tuning.boundary_http_window_bytes", Type: "int64", Default: 4194304, HotReload: true, Risk: RiskMedium, Description: "边界 HTTP fallback 窗口大小。", Validation: "推荐 4MiB；范围 [1MiB,8MiB]"},
		{Name: "transport_tuning.boundary_http_window_threshold_bytes", Type: "int64", Default: 168435456, HotReload: true, Risk: RiskMedium, Description: "边界 HTTP fallback 参考阈值。", Validation: "推荐 256MiB；范围 [64MiB,1GiB]"},
		{Name: "transport_tuning.boundary_http_segment_concurrency", Type: "int", Default: 4, HotReload: true, Risk: RiskHigh, Description: "边界 HTTP fallback 并发。", Validation: "推荐 4；范围 [1,16]"},
		{Name: "transport_tuning.boundary_http_segment_retries", Type: "int", Default: 1, HotReload: true, Risk: RiskLow, Description: "边界 HTTP fallback 重试次数。", Validation: "推荐 1；范围 [0,2]"},
		{Name: "transport_tuning.standard_window_bytes", Type: "int64", Default: 16777216, HotReload: true, Risk: RiskMedium, Description: "普通网络 fallback 窗口大小。", Validation: "推荐 16MiB；范围 [4MiB,32MiB]"},
		{Name: "transport_tuning.standard_window_threshold_bytes", Type: "int64", Default: 168435456, HotReload: true, Risk: RiskMedium, Description: "普通网络 fallback 参考阈值。", Validation: "推荐 256MiB；范围 [64MiB,1GiB]"},
		{Name: "transport_tuning.standard_segment_concurrency", Type: "int", Default: 4, HotReload: true, Risk: RiskHigh, Description: "普通网络 fallback 分段并发。", Validation: "推荐 4；范围 [1,16]"},
		{Name: "transport_tuning.standard_segment_retries", Type: "int", Default: 1, HotReload: true, Risk: RiskLow, Description: "普通网络 fallback 重试次数。", Validation: "推荐 1；范围 [0,2]"},
		{Name: "node.role", Type: "string", Default: "receiver", HotReload: false, Risk: RiskMedium, Description: "节点角色（receiver/sender）。"},
	}
}

func profileValues(profile Profile) map[string]any {
	values := map[string]any{}
	for _, item := range Catalog() {
		values[item.Name] = item.Default
	}
	switch profile {
	case ProfileDev:
		values["network.sip.listen_ip"] = "127.0.0.1"
		values["network.rtp.listen_ip"] = "127.0.0.1"
		values["network.sip.max_message_bytes"] = 4096
		values["network.rtp.max_inflight_transfers"] = 8
	case ProfileTest:
		values["network.sip.listen_ip"] = "127.0.0.1"
		values["network.rtp.listen_ip"] = "127.0.0.1"
		values["network.sip.listen_port"] = 15060
		values["network.rtp.port_start"] = 21000
		values["network.rtp.port_end"] = 21020
		values["network.rtp.max_inflight_transfers"] = 16
	case ProfileProd:
		values["network.sip.advertise_ip"] = "10.20.30.10"
		values["network.rtp.advertise_ip"] = "10.20.30.10"
		values["network.sip.domain"] = "prod.siptunnel.local"
		values["network.rtp.max_inflight_transfers"] = 128
		values["network.rtp.receive_buffer_bytes"] = 8388608
		values["network.rtp.transfer_timeout_ms"] = 45000
	}
	return values
}

func formatDefault(v any) string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return "\"\""
		}
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func markRisk(meta ParamMeta) string {
	if meta.NetworkKey && meta.Risk == RiskHigh {
		return "⚠️ HIGH-NET"
	}
	return strings.ToUpper(string(meta.Risk))
}
