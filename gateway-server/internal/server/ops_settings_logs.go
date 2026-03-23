package server

type dashboardOpsSummary struct {
	TopMappings         []dashboardOpsSummaryItem `json:"top_mappings"`
	TopSourceIPs        []dashboardOpsSummaryItem `json:"top_source_ips"`
	TopFailedMappings   []dashboardOpsSummaryItem `json:"top_failed_mappings"`
	TopFailedSourceIPs  []dashboardOpsSummaryItem `json:"top_failed_source_ips"`
	RateLimitStatus     string                    `json:"rate_limit_status"`
	CircuitBreakerState string                    `json:"circuit_breaker_state"`
	ProtectionStatus    string                    `json:"protection_status"`
}

type dashboardTrendPoint struct {
	Bucket string `json:"bucket"`
	Label  string `json:"label"`
	Total  int    `json:"total"`
	Failed int    `json:"failed"`
	Slow   int    `json:"slow"`
}

type dashboardTrendSeries struct {
	Range       string                `json:"range"`
	Granularity string                `json:"granularity"`
	Points      []dashboardTrendPoint `json:"points"`
}

type dashboardSummary struct {
	SystemHealth        string `json:"system_health"`
	ActiveConnections   int    `json:"active_connections"`
	MappingTotal        int    `json:"mapping_total"`
	MappingErrorCount   int    `json:"mapping_error_count"`
	RecentFailureCount  int    `json:"recent_failure_count"`
	RateLimitState      string `json:"rate_limit_state"`
	CircuitBreakerState string `json:"circuit_breaker_state"`
}

type protectionState struct {
	AlertRules             []string                        `json:"alert_rules"`
	RateLimitRules         []string                        `json:"rate_limit_rules"`
	CircuitBreakerRules    []string                        `json:"circuit_breaker_rules"`
	CurrentTriggered       []string                        `json:"current_triggered"`
	LastTriggeredTime      string                          `json:"last_triggered_time"`
	LastTriggeredTarget    string                          `json:"last_triggered_target"`
	RPS                    int                             `json:"rps,omitempty"`
	Burst                  int                             `json:"burst,omitempty"`
	MaxConcurrent          int                             `json:"max_concurrent,omitempty"`
	FailureThreshold       int                             `json:"failure_threshold,omitempty"`
	RecoveryWindowSec      int                             `json:"recovery_window_sec,omitempty"`
	RateLimitStatus        string                          `json:"rate_limit_status,omitempty"`
	CircuitBreakerStatus   string                          `json:"circuit_breaker_status,omitempty"`
	ProtectionStatus       string                          `json:"protection_status,omitempty"`
	AnalysisWindow         string                          `json:"analysis_window,omitempty"`
	RecentFailureCount     int                             `json:"recent_failure_count,omitempty"`
	RecentSlowRequestCount int                             `json:"recent_slow_request_count,omitempty"`
	CurrentActiveRequests  int                             `json:"current_active_requests,omitempty"`
	RateLimitHitsTotal     uint64                          `json:"rate_limit_hits_total,omitempty"`
	ConcurrentRejectsTotal uint64                          `json:"concurrent_rejects_total,omitempty"`
	AllowedRequestsTotal   uint64                          `json:"allowed_requests_total,omitempty"`
	LastTriggeredType      string                          `json:"last_triggered_type,omitempty"`
	CircuitOpenCount       int                             `json:"circuit_open_count,omitempty"`
	CircuitHalfOpenCount   int                             `json:"circuit_half_open_count,omitempty"`
	CircuitActiveState     string                          `json:"circuit_active_state,omitempty"`
	CircuitLastOpenUntil   string                          `json:"circuit_last_open_until,omitempty"`
	CircuitLastOpenReason  string                          `json:"circuit_last_open_reason,omitempty"`
	CircuitEntries         []upstreamCircuitEntrySnapshot  `json:"circuit_entries,omitempty"`
	TopRateLimitTargets    []protectionTargetStat          `json:"top_rate_limit_targets,omitempty"`
	TopConcurrentTargets   []protectionTargetStat          `json:"top_concurrent_targets,omitempty"`
	TopAllowedTargets      []protectionTargetStat          `json:"top_allowed_targets,omitempty"`
	Scopes                 []protectionScopeSnapshot       `json:"scopes,omitempty"`
	Restrictions           []protectionRestrictionSnapshot `json:"restrictions,omitempty"`
}

