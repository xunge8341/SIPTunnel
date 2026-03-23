package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/observability"
	"siptunnel/internal/persistence"
	"siptunnel/internal/repository"
	filerepo "siptunnel/internal/repository/file"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/service"
	"siptunnel/internal/service/filetransfer"
	"siptunnel/internal/service/sipcontrol"
	"siptunnel/internal/service/taskengine"
	"siptunnel/internal/startupsummary"
	"siptunnel/internal/tunnelmapping"
)

type handlerDeps struct {
	logger            *observability.StructuredLogger
	selfCheckProvider func(context.Context) selfcheck.Report
	networkStatusFunc func(context.Context) NodeNetworkStatus
	startupSummaryFn  func(context.Context) startupsummary.Summary
	audit             observability.AuditStore
	repo              repository.TaskRepository
	engine            *taskengine.Engine
	httpClient        *http.Client

	mu              sync.RWMutex
	limits          OpsLimits
	routes          map[string]OpsRoute
	nodes           []OpsNode
	nodeStore       nodeConfigStore
	mappings        tunnelMappingStore
	localResources  localResourceStore
	runtime         *mappingRuntimeManager
	catalogRegistry *CatalogRegistry
	gbService       *GB28181TunnelService

	nodeConfigSource    string
	mappingSource       string
	localResourceSource string
	uiConfig            config.UIConfig

	lastLinkTest            LinkTestReport
	tunnelConfig            TunnelConfigPayload
	sessionMgr              *tunnelSessionManager
	sqliteStore             *persistence.SQLiteStore
	securitySettings        SecuritySettingsPayload
	licenseInfo             LicenseInfoPayload
	systemSettings          SystemSettingsPayload
	baselineTransportTuning config.TransportTuningConfig
	baselineLimits          OpsLimits
	runtimeProfileState     autoRuntimeProfileState
	accessLogStore          *accessLogStore
	protection              protectionSettings
	protectionRuntime       *protectionRuntime
	securityEvents          *securityEventStore
	rtpPortPool             filetransfer.RTPPortPool
	protectionPath          string
	loadtestJobs            *loadtestJobStore
	opsView                 *opsObservabilityService
	securityPath            string
	tunnelPath              string
	licensePath             string
	licenseFilePath         string
	systemPath              string
	cleaner                 *sqliteCleaner
	metricsCacheMu          sync.RWMutex
	metricsCacheUntil       time.Time
	metricsCacheBody        string
}

type nodeConfigStore interface {
	GetLocalNode() nodeconfig.LocalNodeConfig
	UpdateLocalNode(local nodeconfig.LocalNodeConfig) (nodeconfig.LocalNodeConfig, error)
	ApplyWorkspace(local nodeconfig.LocalNodeConfig, peer nodeconfig.PeerNodeConfig) (nodeconfig.LocalNodeConfig, nodeconfig.PeerNodeConfig, error)
	ListPeers() []nodeconfig.PeerNodeConfig
	CreatePeer(peer nodeconfig.PeerNodeConfig) (nodeconfig.PeerNodeConfig, error)
	UpdatePeer(peer nodeconfig.PeerNodeConfig) (nodeconfig.PeerNodeConfig, error)
	DeletePeer(peerNodeID string) error
}

type responseEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type errorDetails struct {
	Summary    string `json:"summary"`
	Suggestion string `json:"suggestion,omitempty"`
	Detail     string `json:"detail,omitempty"`
	ActionHint string `json:"action_hint,omitempty"`
}

type listData[T any] struct {
	Items      []T        `json:"items"`
	Pagination pagination `json:"pagination"`
}

type pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

type OpsLimits struct {
	RPS           int `json:"rps"`
	Burst         int `json:"burst"`
	MaxConcurrent int `json:"max_concurrent"`
}

