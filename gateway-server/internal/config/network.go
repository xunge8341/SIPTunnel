package config

type NetworkConfig struct {
	Mode            NetworkMode           `yaml:"mode"`
	SIP             SIPConfig             `yaml:"sip"`
	RTP             RTPConfig             `yaml:"rtp"`
	TransportTuning TransportTuningConfig `yaml:"transport_tuning"`
}

type TransportTuningConfig struct {
	Mode                                string `yaml:"mode"`
	UDPControlMaxBytes                  int    `yaml:"udp_control_max_bytes"`
	UDPCatalogMaxBytes                  int    `yaml:"udp_catalog_max_bytes"`
	InlineResponseUDPBudgetBytes        int    `yaml:"inline_response_udp_budget_bytes"`
	InlineResponseSafetyReserveBytes    int    `yaml:"inline_response_safety_reserve_bytes"`
	InlineResponseEnvelopeOverheadBytes int    `yaml:"inline_response_envelope_overhead_bytes"`
	// InlineResponseHeadroomRatio 是新的生效配置，直接表达需要保留的预算比例。
	// InlineResponseHeadroomPercent 保留给旧配置模板兼容，解析时会自动相互回填。
	InlineResponseHeadroomRatio    float64 `yaml:"inline_response_headroom_ratio"`
	InlineResponseHeadroomPercent  int     `yaml:"inline_response_headroom_percent"`
	UDPRequestParallelismPerDevice int     `yaml:"udp_request_parallelism_per_device"`
	UDPCallbackParallelismPerPeer  int     `yaml:"udp_callback_parallelism_per_peer"`
	UDPBulkParallelismPerDevice    int     `yaml:"udp_bulk_parallelism_per_device"`
	// UDPSmallRequestMaxWaitMS 用于 small lane 的独立排队等待上限，避免控制类小包被 bulk 请求拖死。
	UDPSmallRequestMaxWaitMS int `yaml:"udp_small_request_max_wait_ms"`
	// UDPSegmentParallelismPerDevice 约束单设备上 segment-child 控制请求的总并发，避免多个观看者互相串死。
	UDPSegmentParallelismPerDevice int `yaml:"udp_segment_parallelism_per_device"`
	// AdaptivePlaybackHotWindowBytes 允许在差网络下使用更大的窗口，用内存换更少的控制面往返。
	AdaptivePlaybackHotWindowBytes      int64 `yaml:"adaptive_playback_hot_window_bytes"`
	AdaptivePlaybackSegmentCacheBytes   int64 `yaml:"adaptive_playback_segment_cache_bytes"`
	AdaptivePlaybackSegmentCacheTTLMS   int   `yaml:"adaptive_playback_segment_cache_ttl_ms"`
	AdaptivePlaybackPrefetchSegments    int   `yaml:"adaptive_playback_prefetch_segments"`
	AdaptivePrimarySegmentAfterFailures int   `yaml:"adaptive_primary_segment_after_failures"`
	// AdaptiveLoopbackPlaybackSegmentConcurrency 控制本地回环源场景下的稳定态 segment 并发，避免过度保守导致播放体感发钝。
	AdaptiveLoopbackPlaybackSegmentConcurrency int `yaml:"adaptive_loopback_playback_segment_concurrency"`
	// AdaptiveOpenEndedRangeInitialWindowBytes 用于把 recent-probe-abort 后的 bytes=N- 改写成首个有界窗口，避免先拉全量再立刻切段。
	AdaptiveOpenEndedRangeInitialWindowBytes int64 `yaml:"adaptive_open_ended_range_initial_window_bytes"`
	// GenericSegmentedPrimaryThresholdBytes 控制非音视频大文件何时直接走 segmented-primary，避免慢客户端下载把上游整流拖到固定 300KiB/s 左右。
	GenericSegmentedPrimaryThresholdBytes int64 `yaml:"generic_segmented_primary_threshold_bytes"`
	// GenericDownloadWindowBytes 控制非音视频大文件默认分段窗口；用于有界 range / fallback 段调度。
	GenericDownloadWindowBytes int64 `yaml:"generic_download_window_bytes"`
	// GenericDownloadOpenEndedWindowBytes 控制 bytes=N- 这类开区间大文件下载的 bulk 窗口；
	// 默认显著大于普通窗口，优先减少 2MiB 级别的小段事务开销。
	GenericDownloadOpenEndedWindowBytes int64 `yaml:"generic_download_open_ended_window_bytes"`
	// GenericPrefetchSegments 控制非音视频大文件的预取段数；仅在 segmented-primary 下生效。
	GenericPrefetchSegments int `yaml:"generic_prefetch_segments"`
	// GenericDownloadSegmentConcurrency 控制单个非音视频下载默认 segment 并发，避免一个下载把所有带宽和 RTP 端口预算吃满。
	GenericDownloadSegmentConcurrency int `yaml:"generic_download_segment_concurrency"`
	// GenericDownloadSameTransferSplitEnabled 控制是否把同一外层下载事务内的多个 segment 再继续平分带宽；
	// bulk download 默认关闭该平分，保留多线程下载的提速意义。
	GenericDownloadSameTransferSplitEnabled bool `yaml:"generic_download_same_transfer_split_enabled"`
	// GenericDownloadSourceConstrainedAutoSingleflightEnabled 控制是否根据 sender 侧观测速率自动把 bulk download 降成 singleflight；
	// 默认关闭，避免把“2MiB 小窗/会话开销导致的低速”误判成源站本身受限。
	GenericDownloadSourceConstrainedAutoSingleflightEnabled bool `yaml:"generic_download_source_constrained_auto_singleflight_enabled"`
	// GenericDownloadSegmentRetries / Resume* 控制泛型下载的容错预算；下载场景允许比播放更积极地重试。
	GenericDownloadSegmentRetries        int `yaml:"generic_download_segment_retries"`
	GenericDownloadResumeMaxAttempts     int `yaml:"generic_download_resume_max_attempts"`
	GenericDownloadResumePerRangeRetries int `yaml:"generic_download_resume_per_range_retries"`
	// GenericDownloadPenaltyWaitMS 是下载类连续 gap 后的额外等待；默认显著小于播放/设备级罚等，避免整个下载直接“卡死”。
	GenericDownloadPenaltyWaitMS int `yaml:"generic_download_penalty_wait_ms"`
	// GenericDownloadTotalBitrateBps / MinPerTransferBitrateBps 用于做下载带宽整形与公平分享。
	GenericDownloadTotalBitrateBps          int `yaml:"generic_download_total_bitrate_bps"`
	GenericDownloadMinPerTransferBitrateBps int `yaml:"generic_download_min_per_transfer_bitrate_bps"`
	// GenericDownloadCircuit* 控制下载侧熔断/降级阈值；达到阈值后自动降为低并发、低预取、低码率，而不是继续把失败放大。
	GenericDownloadCircuitFailureThreshold int `yaml:"generic_download_circuit_failure_threshold"`
	GenericDownloadCircuitOpenMS           int `yaml:"generic_download_circuit_open_ms"`
	// GenericDownloadRTP* 是下载专用 RTP 发送/接收参数；相对播放更强调“稳态吞吐、低 burst、差网完整性”，默认速率/spacing 也更保守。
	GenericDownloadRTPBitrateBps           int `yaml:"generic_download_rtp_bitrate_bps"`
	GenericDownloadRTPMinSpacingUS         int `yaml:"generic_download_rtp_min_spacing_us"`
	GenericDownloadRTPSocketBufferBytes    int `yaml:"generic_download_rtp_socket_buffer_bytes"`
	GenericDownloadRTPReorderWindowPackets int `yaml:"generic_download_rtp_reorder_window_packets"`
	GenericDownloadRTPLossTolerancePackets int `yaml:"generic_download_rtp_loss_tolerance_packets"`
	GenericDownloadRTPGapTimeoutMS         int `yaml:"generic_download_rtp_gap_timeout_ms"`
	// GenericDownloadRTPFEC* 启用单丢包恢复型 XOR FEC；用额外包开销换更强的差网完整性。
	GenericDownloadRTPFECEnabled      bool `yaml:"generic_download_rtp_fec_enabled"`
	GenericDownloadRTPFECGroupPackets int  `yaml:"generic_download_rtp_fec_group_packets"`
	BoundaryRTPPayloadBytes           int  `yaml:"boundary_rtp_payload_bytes"`
	BoundaryRTPBitrateBps             int  `yaml:"boundary_rtp_bitrate_bps"`
	BoundaryRTPMinSpacingUS           int  `yaml:"boundary_rtp_min_spacing_us"`
	// BoundaryRTPSocketBufferBytes 允许用内存换网络抖动容忍和 burst 吞吐。
	BoundaryRTPSocketBufferBytes            int   `yaml:"boundary_rtp_socket_buffer_bytes"`
	BoundaryRTPReorderWindowPackets         int   `yaml:"boundary_rtp_reorder_window_packets"`
	BoundaryRTPLossTolerancePackets         int   `yaml:"boundary_rtp_loss_tolerance_packets"`
	BoundaryRTPGapTimeoutMS                 int   `yaml:"boundary_rtp_gap_timeout_ms"`
	BoundaryRTPFECEnabled                   bool  `yaml:"boundary_rtp_fec_enabled"`
	BoundaryRTPFECGroupPackets              int   `yaml:"boundary_rtp_fec_group_packets"`
	BoundaryPlaybackRTPReorderWindowPackets int   `yaml:"boundary_playback_rtp_reorder_window_packets"`
	BoundaryPlaybackRTPLossTolerancePackets int   `yaml:"boundary_playback_rtp_loss_tolerance_packets"`
	BoundaryPlaybackRTPGapTimeoutMS         int   `yaml:"boundary_playback_rtp_gap_timeout_ms"`
	BoundaryPlaybackRTPFECEnabled           bool  `yaml:"boundary_playback_rtp_fec_enabled"`
	BoundaryPlaybackRTPFECGroupPackets      int   `yaml:"boundary_playback_rtp_fec_group_packets"`
	BoundaryFixedWindowBytes                int64 `yaml:"boundary_fixed_window_bytes"`
	BoundaryFixedWindowThreshold            int64 `yaml:"boundary_fixed_window_threshold_bytes"`
	BoundarySegmentConcurrency              int   `yaml:"boundary_segment_concurrency"`
	BoundarySegmentRetries                  int   `yaml:"boundary_segment_retries"`
	BoundaryResumeMaxAttempts               int   `yaml:"boundary_resume_max_attempts"`
	BoundaryResumePerRangeRetries           int   `yaml:"boundary_resume_per_range_retries"`
	BoundaryResponseStartWaitMS             int   `yaml:"boundary_response_start_wait_ms"`
	BoundaryRangeResponseWaitMS             int   `yaml:"boundary_range_response_start_wait_ms"`
	BoundaryHTTPWindowBytes                 int64 `yaml:"boundary_http_window_bytes"`
	BoundaryHTTPWindowThreshold             int64 `yaml:"boundary_http_window_threshold_bytes"`
	BoundaryHTTPSegmentConcurrency          int   `yaml:"boundary_http_segment_concurrency"`
	BoundaryHTTPSegmentRetries              int   `yaml:"boundary_http_segment_retries"`
	StandardWindowBytes                     int64 `yaml:"standard_window_bytes"`
	StandardWindowThreshold                 int64 `yaml:"standard_window_threshold_bytes"`
	StandardSegmentConcurrency              int   `yaml:"standard_segment_concurrency"`
	StandardSegmentRetries                  int   `yaml:"standard_segment_retries"`
}