type securityCenterState struct {
	LicenseStatus         string   `json:"license_status"`
	ProductTypeName       string   `json:"product_type_name"`
	ExpiryTime            string   `json:"expiry_time"`
	ActiveTime            string   `json:"active_time"`
	MaintenanceExpireTime string   `json:"maintenance_expire_time"`
	LicenseTime           string   `json:"license_time"`
	ProductType           string   `json:"product_type"`
	LicenseType           string   `json:"license_type"`
	LicenseCounter        string   `json:"license_counter"`
	MachineCode           string   `json:"machine_code"`
	ProjectCode           string   `json:"project_code"`
	LicensedFeatures      []string `json:"licensed_features"`
	LastValidation        string   `json:"last_validation"`
	ManagementSecurity    string   `json:"management_security"`
	SigningAlgorithm      string   `json:"signing_algorithm"`
	AdminTokenConfigured  bool     `json:"admin_token_configured"`
	AdminMFARequired      bool     `json:"admin_mfa_required"`
	AdminMFAConfigured    bool     `json:"admin_mfa_configured"`
	ConfigEncryption      bool     `json:"config_encryption"`
	SignerExternalized    bool     `json:"signer_externalized"`
	AdminTokenFingerprint string   `json:"admin_token_fingerprint,omitempty"`
}

type nodeTunnelWorkspace struct {
	LocalNode          NodeConfigEndpoint  `json:"local_node"`
	PeerNode           NodeConfigEndpoint  `json:"peer_node"`
	NetworkMode        string              `json:"network_mode"`
	CapabilityMatrix   []map[string]any    `json:"capability_matrix"`
	SIPCapability      map[string]any      `json:"sip_capability"`
	RTPCapability      map[string]any      `json:"rtp_capability"`
	SessionSettings    TunnelConfigPayload `json:"session_settings"`
	SecuritySettings   map[string]any      `json:"security_settings"`
	EncryptionSettings map[string]string   `json:"encryption_settings"`
}

type dashboardOpsSummaryItem struct {
	Name         string `json:"name"`
	Count        int    `json:"count"`
	AvgLatencyMS int64  `json:"avg_latency_ms,omitempty"`
}

type accessLogSummary struct {
	Total      int            `json:"total"`
	Failed     int            `json:"failed"`
	Slow       int            `json:"slow"`
	ErrorTypes map[string]int `json:"error_types"`
	Window     string         `json:"window"`
}

type protectionRestrictionSnapshot struct {
	Scope       string `json:"scope"`
	Target      string `json:"target"`
	Reason      string `json:"reason,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	Minutes     int    `json:"minutes,omitempty"`
	Active      bool   `json:"active"`
	Auto        bool   `json:"auto,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
	AutoRelease bool   `json:"auto_release,omitempty"`
}