type AccessLogEntry struct {
	ID            string `json:"id"`
	OccurredAt    string `json:"occurred_at"`
	MappingName   string `json:"mapping_name"`
	SourceIP      string `json:"source_ip"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	StatusCode    int    `json:"status_code"`
	DurationMS    int64  `json:"duration_ms"`
	FailureReason string `json:"failure_reason"`
	RequestID     string `json:"request_id"`
	TraceID       string `json:"trace_id"`
}

type SystemSettingsPayload struct {
	SQLitePath                        string  `json:"sqlite_path"`
	LogCleanupCron                    string  `json:"log_cleanup_cron"`
	MaxTaskAgeDays                    int     `json:"max_task_age_days"`
	MaxTaskRecords                    int     `json:"max_task_records"`
	MaxAccessLogAgeDays               int     `json:"max_access_log_age_days"`
	MaxAccessLogRecords               int     `json:"max_access_log_records"`
	MaxAuditAgeDays                   int     `json:"max_audit_age_days"`
	MaxAuditRecords                   int     `json:"max_audit_records"`
	MaxDiagnosticAgeDays              int     `json:"max_diagnostic_age_days"`
	MaxDiagnosticRecords              int     `json:"max_diagnostic_records"`
	MaxLoadtestAgeDays                int     `json:"max_loadtest_age_days"`
	MaxLoadtestRecords                int     `json:"max_loadtest_records"`
	AdminAllowCIDR                    string  `json:"admin_allow_cidr"`
	AdminRequireMFA                   bool    `json:"admin_require_mfa"`
	GenericDownloadTotalMbps          float64 `json:"generic_download_total_mbps,omitempty"`
	GenericDownloadPerTransferMbps    float64 `json:"generic_download_per_transfer_mbps,omitempty"`
	GenericDownloadWindowMB           float64 `json:"generic_download_window_mb,omitempty"`
	AdaptiveHotCacheMB                float64 `json:"adaptive_hot_cache_mb,omitempty"`
	AdaptiveHotWindowMB               float64 `json:"adaptive_hot_window_mb,omitempty"`
	GenericDownloadSegmentConcurrency int     `json:"generic_download_segment_concurrency,omitempty"`
	GenericDownloadRTPReorderWindow   int     `json:"generic_download_rtp_reorder_window_packets,omitempty"`
	GenericDownloadRTPLossTolerance   int     `json:"generic_download_rtp_loss_tolerance_packets,omitempty"`
	GenericDownloadRTPGapTimeoutMS    int     `json:"generic_download_rtp_gap_timeout_ms,omitempty"`
	GenericDownloadRTPFECEnabled      bool    `json:"generic_download_rtp_fec_enabled"`
	GenericDownloadRTPFECGroupPackets int     `json:"generic_download_rtp_fec_group_packets,omitempty"`
	CleanerLastRunAt                  string  `json:"cleaner_last_run_at"`
	CleanerLastResult                 string  `json:"cleaner_last_result"`
	CleanerLastRemovedRecords         int     `json:"cleaner_last_removed_records"`
}

type OpsRoute struct {
	// Deprecated: OpsRoute is the legacy /api/routes response model.
	APICode    string `json:"api_code"`
	HTTPMethod string `json:"http_method"`
	HTTPPath   string `json:"http_path"`
	Enabled    bool   `json:"enabled"`
}

type TunnelMapping = tunnelmapping.TunnelMapping

type MappingCapabilityValidation = tunnelmapping.CapabilityValidationResult

type MappingWithWarnings struct {
	Mapping   TunnelMappingView `json:"mapping"`
	BoundPeer *PeerBinding      `json:"bound_peer,omitempty"`
	Warnings  []string          `json:"warnings,omitempty"`
}

type MappingTestResponse struct {
	Passed             bool               `json:"passed"`
	Status             string             `json:"status"`
	Stages             []MappingTestStage `json:"stages"`
	FailureStage       string             `json:"failure_stage,omitempty"`
	SignalingRequest   string             `json:"signaling_request"`
	ResponseChannel    string             `json:"response_channel"`
	RegistrationStatus string             `json:"registration_status"`
	FailureReason      string             `json:"failure_reason,omitempty"`
	SuggestedAction    string             `json:"suggested_action,omitempty"`
}

type MappingTestStage struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	Status          string `json:"status"`
	Passed          bool   `json:"passed"`
	Detail          string `json:"detail"`
	BlockingReason  string `json:"blocking_reason,omitempty"`
	SuggestedAction string `json:"suggested_action,omitempty"`
}

type mappingListResponse struct {
	Items        []TunnelMappingView `json:"items"`
	BoundPeer    *PeerBinding        `json:"bound_peer,omitempty"`
	BindingError string              `json:"binding_error,omitempty"`
	Warnings     []string            `json:"warnings,omitempty"`
}

type TunnelMappingView struct {
	TunnelMapping
	LinkStatus      string `json:"link_status,omitempty"`
	LinkStatusText  string `json:"link_status_text,omitempty"`
	StatusReason    string `json:"status_reason,omitempty"`
	FailureReason   string `json:"failure_reason,omitempty"`
	SuggestedAction string `json:"suggested_action,omitempty"`
	RequestCount    int    `json:"request_count,omitempty"`
	FailureCount    int    `json:"failure_count,omitempty"`
	AvgLatencyMS    int64  `json:"avg_latency_ms,omitempty"`
}

type PeerBinding struct {
	PeerNodeID        string `json:"peer_node_id"`
	PeerName          string `json:"peer_name"`
	PeerSignalingIP   string `json:"peer_signaling_ip"`
	PeerSignalingPort int    `json:"peer_signaling_port"`
}

type OpsNode struct {
	NodeID     string `json:"node_id"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	Endpoint   string `json:"endpoint"`
	DataSource string `json:"data_source,omitempty"`
}