type SIPConfig struct {
	Enabled                bool   `yaml:"enabled"`
	ListenIP               string `yaml:"listen_ip"`
	ListenPort             int    `yaml:"listen_port"`
	Transport              string `yaml:"transport"`
	AdvertiseIP            string `yaml:"advertise_ip"`
	Domain                 string `yaml:"domain"`
	MaxMessageBytes        int    `yaml:"max_message_bytes"`
	ReadTimeoutMS          int    `yaml:"read_timeout_ms"`
	WriteTimeoutMS         int    `yaml:"write_timeout_ms"`
	IdleTimeoutMS          int    `yaml:"idle_timeout_ms"`
	TCPKeepAliveEnabled    bool   `yaml:"tcp_keepalive_enabled"`
	TCPKeepAliveIntervalMS int    `yaml:"tcp_keepalive_interval_ms"`
	TCPReadBufferBytes     int    `yaml:"tcp_read_buffer_bytes"`
	TCPWriteBufferBytes    int    `yaml:"tcp_write_buffer_bytes"`
	MaxConnections         int    `yaml:"max_connections"`
}

const SIPUDPRecommendedMaxMessageBytes = 1300

type RTPConfig struct {
	Enabled              bool   `yaml:"enabled"`
	ListenIP             string `yaml:"listen_ip"`
	AdvertiseIP          string `yaml:"advertise_ip"`
	PortStart            int    `yaml:"port_start"`
	PortEnd              int    `yaml:"port_end"`
	Transport            string `yaml:"transport"`
	MaxPacketBytes       int    `yaml:"max_packet_bytes"`
	MaxInflightTransfers int    `yaml:"max_inflight_transfers"`
	ReceiveBufferBytes   int    `yaml:"receive_buffer_bytes"`
	TransferTimeoutMS    int    `yaml:"transfer_timeout_ms"`
	RetransmitMaxRounds  int    `yaml:"retransmit_max_rounds"`
	TCPReadTimeoutMS     int    `yaml:"tcp_read_timeout_ms"`
	TCPWriteTimeoutMS    int    `yaml:"tcp_write_timeout_ms"`
	TCPKeepAliveEnabled  bool   `yaml:"tcp_keepalive_enabled"`
	MaxTCPSessions       int    `yaml:"max_tcp_sessions"`
}