type systemResourceUsage struct {
	CapturedAt                           string   `json:"captured_at"`
	CPUCores                             int      `json:"cpu_cores"`
	GOMAXPROCS                           int      `json:"gomaxprocs"`
	Goroutines                           int      `json:"goroutines"`
	HeapAllocBytes                       uint64   `json:"heap_alloc_bytes"`
	HeapSysBytes                         uint64   `json:"heap_sys_bytes"`
	HeapIdleBytes                        uint64   `json:"heap_idle_bytes"`
	StackInuseBytes                      uint64   `json:"stack_inuse_bytes"`
	LastGCTime                           string   `json:"last_gc_time,omitempty"`
	SIPConnections                       int      `json:"sip_connections"`
	RTPOpenTransfers                     int      `json:"rtp_active_transfers"`
	RTPPortPoolUsed                      int      `json:"rtp_port_pool_used"`
	RTPPortPoolTotal                     int      `json:"rtp_port_pool_total"`
	ActiveRequests                       int      `json:"active_requests"`
	ConfiguredGenericDownloadMbps        float64  `json:"configured_generic_download_mbps"`
	ConfiguredGenericPerTransferMbps     float64  `json:"configured_generic_per_transfer_mbps"`
	ConfiguredAdaptiveHotCacheMB         float64  `json:"configured_adaptive_hot_cache_mb"`
	ConfiguredAdaptiveHotWindowMB        float64  `json:"configured_adaptive_hot_window_mb"`
	ConfiguredGenericDownloadWindowMB    float64  `json:"configured_generic_download_window_mb"`
	ConfiguredGenericSegmentConcurrency  int      `json:"configured_generic_segment_concurrency"`
	ConfiguredGenericRTPReorderWindow    int      `json:"configured_generic_rtp_reorder_window_packets"`
	ConfiguredGenericRTPLossTolerance    int      `json:"configured_generic_rtp_loss_tolerance_packets"`
	ConfiguredGenericRTPGapTimeoutMS     int      `json:"configured_generic_rtp_gap_timeout_ms"`
	ConfiguredGenericRTPFECEnabled       bool     `json:"configured_generic_rtp_fec_enabled"`
	ConfiguredGenericRTPFECGroupPackets  int      `json:"configured_generic_rtp_fec_group_packets"`
	StatusColor                          string   `json:"status_color,omitempty"`
	StatusSummary                        string   `json:"status_summary,omitempty"`
	StatusReasons                        []string `json:"status_reasons,omitempty"`
	RecommendedSummary                   string   `json:"recommended_summary,omitempty"`
	RecommendedProfile                   string   `json:"recommended_profile,omitempty"`
	SuggestedActions                     []string `json:"suggested_actions,omitempty"`
	SuitableScenarios                    []string `json:"suitable_scenarios,omitempty"`
	SelfcheckOverall                     string   `json:"selfcheck_overall,omitempty"`
	RTPPortPoolUsagePercent              float64  `json:"rtp_port_pool_usage_percent,omitempty"`
	ActiveRequestUsagePercent            float64  `json:"active_request_usage_percent,omitempty"`
	HeapUsagePercent                     float64  `json:"heap_usage_percent,omitempty"`
	TheoreticalRTPTransferLimit          int      `json:"theoretical_rtp_transfer_limit,omitempty"`
	RecommendedFileTransferMaxConcurrent int      `json:"recommended_file_transfer_max_concurrent,omitempty"`
	RecommendedMaxConcurrent             int      `json:"recommended_max_concurrent,omitempty"`
	RecommendedRateLimitRPS              int      `json:"recommended_rate_limit_rps,omitempty"`
	RecommendedRateLimitBurst            int      `json:"recommended_rate_limit_burst,omitempty"`
	ObservedJitterLossEvents             uint64   `json:"observed_jitter_loss_events,omitempty"`
	ObservedGapTimeouts                  uint64   `json:"observed_gap_timeouts,omitempty"`
	ObservedFECRecovered                 uint64   `json:"observed_fec_recovered,omitempty"`
	ObservedPeakPending                  int      `json:"observed_peak_pending,omitempty"`
	ObservedMaxGapHoldMS                 int64    `json:"observed_max_gap_hold_ms,omitempty"`
	ObservedWriterBlockMS                int64    `json:"observed_writer_block_ms,omitempty"`
	ObservedMaxWriterBlockMS             int64    `json:"observed_max_writer_block_ms,omitempty"`
	ObservedContextCanceled              uint64   `json:"observed_context_canceled,omitempty"`
	ObservedCircuitOpenCount             int      `json:"observed_circuit_open_count,omitempty"`
	ObservedCircuitHalfOpenCount         int      `json:"observed_circuit_half_open_count,omitempty"`
	RuntimeProfileApplied                string   `json:"runtime_profile_applied,omitempty"`
	RuntimeProfileAppliedAt              string   `json:"runtime_profile_applied_at,omitempty"`
	RuntimeProfileReason                 string   `json:"runtime_profile_reason,omitempty"`
	RuntimeProfileChanged                bool     `json:"runtime_profile_changed,omitempty"`
}