type opsActionRequest struct {
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

type NodeNetworkStatus struct {
	NetworkMode         config.NetworkMode               `json:"network_mode"`
	Capability          config.Capability                `json:"capability"`
	CurrentNetworkMode  config.NetworkMode               `json:"current_network_mode"`
	CurrentCapability   config.Capability                `json:"current_capability"`
	CompatibilityStatus nodeconfig.CheckResult           `json:"compatibility_status"`
	CapabilitySummary   startupsummary.CapabilitySummary `json:"capability_summary"`
	TransportPlan       config.TunnelTransportPlan       `json:"transport_plan"`
	BoundPeer           *PeerBinding                     `json:"bound_peer,omitempty"`
	PeerBindingError    string                           `json:"peer_binding_error,omitempty"`
	SIP                 SIPNetworkStatus                 `json:"sip"`
	RTP                 RTPNetworkStatus                 `json:"rtp"`
	RecentBindErrors    []string                         `json:"recent_bind_errors"`
	RecentNetworkErrors []string                         `json:"recent_network_errors"`
}

type SystemStatusCapability struct {
	SupportsSmallRequestBody        bool `json:"supports_small_request_body"`
	SupportsLargeResponseBody       bool `json:"supports_large_response_body"`
	SupportsStreamingResponse       bool `json:"supports_streaming_response"`
	SupportsLargeFileUpload         bool `json:"supports_large_file_upload"`
	SupportsBidirectionalHTTPTunnel bool `json:"supports_bidirectional_http_tunnel"`
}

type SystemStatusResponse struct {
	TunnelStatus             string                 `json:"tunnel_status"`
	ConnectionReason         string                 `json:"connection_reason"`
	NetworkMode              config.NetworkMode     `json:"network_mode"`
	RegistrationStatus       string                 `json:"registration_status,omitempty"`
	HeartbeatStatus          string                 `json:"heartbeat_status,omitempty"`
	LastRegisterTime         string                 `json:"last_register_time,omitempty"`
	LastHeartbeatTime        string                 `json:"last_heartbeat_time,omitempty"`
	LastFailureReason        string                 `json:"last_failure_reason,omitempty"`
	NextRetryTime            string                 `json:"next_retry_time,omitempty"`
	MappingTotal             int                    `json:"mapping_total"`
	MappingAbnormalTotal     int                    `json:"mapping_abnormal_total"`
	LatestMappingErrorReason string                 `json:"latest_mapping_error_reason,omitempty"`
	BoundPeer                *PeerBinding           `json:"bound_peer,omitempty"`
	PeerBindingError         string                 `json:"peer_binding_error,omitempty"`
	Capability               SystemStatusCapability `json:"capability"`
}

type NodeDetailResponse struct {
	LocalNode           nodeconfig.LocalNodeConfig `json:"local_node"`
	CurrentNetworkMode  config.NetworkMode         `json:"current_network_mode"`
	CurrentCapability   config.Capability          `json:"current_capability"`
	CompatibilityStatus nodeconfig.CheckResult     `json:"compatibility_status"`
}

type NodeConfigEndpoint struct {
	NodeIP           string `json:"node_ip"`
	SignalingPort    int    `json:"signaling_port"`
	DeviceID         string `json:"device_id"`
	NodeType         string `json:"node_type,omitempty"`
	RTPPortStart     int    `json:"rtp_port_start,omitempty"`
	RTPPortEnd       int    `json:"rtp_port_end,omitempty"`
	MappingPortStart int    `json:"mapping_port_start,omitempty"`
	MappingPortEnd   int    `json:"mapping_port_end,omitempty"`
}

type NodeConfigPayload struct {
	LocalNode NodeConfigEndpoint `json:"local_node"`
	PeerNode  NodeConfigEndpoint `json:"peer_node"`
}

type TunnelConfigPayload struct {
	ChannelProtocol     string `json:"channel_protocol"`
	ConnectionInitiator string `json:"connection_initiator"`
	MappingRelayMode    string `json:"mapping_relay_mode"`
	// Codes are derived from node configuration and are read-only in tunnel config.
	LocalDeviceID                  string                  `json:"local_device_id"`
	PeerDeviceID                   string                  `json:"peer_device_id"`
	HeartbeatIntervalSec           int                     `json:"heartbeat_interval_sec"`
	RegisterRetryCount             int                     `json:"register_retry_count"`
	RegisterRetryIntervalSec       int                     `json:"register_retry_interval_sec"`
	RegistrationStatus             string                  `json:"registration_status"`
	LastRegisterTime               string                  `json:"last_register_time"`
	LastHeartbeatTime              string                  `json:"last_heartbeat_time"`
	HeartbeatStatus                string                  `json:"heartbeat_status"`
	LastFailureReason              string                  `json:"last_failure_reason"`
	NextRetryTime                  string                  `json:"next_retry_time"`
	ConsecutiveHBTimeout           int                     `json:"consecutive_heartbeat_timeout"`
	SupportedCapabilities          []string                `json:"supported_capabilities"`
	RequestChannel                 string                  `json:"request_channel"`
	ResponseChannel                string                  `json:"response_channel"`
	NetworkMode                    config.NetworkMode      `json:"network_mode"`
	Capability                     config.Capability       `json:"capability"`
	CapabilityItems                []config.CapabilityItem `json:"capability_items"`
	RegisterAuthEnabled            bool                    `json:"register_auth_enabled"`
	RegisterAuthUsername           string                  `json:"register_auth_username"`
	RegisterAuthPassword           string                  `json:"register_auth_password,omitempty"`
	RegisterAuthPasswordConfigured bool                    `json:"register_auth_password_configured"`
	RegisterAuthRealm              string                  `json:"register_auth_realm"`
	RegisterAuthAlgorithm          string                  `json:"register_auth_algorithm"`
	CatalogSubscribeExpiresSec     int                     `json:"catalog_subscribe_expires_sec"`
}

type TunnelConfigUpdatePayload struct {
	ChannelProtocol            string             `json:"channel_protocol"`
	ConnectionInitiator        string             `json:"connection_initiator"`
	MappingRelayMode           string             `json:"mapping_relay_mode"`
	HeartbeatIntervalSec       int                `json:"heartbeat_interval_sec"`
	RegisterRetryCount         int                `json:"register_retry_count"`
	RegisterRetryIntervalSec   int                `json:"register_retry_interval_sec"`
	RegistrationStatus         string             `json:"registration_status"`
	LastRegisterTime           string             `json:"last_register_time"`
	LastHeartbeatTime          string             `json:"last_heartbeat_time"`
	HeartbeatStatus            string             `json:"heartbeat_status"`
	NetworkMode                config.NetworkMode `json:"network_mode"`
	RegisterAuthEnabled        *bool              `json:"register_auth_enabled"`
	RegisterAuthUsername       string             `json:"register_auth_username"`
	RegisterAuthPassword       string             `json:"register_auth_password"`
	RegisterAuthRealm          string             `json:"register_auth_realm"`
	RegisterAuthAlgorithm      string             `json:"register_auth_algorithm"`
	CatalogSubscribeExpiresSec int                `json:"catalog_subscribe_expires_sec"`
}

type SIPNetworkStatus struct {
	ListenIP                 string `json:"listen_ip"`
	ListenPort               int    `json:"listen_port"`
	Transport                string `json:"transport"`
	CurrentSessions          int    `json:"current_sessions"`
	CurrentConnections       int    `json:"current_connections"`
	AcceptedConnectionsTotal uint64 `json:"accepted_connections_total"`
	ClosedConnectionsTotal   uint64 `json:"closed_connections_total"`
	ReadTimeoutTotal         uint64 `json:"read_timeout_total"`
	WriteTimeoutTotal        uint64 `json:"write_timeout_total"`
	ConnectionErrorTotal     uint64 `json:"connection_error_total"`
	TCPKeepAliveEnabled      bool   `json:"tcp_keepalive_enabled"`
	TCPKeepAliveIntervalMS   int    `json:"tcp_keepalive_interval_ms"`
	TCPReadBufferBytes       int    `json:"tcp_read_buffer_bytes"`
	TCPWriteBufferBytes      int    `json:"tcp_write_buffer_bytes"`
	MaxConnections           int    `json:"max_connections"`
}

type RTPNetworkStatus struct {
	ListenIP            string `json:"listen_ip"`
	PortStart           int    `json:"port_start"`
	PortEnd             int    `json:"port_end"`
	Transport           string `json:"transport"`
	ActiveTransfers     int    `json:"active_transfers"`
	UsedPorts           int    `json:"used_ports"`
	AvailablePorts      int    `json:"available_ports"`
	PortPoolTotal       int    `json:"rtp_port_pool_total"`
	PortPoolUsed        int    `json:"rtp_port_pool_used"`
	PortAllocFailTotal  int    `json:"rtp_port_alloc_fail_total"`
	TCPSessionsCurrent  int64  `json:"rtp_tcp_sessions_current"`
	TCPSessionsTotal    uint64 `json:"rtp_tcp_sessions_total"`
	TCPReadErrorsTotal  uint64 `json:"rtp_tcp_read_errors_total"`
	TCPWriteErrorsTotal uint64 `json:"rtp_tcp_write_errors_total"`
}

type DiagnosticExportData struct {
	GeneratedAt time.Time  `json:"generated_at"`
	JobID       string     `json:"job_id"`
	NodeID      string     `json:"node_id"`
	RequestID   string     `json:"request_id,omitempty"`
	TraceID     string     `json:"trace_id,omitempty"`
	FileName    string     `json:"file_name"`
	OutputDir   string     `json:"output_dir"`
	Files       []DiagFile `json:"files"`
}

type DiagFile struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     any    `json:"content"`
}

type updateLimitsRequest struct {
	RPS           int `json:"rps"`
	Burst         int `json:"burst"`
	MaxConcurrent int `json:"max_concurrent"`
}

type updateRoutesRequest struct {
	Routes []OpsRoute `json:"routes"`
}

type tunnelMappingStore interface {
	List() []TunnelMapping
	Create(TunnelMapping) (TunnelMapping, error)
	Update(id string, mapping TunnelMapping) (TunnelMapping, error)
	Delete(id string) error
}

type LinkTestReport struct {
	Passed     bool           `json:"passed"`
	Status     string         `json:"status"`
	RequestID  string         `json:"request_id"`
	TraceID    string         `json:"trace_id"`
	DurationMS int64          `json:"duration_ms"`
	CheckedAt  time.Time      `json:"checked_at"`
	Items      []LinkTestItem `json:"items"`
	MockTarget string         `json:"mock_target"`
}

type LinkTestItem struct {
	Name       string `json:"name"`
	Passed     bool   `json:"passed"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	DurationMS int64  `json:"duration_ms"`
}

type capacityAssessmentRequest struct {
	SummaryFile string `json:"summary_file"`
	Current     struct {
		CommandMaxConcurrent      int `json:"command_max_concurrent"`
		FileTransferMaxConcurrent int `json:"file_transfer_max_concurrent"`
		RTPPortPoolSize           int `json:"rtp_port_pool_size"`
		MaxConnections            int `json:"max_connections"`
		RateLimitRPS              int `json:"rate_limit_rps"`
		RateLimitBurst            int `json:"rate_limit_burst"`
	} `json:"current"`
}

type HandlerOptions struct {
	LogDir                 string
	LogRetention           observability.LogRetentionPolicy
	AuditDir               string
	DataDir                string
	SQLitePath             string
	UseMemoryBackend       bool
	Retention              persistence.RetentionPolicy
	CleanupInterval        time.Duration
	RTPPortPool            filetransfer.RTPPortPool
	SelfCheckProvider      func(context.Context) selfcheck.Report
	NetworkStatusFunc      func(context.Context) NodeNetworkStatus
	StartupSummaryProvider func(context.Context) startupsummary.Summary
	UIConfig               config.UIConfig
}

func dataSourceLabel(path, category string) string {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		cleanPath = filepath.Join(".", "data", "final")
	}
	return fmt.Sprintf("file:%s/%s", filepath.Clean(cleanPath), category)
}

func NewHandler() http.Handler {
	h, _, err := NewHandlerWithOptions(HandlerOptions{})
	if err != nil {
		return wrapHTTPRecovery("gateway-init", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeErrorWithDetails(w, http.StatusInternalServerError, "HANDLER_INIT_FAILED", "handler initialization failed", err.Error(), "检查日志目录、SQLite 路径和 data 目录权限后重新启动。", "启动前先执行配置校验并复核 /api/selfcheck。")
		}))
	}
	return h
}

func NewHandlerWithOptions(opts HandlerOptions) (http.Handler, io.Closer, error) {
	repo := repository.TaskRepository(memrepo.NewTaskRepository())
	engine := taskengine.NewEngine(repo, service.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second})
	logger := observability.NewStructuredLogger(nil)
	var logCloser io.Closer
	if opts.LogDir != "" {
		var err error
		logger, logCloser, err = observability.NewStructuredLoggerWithFile(opts.LogDir, opts.LogRetention)
		if err != nil {
			return nil, nil, fmt.Errorf("init logger: %w", err)
		}
	}
	audit := observability.AuditStore(observability.NewInMemoryAuditStore())
	var sqliteStore *persistence.SQLiteStore
	var auditCloser io.Closer
	if !opts.UseMemoryBackend && strings.TrimSpace(opts.SQLitePath) != "" {
		store, err := persistence.OpenSQLiteStore(opts.SQLitePath, opts.Retention)
		if err != nil {
			if logCloser != nil {
				_ = logCloser.Close()
			}
			return nil, nil, fmt.Errorf("init sqlite store: %w", err)
		}
		sqliteStore = store
		repo = store.TaskRepository()
		engine = taskengine.NewEngine(repo, service.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second})
		audit = store
		auditCloser = store
	} else if opts.AuditDir != "" {
		store, err := observability.NewFileBackedAuditStore(opts.AuditDir)
		if err != nil {
			if logCloser != nil {
				_ = logCloser.Close()
			}
			return nil, nil, fmt.Errorf("init audit store: %w", err)
		}
		audit = store
		auditCloser = store
	}
	dataDir := strings.TrimSpace(opts.DataDir)
	if dataDir == "" {
		dataDir = filepath.Join(".", "data", "final")
	}
	nodeStore, err := filerepo.NewNodeConfigStore(filepath.Join(dataDir, "node_config.json"))
	if err != nil {
		if logCloser != nil {
			_ = logCloser.Close()
		}
		if auditCloser != nil {
			_ = auditCloser.Close()
		}
		return nil, nil, fmt.Errorf("init node config store: %w", err)
	}
	mappingStore, err := filerepo.NewTunnelMappingStore(filepath.Join(dataDir, "tunnel_mappings.json"))
	if err != nil {
		if logCloser != nil {
			_ = logCloser.Close()
		}
		if auditCloser != nil {
			_ = auditCloser.Close()
		}
		return nil, nil, fmt.Errorf("init tunnel mapping store: %w", err)
	}
	localResourceStore, err := newFileLocalResourceStore(filepath.Join(dataDir, "local_resources.json"))
	if err != nil {
		if logCloser != nil {
			_ = logCloser.Close()
		}
		if auditCloser != nil {
			_ = auditCloser.Close()
		}
		return nil, nil, fmt.Errorf("init local resource store: %w", err)
	}

	deps := handlerDeps{
		logger: logger,
		audit:  audit,
		repo:   repo,
		engine: engine,
		httpClient: &http.Client{
			Timeout: 1500 * time.Millisecond,
		},
		limits:              defaultOpsLimits(),
		routes:              map[string]OpsRoute{},
		mappings:            mappingStore,
		localResources:      localResourceStore,
		runtime:             nil,
		nodeStore:           nodeStore,
		nodeConfigSource:    dataSourceLabel(dataDir, "node_config.json"),
		mappingSource:       dataSourceLabel(dataDir, "tunnel_mappings.json"),
		localResourceSource: dataSourceLabel(dataDir, "local_resources.json"),
		uiConfig:            opts.UIConfig,
		selfCheckProvider:   opts.SelfCheckProvider,
		networkStatusFunc:   opts.NetworkStatusFunc,
		startupSummaryFn:    opts.StartupSummaryProvider,
		tunnelConfig:        defaultTunnelConfigPayload(config.DefaultNetworkMode()),
		sqliteStore:         sqliteStore,
		securitySettings:    defaultSecuritySettings(),
		licenseInfo:         defaultLicenseInfo(),
		accessLogStore:      newAccessLogStore(7, 20000, sqliteStore),
		protection:          defaultProtectionSettings(),
		protectionRuntime:   nil,
		securityEvents:      newSecurityEventStore(512, filepath.Join(dataDir, "security-events.jsonl"), audit),
		loadtestJobs:        newLoadtestJobStore(),
		securityPath:        filepath.Join(dataDir, "security_settings.json"),
		tunnelPath:          filepath.Join(dataDir, "tunnel_config.json"),
		licensePath:         filepath.Join(dataDir, "license_info.json"),
		licenseFilePath:     filepath.Join(dataDir, "license.lic"),
		protectionPath:      filepath.Join(dataDir, "protection_settings.json"),
		rtpPortPool:         opts.RTPPortPool,
	}

	deps.securitySettings = loadJSONOrDefault(deps.securityPath, deps.securitySettings)
	deps.tunnelConfig = normalizeTunnelConfigPayload(loadJSONOrDefault(deps.tunnelPath, deps.tunnelConfig), config.DefaultNetworkMode())
	deps.licenseInfo = loadJSONOrDefault(deps.licensePath, deps.licenseInfo)
	hw := collectLicenseHardware(nodeStore.GetLocalNode().NodeID)
	if rawLicense, err := os.ReadFile(deps.licenseFilePath); err == nil && strings.TrimSpace(string(rawLicense)) != "" {
		if verified, verifyErr := verifyLicenseSummary(string(rawLicense), hw.MachineCode); verifyErr == nil {
			deps.licenseInfo = verified
		}
	} else {
		if trialContent, trialInfo, trialErr := buildTrialLicenseContent(hw, time.Now().UTC()); trialErr == nil {
			deps.licenseInfo = trialInfo
			_ = os.WriteFile(deps.licenseFilePath, []byte(trialContent), 0o644)
			_ = saveJSON(deps.licensePath, trialInfo)
		}
	}
	deps.systemPath = filepath.Join(dataDir, "system_settings.json")
	deps.systemSettings = loadJSONOrDefault(deps.systemPath, defaultSystemSettings(&deps, opts.SQLitePath))
	normalizeSystemSettingsRuntimeProfile(&deps.systemSettings, currentTransportTuning())
	deps.baselineTransportTuning = applySystemSettingsRuntimeProfile(config.DefaultTransportTuningConfig(), deps.systemSettings)
	ApplyTransportTuning(deps.baselineTransportTuning)
	deps.limits = normalizeOpsLimits(deps.limits)
	deps.baselineLimits = deps.limits
	deps.protection = normalizeProtectionSettings(loadJSONOrDefault(deps.protectionPath, deps.protection))
	deps.protectionRuntime = newProtectionRuntime(deps.limits)
	if _, err := os.Stat(deps.securityPath); err != nil {
		_ = saveJSON(deps.securityPath, deps.securitySettings)
	}
	if _, err := os.Stat(deps.tunnelPath); err != nil {
		_ = saveJSON(deps.tunnelPath, deps.tunnelConfig)
	}
	if _, err := os.Stat(deps.systemPath); err != nil {
		_ = saveJSON(deps.systemPath, deps.systemSettings)
	}
	if _, err := os.Stat(deps.protectionPath); err != nil {
		_ = saveJSON(deps.protectionPath, deps.protection)
	}
	if deps.accessLogStore != nil {
		deps.accessLogStore.Configure(deps.systemSettings.MaxAccessLogAgeDays, deps.systemSettings.MaxAccessLogRecords)
	}
	deps.opsView = newOpsObservabilityService(deps.accessLogStore)
	if sqliteStore != nil {
		sqliteStore.UpdateRetention(persistence.RetentionPolicy{
			MaxTaskAgeDays:       deps.systemSettings.MaxTaskAgeDays,
			MaxTaskRecords:       deps.systemSettings.MaxTaskRecords,
			MaxAccessLogAgeDays:  deps.systemSettings.MaxAccessLogAgeDays,
			MaxAccessLogRecords:  deps.systemSettings.MaxAccessLogRecords,
			MaxAuditAgeDays:      deps.systemSettings.MaxAuditAgeDays,
			MaxAuditRecords:      deps.systemSettings.MaxAuditRecords,
			MaxDiagnosticAgeDays: deps.systemSettings.MaxDiagnosticAgeDays,
			MaxDiagnosticRecords: deps.systemSettings.MaxDiagnosticRecords,
		})
	}

	if sqliteStore != nil {
		_ = sqliteStore.SaveSystemConfig(context.Background(), "initial.limits", deps.limits)
		cleanupSchedule := strings.TrimSpace(deps.systemSettings.LogCleanupCron)
		if cleanupSchedule == "" {
			cleanupSchedule = cleanupScheduleFromInterval(opts.CleanupInterval)
		}
		cleaner := newSQLiteCleaner(sqliteStore, cleanupSchedule)
		cleaner.Start()
		deps.cleaner = cleaner
		auditCloser = joinClosers(auditCloser, cleaner)
	}
	if deps.runtime != nil {
		deps.runtime.SetAccessLogRecorder(func(entry AccessLogEntry) {
			if deps.accessLogStore != nil {
				deps.accessLogStore.Add(entry)
			}
		})
	}
	sipcontrol.SetGlobalSecurityEventRecorder(func(event sipcontrol.SecurityEvent) {
		if deps.securityEvents != nil {
			deps.securityEvents.Add(securityEventRecord{When: formatTimestamp(event.OccurredAt), Category: event.Category, Transport: event.Transport, RequestID: event.RequestID, TraceID: event.TraceID, SessionID: event.SessionID, Reason: event.Reason})
		}
	})
	filetransfer.SetSecurityEventRecorder(func(category, transport, reason, requestID, traceID string) {
		if deps.securityEvents != nil {
			deps.securityEvents.Add(securityEventRecord{When: formatTimestamp(time.Now().UTC()), Category: category, Transport: transport, RequestID: strings.TrimRight(requestID, "\x00"), TraceID: strings.TrimRight(traceID, "\x00"), Reason: reason})
		}
	})
	deps.catalogRegistry = NewCatalogRegistry()
	deps.catalogRegistry.SyncLocalResources(deps.localResources.List(), deps.tunnelConfig.NetworkMode)
	deps.catalogRegistry.SyncExposureMappings(deps.mappings.List())
	gbService := NewGB28181TunnelService(func() nodeconfig.LocalNodeConfig {
		return deps.nodeStore.GetLocalNode()
	}, func() []tunnelmapping.TunnelMapping {
		return deps.mappings.List()
	}, func() []LocalResourceRecord {
		if deps.localResources == nil {
			return nil
		}
		return deps.localResources.List()
	}, func() TunnelConfigPayload {
		deps.mu.RLock()
		defer deps.mu.RUnlock()
		return deps.tunnelConfig
	}, deps.catalogRegistry, deps.accessLogStore, deps.rtpPortPool)
	gbService.SetCatalogChangeCallback(func() { deps.syncMappingRuntime() })
	SetGlobalGB28181TunnelService(gbService)
	deps.gbService = gbService
	deps.sessionMgr = newTunnelSessionManager(&gb28181Registrar{nodeStore: deps.nodeStore, preferredPeerID: func() string {
		deps.mu.RLock()
		defer deps.mu.RUnlock()
		return strings.TrimSpace(deps.tunnelConfig.PeerDeviceID)
	}, localNode: func() nodeconfig.LocalNodeConfig {
		return deps.nodeStore.GetLocalNode()
	}, tunnelConfig: func() TunnelConfigPayload {
		deps.mu.RLock()
		defer deps.mu.RUnlock()
		return deps.tunnelConfig
	}, portPool: deps.rtpPortPool}, deps.tunnelConfig)
	deps.sessionMgr.Start()
	deps.runtime = newMappingRuntimeManager(newTunneledHTTPMappingForwarder(func(mapping tunnelmapping.TunnelMapping) (*PeerBinding, error) {
		if strings.TrimSpace(mapping.PeerNodeID) != "" {
			for _, peer := range deps.nodeStore.ListPeers() {
				if peer.Enabled && strings.EqualFold(strings.TrimSpace(peer.PeerNodeID), strings.TrimSpace(mapping.PeerNodeID)) {
					return &PeerBinding{PeerNodeID: peer.PeerNodeID, PeerName: peer.PeerName, PeerSignalingIP: peer.PeerSignalingIP, PeerSignalingPort: peer.PeerSignalingPort}, nil
				}
			}
		}
		return deps.currentPeerBinding()
	}, func() nodeconfig.LocalNodeConfig {
		return deps.nodeStore.GetLocalNode()
	}, func() string {
		deps.mu.RLock()
		defer deps.mu.RUnlock()
		return deps.tunnelConfig.MappingRelayMode
	}, deps.rtpPortPool))
	setGlobalTunnelHTTPRelayExecutor(func(ctx context.Context, req tunnelHTTPRelayRequest) (tunnelHTTPRelayResponse, error) {
		return executeTunnelRelayRequest(ctx, req, deps.accessLogStore, deps.nodeStore.GetLocalNode(), deps.rtpPortPool)
	})
	if deps.runtime != nil {
		deps.runtime.SetProtector(deps.protectionRuntime)
		if deps.gbService != nil {
			deps.gbService.SetProtector(deps.protectionRuntime)
		}
		deps.runtime.SetAccessLogRecorder(func(entry AccessLogEntry) {
			if deps.accessLogStore != nil {
				deps.accessLogStore.Add(entry)
			}
		})
	}
	if deps.networkStatusFunc == nil {
		defaults := config.DefaultNetworkConfig()
		deps.networkStatusFunc = func(context.Context) NodeNetworkStatus {
			availablePorts := defaults.RTP.PortEnd - defaults.RTP.PortStart + 1
			if availablePorts < 0 {
				availablePorts = 0
			}
			mode := defaults.Mode.Normalize()
			capability := config.DeriveCapability(mode)
			transportPlan := config.ResolveTransportPlan(mode)
			return NodeNetworkStatus{
				NetworkMode:   mode,
				Capability:    capability,
				TransportPlan: transportPlan,
				CapabilitySummary: startupsummary.CapabilitySummary{
					Supported:   capability.SupportedFeatures(),
					Unsupported: capability.UnsupportedFeatures(),
					Items:       capability.Matrix(),
				},
				SIP: SIPNetworkStatus{
					ListenIP:           defaults.SIP.ListenIP,
					ListenPort:         defaults.SIP.ListenPort,
					Transport:          defaults.SIP.Transport,
					CurrentSessions:    0,
					CurrentConnections: 0,
				},
				RTP: RTPNetworkStatus{
					ListenIP:            defaults.RTP.ListenIP,
					PortStart:           defaults.RTP.PortStart,
					PortEnd:             defaults.RTP.PortEnd,
					Transport:           defaults.RTP.Transport,
					ActiveTransfers:     0,
					UsedPorts:           0,
					AvailablePorts:      availablePorts,
					PortPoolTotal:       availablePorts,
					PortPoolUsed:        0,
					PortAllocFailTotal:  0,
					TCPSessionsCurrent:  0,
					TCPSessionsTotal:    0,
					TCPReadErrorsTotal:  0,
					TCPWriteErrorsTotal: 0,
				},
				RecentBindErrors:    []string{},
				RecentNetworkErrors: []string{},
			}
		}
	}
	if deps.startupSummaryFn == nil {
		deps.startupSummaryFn = func(context.Context) startupsummary.Summary {
			return startupsummary.Summary{}
		}
	}
	deps.syncMappingRuntime()
	return newMux(deps), joinClosers(logCloser, auditCloser, deps.runtime, deps.sessionMgr), nil
}

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var first error
	for _, c := range m {
		if c == nil {
			continue
		}
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (d *handlerDeps) syncMappingRuntime() {
	var items []TunnelMapping
	if d.mappings != nil {
		items = d.mappings.List()
	}
	var resources []LocalResourceRecord
	if d.localResources != nil {
		resources = d.localResources.List()
	}
	effective := items
	if d.catalogRegistry != nil {
		d.catalogRegistry.SyncLocalResources(resources, d.tunnelConfig.NetworkMode)
		plan := buildCatalogExposurePlan(items, d.catalogRegistry.RemoteSnapshot())
		effective = plan.EffectiveMappings
	}
	if d.runtime != nil {
		d.runtime.SyncMappings(effective)
	}
	if d.catalogRegistry != nil {
		d.catalogRegistry.SyncExposureMappings(effective)
	}
}

func joinClosers(closers ...io.Closer) io.Closer {
	out := make(multiCloser, 0, len(closers))
	for _, c := range closers {
		if c != nil {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (d *handlerDeps) withObservability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := observability.ExtractTraceContext(r)
		fields := observability.BuildCoreFieldsFromRequest(r)
		ctx = observability.WithCoreFields(ctx, fields)
		r = r.WithContext(ctx)
		if shouldLogInboundRequest(r) {
			d.logger.Info(ctx, "request_received", fields, "method", r.Method, "path", r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

func (d *handlerDeps) healthz(w http.ResponseWriter, r *http.Request) {
	discardRequestBody(r, 64*1024)
	writeHealthzFastJSON(w)
}

func (d *handlerDeps) readinessReport(ctx context.Context) (bool, []string) {
	reasons := make([]string, 0, 4)
	if d.selfCheckProvider != nil {
		report := d.selfCheckProvider(ctx)
		if report.Overall == selfcheck.LevelError {
			reasons = append(reasons, "selfcheck.overall=error")
		}
	}
	if d.networkStatusFunc != nil {
		status := d.networkStatusFunc(ctx)
		if status.SIP.ListenPort <= 0 {
			reasons = append(reasons, "sip listener not ready")
		}
		if status.RTP.PortPoolTotal <= 0 || status.RTP.AvailablePorts <= 0 {
			reasons = append(reasons, "rtp port pool unavailable")
		}
	}
	if d.runtime != nil {
		for _, item := range d.runtime.Snapshot() {
			if item.State == mappingStateStartFailed || item.State == mappingStateInterrupted {
				reasons = append(reasons, item.Reason)
				break
			}
		}
	}
	return len(reasons) == 0, reasons
}

func (d *handlerDeps) readyz(w http.ResponseWriter, r *http.Request) {
	discardRequestBody(r, 64*1024)
	fields := observability.CoreFieldsFromContext(r.Context())
	ready, reasons := d.readinessReport(r.Context())
	statusCode := http.StatusOK
	result := "READY"
	state := "ready"
	if !ready {
		statusCode = http.StatusServiceUnavailable
		result = "NOT_READY"
		state = "not_ready"
	}
	fields.ResultCode = result
	writeJSON(w, statusCode, responseEnvelope{Code: result, Message: state, Data: map[string]any{"status": state, "reasons": reasons}})
	if !ready || shouldLogReadyzSuccess(time.Now()) {
		d.logger.Info(r.Context(), "readyz_checked", fields, "ready", ready, "reasons", reasons)
	}
}

func (d *handlerDeps) demoProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	ctx, span := observability.StartSpan(r.Context(), "demo.process")
	defer span.End()

	fields := observability.CoreFieldsFromContext(ctx)
	initiator := r.Header.Get("X-Initiator")
	if initiator == "" {
		initiator = "anonymous"
	}

	validated := fields.APICode != "unknown"
	if validated {
		fields.ResultCode = "OK"
	} else {
		fields.ResultCode = "VALIDATION_FAILED"
	}

	auditLogger := observability.NewAuditLogger(d.logger, d.audit)
	_ = auditLogger.Record(ctx, observability.AuditEvent{
		Who:               initiator,
		When:              time.Now().UTC(),
		RequestType:       "demo.process",
		ValidationPassed:  validated,
		LocalServiceRoute: "local.mock.service",
		FinalResult:       fields.ResultCode,
		OpsAction:         "NONE",
		Core:              fields,
	})

	status := http.StatusOK
	if !validated {
		status = http.StatusBadRequest
	}
	observability.InjectTraceContext(ctx, w.Header())
	writeJSON(w, status, responseEnvelope{Code: fields.ResultCode, Message: "processed", Data: map[string]any{"core": fields}})
}

func (d *handlerDeps) validateMappingAgainstCapability(mapping TunnelMapping) MappingCapabilityValidation {
	status := d.networkStatusFunc(context.Background())
	return tunnelmapping.ValidateMappingCapability(mapping, status.NetworkMode, status.Capability)
}

func (d *handlerDeps) validateMappingsAgainstCapability(mappings []TunnelMapping) MappingCapabilityValidation {
	status := d.networkStatusFunc(context.Background())
	return tunnelmapping.ValidateMappingsCapability(mappings, status.NetworkMode, status.Capability)
}

func writeJSON(w http.ResponseWriter, status int, payload responseEnvelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, responseEnvelope{Code: code, Message: message})
}

func writeErrorWithDetails(w http.ResponseWriter, status int, code, message, detail, suggestion, actionHint string) {
	writeJSON(w, status, responseEnvelope{Code: code, Message: message, Data: errorDetails{Summary: message, Suggestion: suggestion, Detail: detail, ActionHint: actionHint}})
}

func (d *handlerDeps) handleSecurityEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if d.securityEvents == nil {
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: []securityEventRecord{}})
		return
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: d.securityEvents.List(200)})
}
