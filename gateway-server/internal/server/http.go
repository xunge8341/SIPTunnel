package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/observability"
	"siptunnel/internal/repository"
	filerepo "siptunnel/internal/repository/file"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/service"
	"siptunnel/internal/service/taskengine"
	"siptunnel/internal/startupsummary"
	"siptunnel/internal/tunnelmapping"
	"siptunnel/loadtest"
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

	mu        sync.RWMutex
	limits    OpsLimits
	routes    map[string]OpsRoute
	nodes     []OpsNode
	nodeStore nodeConfigStore
	mappings  tunnelMappingStore
	runtime   *mappingRuntimeManager

	nodeConfigSource string
	mappingSource    string

	lastLinkTest LinkTestReport
	tunnelConfig TunnelConfigPayload
	sessionMgr   *tunnelSessionManager
}

type nodeConfigStore interface {
	GetLocalNode() nodeconfig.LocalNodeConfig
	UpdateLocalNode(local nodeconfig.LocalNodeConfig) (nodeconfig.LocalNodeConfig, error)
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
	NodeIP        string `json:"node_ip"`
	SignalingPort int    `json:"signaling_port"`
	DeviceID      string `json:"device_id"`
	RTPPortStart  int    `json:"rtp_port_start,omitempty"`
	RTPPortEnd    int    `json:"rtp_port_end,omitempty"`
}

type NodeConfigPayload struct {
	LocalNode NodeConfigEndpoint `json:"local_node"`
	PeerNode  NodeConfigEndpoint `json:"peer_node"`
}

type TunnelConfigPayload struct {
	ChannelProtocol     string `json:"channel_protocol"`
	ConnectionInitiator string `json:"connection_initiator"`
	// Device IDs are derived from node configuration and are read-only in tunnel config.
	LocalDeviceID            string                  `json:"local_device_id"`
	PeerDeviceID             string                  `json:"peer_device_id"`
	HeartbeatIntervalSec     int                     `json:"heartbeat_interval_sec"`
	RegisterRetryCount       int                     `json:"register_retry_count"`
	RegisterRetryIntervalSec int                     `json:"register_retry_interval_sec"`
	RegistrationStatus       string                  `json:"registration_status"`
	LastRegisterTime         string                  `json:"last_register_time"`
	LastHeartbeatTime        string                  `json:"last_heartbeat_time"`
	HeartbeatStatus          string                  `json:"heartbeat_status"`
	LastFailureReason        string                  `json:"last_failure_reason"`
	NextRetryTime            string                  `json:"next_retry_time"`
	ConsecutiveHBTimeout     int                     `json:"consecutive_heartbeat_timeout"`
	SupportedCapabilities    []string                `json:"supported_capabilities"`
	RequestChannel           string                  `json:"request_channel"`
	ResponseChannel          string                  `json:"response_channel"`
	NetworkMode              config.NetworkMode      `json:"network_mode"`
	Capability               config.Capability       `json:"capability"`
	CapabilityItems          []config.CapabilityItem `json:"capability_items"`
}

type TunnelConfigUpdatePayload struct {
	ChannelProtocol          string             `json:"channel_protocol"`
	ConnectionInitiator      string             `json:"connection_initiator"`
	HeartbeatIntervalSec     int                `json:"heartbeat_interval_sec"`
	RegisterRetryCount       int                `json:"register_retry_count"`
	RegisterRetryIntervalSec int                `json:"register_retry_interval_sec"`
	RegistrationStatus       string             `json:"registration_status"`
	LastRegisterTime         string             `json:"last_register_time"`
	LastHeartbeatTime        string             `json:"last_heartbeat_time"`
	HeartbeatStatus          string             `json:"heartbeat_status"`
	NetworkMode              config.NetworkMode `json:"network_mode"`
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
	AuditDir               string
	DataDir                string
	SelfCheckProvider      func(context.Context) selfcheck.Report
	NetworkStatusFunc      func(context.Context) NodeNetworkStatus
	StartupSummaryProvider func(context.Context) startupsummary.Summary
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
		panic(err)
	}
	return h
}

func NewHandlerWithOptions(opts HandlerOptions) (http.Handler, io.Closer, error) {
	repo := memrepo.NewTaskRepository()
	engine := taskengine.NewEngine(repo, service.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second})
	logger := observability.NewStructuredLogger(nil)
	var logCloser io.Closer
	if opts.LogDir != "" {
		var err error
		logger, logCloser, err = observability.NewStructuredLoggerWithFile(opts.LogDir)
		if err != nil {
			return nil, nil, fmt.Errorf("init logger: %w", err)
		}
	}
	audit := observability.AuditStore(observability.NewInMemoryAuditStore())
	var auditCloser io.Closer
	if opts.AuditDir != "" {
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

	deps := handlerDeps{
		logger: logger,
		audit:  audit,
		repo:   repo,
		engine: engine,
		httpClient: &http.Client{
			Timeout: 1500 * time.Millisecond,
		},
		limits:            OpsLimits{RPS: 200, Burst: 400, MaxConcurrent: 100},
		routes:            map[string]OpsRoute{},
		mappings:          mappingStore,
		runtime:           newMappingRuntimeManager(nil),
		nodeStore:         nodeStore,
		nodeConfigSource:  dataSourceLabel(dataDir, "node_config.json"),
		mappingSource:     dataSourceLabel(dataDir, "tunnel_mappings.json"),
		selfCheckProvider: opts.SelfCheckProvider,
		networkStatusFunc: opts.NetworkStatusFunc,
		startupSummaryFn:  opts.StartupSummaryProvider,
		tunnelConfig:      defaultTunnelConfigPayload(config.DefaultNetworkMode()),
	}
	deps.sessionMgr = newTunnelSessionManager(tcpTunnelRegistrar{nodeStore: deps.nodeStore}, deps.tunnelConfig)
	deps.sessionMgr.Start()
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
	deps.runtime.SyncMappings(deps.mappings.List())
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

// newMux 集中注册运维 API；接口清单见 gateway-server/docs/openapi-ops.yaml，
// 排障动作与升级路径见 docs/runbook.md、docs/oncall-handbook.md。
func newMux(deps handlerDeps) http.Handler {
	if deps.runtime == nil {
		deps.runtime = newMappingRuntimeManager(nil)
		deps.runtime.SyncMappings(deps.mappings.List())
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", deps.healthz)
	mux.HandleFunc("/demo/process", deps.demoProcess)
	mux.HandleFunc("/audit/events", deps.listAuditEvents)
	mux.HandleFunc("/api/tasks", deps.handleTasks)
	mux.HandleFunc("/api/tasks/", deps.handleTaskByID)
	mux.HandleFunc("/api/limits", deps.handleLimits)
	mux.HandleFunc("/api/mappings", deps.handleMappings)
	mux.HandleFunc("/api/mappings/", deps.handleMappings)
	mux.HandleFunc("/api/mapping/test", deps.handleMappingTest)
	mux.HandleFunc("/api/routes", deps.handleRoutes)
	mux.HandleFunc("/api/nodes", deps.handleNodes)
	mux.HandleFunc("/api/audits", deps.handleAudits)
	mux.HandleFunc("/api/selfcheck", deps.handleSelfCheck)
	mux.HandleFunc("/api/startup-summary", deps.handleStartupSummary)
	mux.HandleFunc("/api/system/status", deps.handleSystemStatus)
	mux.HandleFunc("/api/node/network-status", deps.handleNodeNetworkStatus)
	mux.HandleFunc("/api/node", deps.handleNode)
	mux.HandleFunc("/api/node/config", deps.handleNodeConfig)
	mux.HandleFunc("/api/tunnel/config", deps.handleTunnelConfig)
	mux.HandleFunc("/api/tunnel/session/actions", deps.handleTunnelSessionActions)
	mux.HandleFunc("/api/peers", deps.handlePeers)
	mux.HandleFunc("/api/peers/", deps.handlePeers)
	mux.HandleFunc("/api/ops/link-test", deps.handleLinkTest)
	mux.HandleFunc("/api/diagnostics/export", deps.handleDiagnosticsExport)
	mux.HandleFunc("/api/capacity/recommendation", deps.handleCapacityRecommendation)
	return deps.withObservability(mux)
}

func (d *handlerDeps) handleLinkTest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		report := d.runLinkTest(r.Context())
		d.mu.Lock()
		d.lastLinkTest = report
		d.mu.Unlock()
		d.recordOpsAudit(r, readOperator(r), "RUN_LINK_TEST", map[string]any{"status": report.Status, "passed": report.Passed})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: report})
	case http.MethodGet:
		d.mu.RLock()
		report := d.lastLinkTest
		d.mu.RUnlock()
		if report.CheckedAt.IsZero() {
			writeError(w, http.StatusNotFound, "LINK_TEST_NOT_FOUND", "no link test report yet")
			return
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: report})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d handlerDeps) runLinkTest(ctx context.Context) LinkTestReport {
	started := time.Now()
	core := observability.CoreFieldsFromContext(ctx)
	status := d.networkStatusFunc(ctx)

	items := []LinkTestItem{d.checkSIPControlPath(ctx, status), d.checkRTPPortPool(status), d.checkHTTPMockReachability(ctx)}
	passed := true
	for _, item := range items {
		if !item.Passed {
			passed = false
			break
		}
	}

	report := LinkTestReport{
		Passed:     passed,
		Status:     map[bool]string{true: "passed", false: "failed"}[passed],
		RequestID:  core.RequestID,
		TraceID:    core.TraceID,
		DurationMS: time.Since(started).Milliseconds(),
		CheckedAt:  time.Now().UTC(),
		Items:      items,
		MockTarget: linkTestHTTPTarget(),
	}
	return report
}

func (d handlerDeps) checkSIPControlPath(ctx context.Context, status NodeNetworkStatus) LinkTestItem {
	start := time.Now()
	transport := strings.ToUpper(strings.TrimSpace(status.SIP.Transport))
	if transport == "" || status.SIP.ListenPort <= 0 {
		return LinkTestItem{Name: "sip_control", Passed: false, Status: "failed", Detail: "SIP 监听参数无效", DurationMS: time.Since(start).Milliseconds()}
	}
	if transport == "TCP" {
		host := strings.TrimSpace(status.SIP.ListenIP)
		if host == "" || host == "0.0.0.0" {
			host = "127.0.0.1"
		}
		addr := net.JoinHostPort(host, strconv.Itoa(status.SIP.ListenPort))
		dialer := &net.Dialer{Timeout: 600 * time.Millisecond}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return LinkTestItem{Name: "sip_control", Passed: false, Status: "failed", Detail: fmt.Sprintf("TCP 握手失败: %v", err), DurationMS: time.Since(start).Milliseconds()}
		}
		_ = conn.Close()
		return LinkTestItem{Name: "sip_control", Passed: true, Status: "passed", Detail: "SIP TCP 控制面握手成功（无业务载荷）", DurationMS: time.Since(start).Milliseconds()}
	}
	if len(status.RecentBindErrors) > 0 {
		return LinkTestItem{Name: "sip_control", Passed: false, Status: "failed", Detail: "发现 SIP 最近绑定错误，判定控制链路不可用", DurationMS: time.Since(start).Milliseconds()}
	}
	return LinkTestItem{Name: "sip_control", Passed: true, Status: "passed", Detail: "SIP UDP 采用监听状态与错误计数进行最小通路验证（无业务载荷）", DurationMS: time.Since(start).Milliseconds()}
}

func (d handlerDeps) checkRTPPortPool(status NodeNetworkStatus) LinkTestItem {
	start := time.Now()
	available := status.RTP.AvailablePorts
	if available <= 0 || status.RTP.PortPoolTotal <= 0 {
		return LinkTestItem{Name: "rtp_port_pool", Passed: false, Status: "failed", Detail: "RTP 端口池不可用或已耗尽", DurationMS: time.Since(start).Milliseconds()}
	}
	return LinkTestItem{Name: "rtp_port_pool", Passed: true, Status: "passed", Detail: fmt.Sprintf("RTP 端口池可用: %d/%d", available, status.RTP.PortPoolTotal), DurationMS: time.Since(start).Milliseconds()}
}

func (d handlerDeps) checkHTTPMockReachability(ctx context.Context) LinkTestItem {
	start := time.Now()
	target := linkTestHTTPTarget()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return LinkTestItem{Name: "http_downstream", Passed: false, Status: "failed", Detail: fmt.Sprintf("构建 HTTP mock 探测请求失败: %v", err), DurationMS: time.Since(start).Milliseconds()}
	}
	req.Header.Set("X-Link-Test", "true")
	req.Header.Set("X-Api-Code", "ops.link_test")
	client := d.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return LinkTestItem{Name: "http_downstream", Passed: false, Status: "failed", Detail: fmt.Sprintf("HTTP mock/downstream 不可达: %v", err), DurationMS: time.Since(start).Milliseconds()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return LinkTestItem{Name: "http_downstream", Passed: false, Status: "failed", Detail: fmt.Sprintf("HTTP mock/downstream 返回状态异常: %d", resp.StatusCode), DurationMS: time.Since(start).Milliseconds()}
	}
	return LinkTestItem{Name: "http_downstream", Passed: true, Status: "passed", Detail: fmt.Sprintf("HTTP mock/downstream 探测成功: %s", target), DurationMS: time.Since(start).Milliseconds()}
}

func linkTestHTTPTarget() string {
	target := strings.TrimSpace(os.Getenv("GATEWAY_LINK_TEST_HTTP_TARGET"))
	if target == "" {
		return "http://127.0.0.1:18080/healthz"
	}
	return target
}

func (d handlerDeps) withObservability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := observability.ExtractTraceContext(r)
		fields := observability.BuildCoreFieldsFromRequest(r)
		ctx = observability.WithCoreFields(ctx, fields)
		r = r.WithContext(ctx)
		d.logger.Info(ctx, "request_received", fields, "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func (d handlerDeps) healthz(w http.ResponseWriter, r *http.Request) {
	fields := observability.CoreFieldsFromContext(r.Context())
	fields.ResultCode = "OK"
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]string{"status": "ok"}})
	d.logger.Info(r.Context(), "healthz_ok", fields)
}

func (d handlerDeps) demoProcess(w http.ResponseWriter, r *http.Request) {
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

func (d handlerDeps) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	query := readAuditQuery(r)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			query.Limit = n
		}
	}

	events, err := d.audit.List(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "query audit failed")
		return
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"events": events}})
}

func (d handlerDeps) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	page, pageSize := parsePagination(r)
	filter := repository.TaskFilter{
		TaskType:     repository.TaskType(r.URL.Query().Get("task_type")),
		Status:       repository.TaskStatus(r.URL.Query().Get("status")),
		RequestID:    r.URL.Query().Get("request_id"),
		TraceID:      r.URL.Query().Get("trace_id"),
		SourceSystem: r.URL.Query().Get("source_system"),
		Offset:       (page - 1) * pageSize,
		Limit:        pageSize,
	}
	items, err := d.repo.ListTasks(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "list tasks failed")
		return
	}
	countFilter := filter
	countFilter.Offset = 0
	countFilter.Limit = 0
	all, err := d.repo.ListTasks(r.Context(), countFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "count tasks failed")
		return
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: listData[repository.Task]{Items: items, Pagination: pagination{Page: page, PageSize: pageSize, Total: len(all)}}})
}

func (d handlerDeps) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	taskID, action, ok := parseTaskPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "resource not found")
		return
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		task, err := d.repo.GetTaskByID(r.Context(), taskID)
		if err != nil {
			if errors.Is(err, repository.ErrTaskNotFound) {
				writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "get task failed")
			return
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: task})
	case action == "retry" && r.Method == http.MethodPost:
		d.performTaskAction(w, r, taskID, "retry")
	case action == "cancel" && r.Method == http.MethodPost:
		d.performTaskAction(w, r, taskID, "cancel")
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d handlerDeps) performTaskAction(w http.ResponseWriter, r *http.Request, taskID string, action string) {
	var req opsActionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Operator == "" {
		req.Operator = r.Header.Get("X-Initiator")
	}
	if req.Operator == "" {
		req.Operator = "system"
	}

	var updated repository.Task
	var err error
	switch action {
	case "retry":
		task, getErr := d.repo.GetTaskByID(r.Context(), taskID)
		if getErr != nil {
			err = getErr
			break
		}
		if task.Status == repository.TaskStatusDeadLettered {
			updated, err = d.engine.ReplayDeadLetter(r.Context(), taskID)
		} else {
			updated, err = d.engine.TransitTask(r.Context(), taskID, repository.TaskStatusRetryWait, "RETRY_REQUESTED", req.Reason)
		}
	case "cancel":
		updated, err = d.engine.TransitTask(r.Context(), taskID, repository.TaskStatusCancelled, "CANCELLED_BY_OPS", req.Reason)
	}
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotFound) {
			writeError(w, http.StatusNotFound, "TASK_NOT_FOUND", "task not found")
			return
		}
		writeError(w, http.StatusConflict, "TASK_ACTION_CONFLICT", err.Error())
		return
	}
	d.recordOpsAudit(r, req.Operator, fmt.Sprintf("TASK_%s", strings.ToUpper(action)), updated)
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: updated})
}

func (d handlerDeps) handleLimits(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		limits := d.limits
		d.mu.RUnlock()
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: limits})
	case http.MethodPut:
		var req updateLimitsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		if req.RPS <= 0 || req.Burst <= 0 || req.MaxConcurrent <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "limits must be positive")
			return
		}
		d.mu.Lock()
		d.limits = OpsLimits(req)
		updated := d.limits
		d.mu.Unlock()
		d.recordOpsAudit(r, readOperator(r), "UPDATE_LIMITS", updated)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: updated})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d handlerDeps) handleRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Deprecation", "true")
	w.Header().Set("Sunset", "legacy-ops-route")
	switch r.Method {
	case http.MethodGet:
		items := d.listLegacyRoutesFromMappings()
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": items}})
	case http.MethodPut:
		var req updateRoutesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		created := make([]TunnelMapping, 0, len(req.Routes))
		for _, route := range req.Routes {
			if route.APICode == "" || route.HTTPMethod == "" || route.HTTPPath == "" {
				writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "route fields are required")
				return
			}
			created = append(created, TunnelMapping{
				MappingID:            route.APICode,
				Name:                 route.APICode,
				Enabled:              route.Enabled,
				PeerNodeID:           "legacy-peer",
				LocalBindIP:          "127.0.0.1",
				LocalBindPort:        18080,
				LocalBasePath:        route.HTTPPath,
				RemoteTargetIP:       "127.0.0.1",
				RemoteTargetPort:     8080,
				RemoteBasePath:       route.HTTPPath,
				AllowedMethods:       []string{route.HTTPMethod},
				ConnectTimeoutMS:     500,
				RequestTimeoutMS:     3000,
				ResponseTimeoutMS:    3000,
				MaxRequestBodyBytes:  tunnelmapping.SmallBodyLimitBytes,
				MaxResponseBodyBytes: 20 * 1024 * 1024,
				Description:          "deprecated /api/routes compatibility mapping",
			})
		}
		validation := d.validateMappingsAgainstCapability(created)
		if validation.HasErrors() {
			writeError(w, http.StatusBadRequest, "MAPPING_CAPABILITY_INVALID", strings.Join(validation.Errors, "; "))
			return
		}
		for _, item := range d.mappings.List() {
			_ = d.mappings.Delete(item.MappingID)
		}
		for _, item := range created {
			if _, err := d.mappings.Create(item); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", fmt.Sprintf("save mapping failed: %v", err))
				return
			}
		}
		d.runtime.SyncMappings(d.mappings.List())
		d.recordOpsAudit(r, readOperator(r), "UPDATE_ROUTES", map[string]any{"count": len(req.Routes)})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": req.Routes, "warnings": validation.Warnings}})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d handlerDeps) handleMappings(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/mappings/")
	if r.URL.Path == "/api/mappings" || r.URL.Path == "/api/mappings/" {
		id = ""
	}
	switch r.Method {
	case http.MethodGet:
		if id != "" {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		items := d.mappings.List()
		validation := d.validateMappingsAgainstCapability(items)
		binding, bindErr := d.currentPeerBinding()
		resp := mappingListResponse{Items: d.decorateMappings(items), BoundPeer: binding, Warnings: validation.Warnings}
		if bindErr != nil {
			resp.BindingError = bindErr.Error()
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
	case http.MethodPost:
		if id != "" {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		var req TunnelMapping
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		req.Normalize()
		if err := d.enforceCurrentPeerBinding(&req); err != nil {
			writeError(w, http.StatusBadRequest, "PEER_BINDING_INVALID", err.Error())
			return
		}
		validation := d.validateMappingAgainstCapability(req)
		if validation.HasErrors() {
			writeError(w, http.StatusBadRequest, "MAPPING_CAPABILITY_INVALID", strings.Join(validation.Errors, "; "))
			return
		}
		created, err := d.mappings.Create(req)
		if err != nil {
			status := http.StatusBadRequest
			code := "INVALID_ARGUMENT"
			if errors.Is(err, filerepo.ErrMappingExists) {
				status = http.StatusConflict
				code = "MAPPING_EXISTS"
			}
			writeError(w, status, code, err.Error())
			return
		}
		d.runtime.SyncMappings(d.mappings.List())
		d.recordOpsAudit(r, readOperator(r), "CREATE_MAPPING", map[string]any{"mapping_id": created.MappingID})
		writeJSON(w, http.StatusCreated, responseEnvelope{Code: "OK", Message: "success", Data: MappingWithWarnings{Mapping: d.decorateMapping(created), BoundPeer: bindingFromMapping(created), Warnings: validation.Warnings}})
	case http.MethodPut:
		if id == "" {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "mapping id is required in path")
			return
		}
		var req TunnelMapping
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		req.Normalize()
		if err := d.enforceCurrentPeerBinding(&req); err != nil {
			writeError(w, http.StatusBadRequest, "PEER_BINDING_INVALID", err.Error())
			return
		}
		validation := d.validateMappingAgainstCapability(req)
		if validation.HasErrors() {
			writeError(w, http.StatusBadRequest, "MAPPING_CAPABILITY_INVALID", strings.Join(validation.Errors, "; "))
			return
		}
		updated, err := d.mappings.Update(id, req)
		if err != nil {
			status := http.StatusBadRequest
			code := "INVALID_ARGUMENT"
			if errors.Is(err, filerepo.ErrMappingNotFound) {
				status = http.StatusNotFound
				code = "MAPPING_NOT_FOUND"
			}
			writeError(w, status, code, err.Error())
			return
		}
		d.runtime.SyncMappings(d.mappings.List())
		d.recordOpsAudit(r, readOperator(r), "UPDATE_MAPPING", map[string]any{"mapping_id": updated.MappingID})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: MappingWithWarnings{Mapping: d.decorateMapping(updated), BoundPeer: bindingFromMapping(updated), Warnings: validation.Warnings}})
	case http.MethodDelete:
		if id == "" {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "mapping id is required in path")
			return
		}
		if err := d.mappings.Delete(id); err != nil {
			if errors.Is(err, filerepo.ErrMappingNotFound) {
				writeError(w, http.StatusNotFound, "MAPPING_NOT_FOUND", err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.runtime.SyncMappings(d.mappings.List())
		d.recordOpsAudit(r, readOperator(r), "DELETE_MAPPING", map[string]any{"mapping_id": id})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d handlerDeps) handleMappingTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	status := d.networkStatusFunc(r.Context())
	sip := d.checkSIPControlPath(r.Context(), status)
	rtp := d.checkRTPPortPool(status)
	localListeningPassed := sip.Passed && rtp.Passed
	session := d.sessionMgr.Snapshot()
	registrationNormal := strings.EqualFold(strings.TrimSpace(session.RegistrationStatus), "registered")
	heartbeatNormal := strings.EqualFold(strings.TrimSpace(session.HeartbeatStatus), "healthy")
	peerStage := d.checkPeerReachabilityStage(r.Context())
	sessionReady := registrationNormal && heartbeatNormal && peerStage.Passed
	forwardStage := d.checkMappingForwardReadinessStage(sessionReady)

	stages := []MappingTestStage{
		{Key: "local_listening", Name: "本地监听可用", Status: boolLabel(localListeningPassed, "passed", "failed"), Passed: localListeningPassed, Detail: fmt.Sprintf("SIP=%s；RTP=%s", sip.Detail, rtp.Detail), BlockingReason: firstNonEmpty(failedReason(sip), failedReason(rtp)), SuggestedAction: "检查本端 SIP 监听与 RTP 端口池配置。"},
		{Key: "registration", Name: "注册状态正常", Status: boolLabel(registrationNormal, "passed", "failed"), Passed: registrationNormal, Detail: fmt.Sprintf("当前注册状态：%s", normalizeValue(session.RegistrationStatus, "unknown")), BlockingReason: boolLabel(registrationNormal, "", normalizeValue(session.LastFailureReason, "注册尚未完成")), SuggestedAction: "检查鉴权参数并触发重新注册。"},
		{Key: "heartbeat", Name: "心跳状态正常", Status: boolLabel(heartbeatNormal, "passed", "failed"), Passed: heartbeatNormal, Detail: fmt.Sprintf("当前心跳状态：%s", normalizeValue(session.HeartbeatStatus, "unknown")), BlockingReason: boolLabel(heartbeatNormal, "", normalizeValue(session.LastFailureReason, "心跳未恢复健康")), SuggestedAction: "检查心跳周期、网络时延与丢包。"},
		peerStage,
		{Key: "session_ready", Name: "会话已准备", Status: ternary(sessionReady, "passed", "blocked"), Passed: sessionReady, Detail: "会话准备要求：注册正常 + 心跳正常 + 对端可达。", BlockingReason: blockingReasonsForSession(registrationNormal, heartbeatNormal, peerStage.Passed), SuggestedAction: "按前置阶段提示恢复会话条件后重试。"},
		forwardStage,
	}

	passed := allMappingStagesPassed(stages)
	result := MappingTestResponse{
		Passed:             passed,
		Status:             boolLabel(passed, "passed", "failed"),
		Stages:             stages,
		SignalingRequest:   boolLabel(localListeningPassed, "成功", "失败"),
		ResponseChannel:    boolLabel(rtp.Passed, "正常", "异常"),
		RegistrationStatus: boolLabel(registrationNormal, "正常", "未注册"),
	}

	if failed := firstFailedMappingStage(stages); failed != nil {
		result.FailureStage = failed.Name
		result.FailureReason = normalizeValue(failed.BlockingReason, failed.Detail)
		result.SuggestedAction = failed.SuggestedAction
	}

	d.recordOpsAudit(r, readOperator(r), "RUN_MAPPING_TEST", map[string]any{"status": result.Status, "failure_stage": result.FailureStage, "signaling_request": result.SignalingRequest, "response_channel": result.ResponseChannel, "registration_status": result.RegistrationStatus})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: result})
}

func (d handlerDeps) checkPeerReachabilityStage(ctx context.Context) MappingTestStage {
	binding, err := d.currentPeerBinding()
	if err != nil {
		return MappingTestStage{Key: "peer_reachability", Name: "对端可达", Status: "failed", Passed: false, Detail: "对端绑定检查失败", BlockingReason: err.Error(), SuggestedAction: "在对端配置页面保持且仅保持一个启用对端。"}
	}
	if strings.TrimSpace(binding.PeerSignalingIP) == "" || binding.PeerSignalingPort <= 0 {
		return MappingTestStage{Key: "peer_reachability", Name: "对端可达", Status: "failed", Passed: false, Detail: "对端信令地址未配置", BlockingReason: "peer_signaling_ip 或 peer_signaling_port 未配置", SuggestedAction: "补齐对端信令地址后再测试。"}
	}
	addr := net.JoinHostPort(strings.TrimSpace(binding.PeerSignalingIP), strconv.Itoa(binding.PeerSignalingPort))
	dialer := &net.Dialer{Timeout: 1200 * time.Millisecond}
	conn, dialErr := dialer.DialContext(ctx, "tcp", addr)
	if dialErr != nil {
		return MappingTestStage{Key: "peer_reachability", Name: "对端可达", Status: "failed", Passed: false, Detail: fmt.Sprintf("TCP 探测 %s 失败", addr), BlockingReason: dialErr.Error(), SuggestedAction: "检查对端进程、ACL 与路由。"}
	}
	_ = conn.Close()
	return MappingTestStage{Key: "peer_reachability", Name: "对端可达", Status: "passed", Passed: true, Detail: fmt.Sprintf("TCP 探测 %s 成功", addr)}
}

func (d handlerDeps) checkMappingForwardReadinessStage(sessionReady bool) MappingTestStage {
	if !sessionReady {
		return MappingTestStage{Key: "mapping_forward", Name: "映射转发准备就绪", Status: "blocked", Passed: false, Detail: "会话尚未准备完成，暂不执行转发准备判定", BlockingReason: "依赖阶段“会话已准备”未通过", SuggestedAction: "先恢复注册/心跳/对端可达后重试。"}
	}
	items := d.mappings.List()
	enabled := make([]TunnelMapping, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}
	if len(enabled) == 0 {
		return MappingTestStage{Key: "mapping_forward", Name: "映射转发准备就绪", Status: "failed", Passed: false, Detail: "未找到启用的映射规则", BlockingReason: "至少需要一个 enabled=true 的映射规则", SuggestedAction: "启用至少一个映射规则后再执行联调。"}
	}
	runtime := d.runtime.Snapshot()
	notReady := make([]string, 0)
	for _, item := range enabled {
		rs, ok := runtime[item.MappingID]
		if !ok {
			notReady = append(notReady, fmt.Sprintf("%s: 运行时状态缺失", item.MappingID))
			continue
		}
		if rs.State != mappingStateListening && rs.State != "connected" {
			notReady = append(notReady, fmt.Sprintf("%s: %s", item.MappingID, normalizeValue(rs.Reason, rs.State)))
		}
	}
	if len(notReady) > 0 {
		return MappingTestStage{Key: "mapping_forward", Name: "映射转发准备就绪", Status: "failed", Passed: false, Detail: fmt.Sprintf("共有 %d 条启用规则未就绪", len(notReady)), BlockingReason: strings.Join(notReady, "；"), SuggestedAction: "修复映射监听失败/中断后重试。"}
	}
	return MappingTestStage{Key: "mapping_forward", Name: "映射转发准备就绪", Status: "passed", Passed: true, Detail: fmt.Sprintf("%d 条启用规则已进入监听态", len(enabled))}
}

func allMappingStagesPassed(stages []MappingTestStage) bool {
	for _, stage := range stages {
		if !stage.Passed {
			return false
		}
	}
	return true
}

func firstFailedMappingStage(stages []MappingTestStage) *MappingTestStage {
	for _, stage := range stages {
		if !stage.Passed {
			item := stage
			return &item
		}
	}
	return nil
}

func failedReason(item LinkTestItem) string {
	if item.Passed {
		return ""
	}
	return item.Detail
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func normalizeValue(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func blockingReasonsForSession(registrationNormal, heartbeatNormal, peerReachable bool) string {
	reasons := make([]string, 0, 3)
	if !registrationNormal {
		reasons = append(reasons, "注册状态未就绪")
	}
	if !heartbeatNormal {
		reasons = append(reasons, "心跳状态未恢复")
	}
	if !peerReachable {
		reasons = append(reasons, "对端不可达")
	}
	if len(reasons) == 0 {
		return ""
	}
	return strings.Join(reasons, "；")
}

func ternary(ok bool, pass, fail string) string {
	if ok {
		return pass
	}
	return fail
}

func (d handlerDeps) listLegacyRoutesFromMappings() []OpsRoute {
	items := d.mappings.List()
	legacy := make([]OpsRoute, 0, len(items))
	for _, item := range items {
		method := "ANY"
		if len(item.AllowedMethods) > 0 {
			method = item.AllowedMethods[0]
		}
		legacy = append(legacy, OpsRoute{APICode: item.MappingID, HTTPMethod: method, HTTPPath: item.LocalBasePath, Enabled: item.Enabled})
	}
	return legacy
}

func (d handlerDeps) decorateMappings(items []TunnelMapping) []TunnelMappingView {
	runtime := d.runtime.Snapshot()
	out := make([]TunnelMappingView, 0, len(items))
	for _, item := range items {
		out = append(out, d.decorateMappingWithRuntime(item, runtime))
	}
	return out
}

func (d handlerDeps) decorateMapping(item TunnelMapping) TunnelMappingView {
	runtime := d.runtime.Snapshot()
	return d.decorateMappingWithRuntime(item, runtime)
}

func (d handlerDeps) decorateMappingWithRuntime(item TunnelMapping, runtime map[string]mappingRuntimeStatus) TunnelMappingView {
	view := TunnelMappingView{TunnelMapping: item}
	if rs, ok := runtime[item.MappingID]; ok {
		view.LinkStatus = rs.State
		view.LinkStatusText, view.StatusReason, view.SuggestedAction = mappingStatusDiagnosis(rs.State, rs.Reason)
		view.FailureReason = view.StatusReason
		return view
	}
	if item.Enabled {
		view.LinkStatus = mappingStateStartFailed
		view.LinkStatusText, view.StatusReason, view.SuggestedAction = mappingStatusDiagnosis(mappingStateStartFailed, "监听状态未知，请检查运行时管理器")
		view.FailureReason = view.StatusReason
		return view
	}
	view.LinkStatus = mappingStateDisabled
	view.LinkStatusText, view.StatusReason, view.SuggestedAction = mappingStatusDiagnosis(mappingStateDisabled, "映射未启用")
	view.FailureReason = view.StatusReason
	return view
}

func mappingStatusDiagnosis(state, reason string) (statusText, failureReason, suggestedAction string) {
	trimmedReason := strings.TrimSpace(reason)
	switch state {
	case mappingStateDisabled:
		return "未启用", "映射规则未启用。", "按需开启规则后再观察链路状态。"
	case mappingStateListening:
		return "监听中", "本端入口监听已建立，等待对端完成业务连接。", "可发起联调请求，确认信令请求与响应通道状态。"
	case "connected":
		return "已连接", "映射链路已连接。", "无需处理，持续观察心跳与延迟指标。"
	case mappingStateInterrupted:
		if trimmedReason == "" {
			trimmedReason = "监听线程异常中断。"
		}
		return "异常", trimmedReason, "查看节点状态与最近网络错误，恢复后重新启用映射规则。"
	case mappingStateStartFailed:
		if trimmedReason == "" {
			trimmedReason = "映射启动失败。"
		}
		return "启动失败", trimmedReason, "检查本端监听地址、端口占用与权限，再执行重启。"
	default:
		if trimmedReason == "" {
			trimmedReason = "映射链路状态异常。"
		}
		return "异常", trimmedReason, "检查注册、心跳与对端可达性，定位后再恢复流量。"
	}
}

func boolLabel(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func (d handlerDeps) handleCapacityRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var req capacityAssessmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
		return
	}
	if strings.TrimSpace(req.SummaryFile) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "summary_file is required")
		return
	}
	report, err := loadtest.LoadReportFromSummary(req.SummaryFile)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", fmt.Sprintf("read summary failed: %v", err))
		return
	}
	d.mu.RLock()
	limits := d.limits
	d.mu.RUnlock()
	status := d.networkStatusFunc(r.Context())
	current := loadtest.CapacityCurrentConfig{
		CommandMaxConcurrent:      fallbackPositive(req.Current.CommandMaxConcurrent, limits.MaxConcurrent),
		FileTransferMaxConcurrent: fallbackPositive(req.Current.FileTransferMaxConcurrent, status.RTP.ActiveTransfers),
		RTPPortPoolSize:           fallbackPositive(req.Current.RTPPortPoolSize, status.RTP.PortPoolTotal),
		MaxConnections:            fallbackPositive(req.Current.MaxConnections, status.SIP.MaxConnections),
		RateLimitRPS:              fallbackPositive(req.Current.RateLimitRPS, limits.RPS),
		RateLimitBurst:            fallbackPositive(req.Current.RateLimitBurst, limits.Burst),
	}
	assessment := loadtest.AssessCapacity(report, current)
	response := map[string]any{
		"assessment": assessment,
		"summary": map[string]any{
			"run_id":       report.RunID,
			"generated_at": report.Generated,
			"targets":      len(report.Summaries),
		},
	}
	d.recordOpsAudit(r, readOperator(r), "QUERY_CAPACITY_RECOMMENDATION", map[string]any{"summary_file": req.SummaryFile})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: response})
}

func resolveSinglePeerBinding(peers []nodeconfig.PeerNodeConfig) (*PeerBinding, error) {
	enabled := make([]nodeconfig.PeerNodeConfig, 0, len(peers))
	for _, peer := range peers {
		if peer.Enabled {
			enabled = append(enabled, peer)
		}
	}
	if len(enabled) == 0 {
		return nil, fmt.Errorf("no enabled peer node configured; configure exactly one peer node in /api/peers")
	}
	if len(enabled) > 1 {
		ids := make([]string, 0, len(enabled))
		for _, peer := range enabled {
			ids = append(ids, peer.PeerNodeID)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("multiple enabled peer nodes configured (%s); current single-binding mode requires exactly one", strings.Join(ids, ","))
	}
	peer := enabled[0]
	return &PeerBinding{PeerNodeID: peer.PeerNodeID, PeerName: peer.PeerName, PeerSignalingIP: peer.PeerSignalingIP, PeerSignalingPort: peer.PeerSignalingPort}, nil
}

func (d handlerDeps) currentPeerBinding() (*PeerBinding, error) {
	if d.nodeStore == nil {
		return nil, fmt.Errorf("node config store not configured")
	}
	return resolveSinglePeerBinding(d.nodeStore.ListPeers())
}

func (d handlerDeps) enforceCurrentPeerBinding(mapping *TunnelMapping) error {
	if mapping == nil {
		return fmt.Errorf("mapping is required")
	}
	binding, err := d.currentPeerBinding()
	if err != nil {
		return err
	}
	mapping.PeerNodeID = binding.PeerNodeID
	return nil
}

func bindingFromMapping(mapping TunnelMapping) *PeerBinding {
	if strings.TrimSpace(mapping.PeerNodeID) == "" {
		return nil
	}
	return &PeerBinding{PeerNodeID: mapping.PeerNodeID}
}

func fallbackPositive(v int, fallback int) int {
	if v > 0 {
		return v
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func (d handlerDeps) compatibilitySnapshot(ctx context.Context) nodeconfig.CompatibilityStatus {
	status := d.networkStatusFunc(ctx)
	local := d.nodeStore.GetLocalNode()
	peers := d.nodeStore.ListPeers()
	return nodeconfig.EvaluateCompatibility(local, peers, status.NetworkMode, status.Capability)
}

func (d handlerDeps) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if d.nodeStore == nil {
		writeError(w, http.StatusNotImplemented, "NODE_STORE_NOT_ENABLED", "node config store not configured")
		return
	}
	local := d.nodeStore.GetLocalNode()
	nodeSource := d.nodeConfigSource
	if strings.TrimSpace(nodeSource) == "" {
		nodeSource = dataSourceLabel("", "node_config.json")
	}
	node := OpsNode{
		NodeID:     local.NodeID,
		Role:       local.NodeRole,
		Status:     "configured",
		Endpoint:   net.JoinHostPort(local.SIPListenIP, strconv.Itoa(local.SIPListenPort)),
		DataSource: nodeSource,
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": []OpsNode{node}}})
}

func (d handlerDeps) handleNode(w http.ResponseWriter, r *http.Request) {
	if d.nodeStore == nil {
		writeError(w, http.StatusNotImplemented, "NODE_STORE_NOT_ENABLED", "node config store not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		compat := d.compatibilitySnapshot(r.Context())
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: NodeDetailResponse{LocalNode: d.nodeStore.GetLocalNode(), CurrentNetworkMode: compat.CurrentNetworkMode, CurrentCapability: compat.CurrentCapability, CompatibilityStatus: compat.CompatibilityCheck}})
	case http.MethodPut:
		var req nodeconfig.LocalNodeConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		status := d.networkStatusFunc(r.Context())
		if req.NetworkMode.Normalize() != status.NetworkMode.Normalize() {
			writeError(w, http.StatusBadRequest, "NETWORK_MODE_MISMATCH", fmt.Sprintf("local node network_mode=%s must match current network_mode=%s", req.NetworkMode.Normalize(), status.NetworkMode.Normalize()))
			return
		}
		updated, err := d.nodeStore.UpdateLocalNode(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_LOCAL_NODE", updated)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: updated})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d handlerDeps) handleNodeConfig(w http.ResponseWriter, r *http.Request) {
	if d.nodeStore == nil {
		writeError(w, http.StatusNotImplemented, "NODE_STORE_NOT_ENABLED", "node config store not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		local := d.nodeStore.GetLocalNode()
		payload := NodeConfigPayload{LocalNode: NodeConfigEndpoint{NodeIP: local.SIPListenIP, SignalingPort: local.SIPListenPort, DeviceID: local.NodeID, RTPPortStart: local.RTPPortStart, RTPPortEnd: local.RTPPortEnd}}
		peers := d.nodeStore.ListPeers()
		if len(peers) > 0 {
			peer := peers[0]
			payload.PeerNode = NodeConfigEndpoint{NodeIP: peer.PeerSignalingIP, SignalingPort: peer.PeerSignalingPort, DeviceID: peer.PeerNodeID}
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: payload})
	case http.MethodPost:
		var req NodeConfigPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		if strings.TrimSpace(req.LocalNode.NodeIP) == "" || strings.TrimSpace(req.LocalNode.DeviceID) == "" || req.LocalNode.SignalingPort <= 0 || req.LocalNode.RTPPortStart <= 0 || req.LocalNode.RTPPortEnd <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "local_node fields are required")
			return
		}
		if strings.TrimSpace(req.PeerNode.NodeIP) == "" || strings.TrimSpace(req.PeerNode.DeviceID) == "" || req.PeerNode.SignalingPort <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "peer_node fields are required")
			return
		}

		local := d.nodeStore.GetLocalNode()
		local.NodeID = strings.TrimSpace(req.LocalNode.DeviceID)
		local.NodeName = local.NodeID
		local.SIPListenIP = strings.TrimSpace(req.LocalNode.NodeIP)
		local.RTPListenIP = local.SIPListenIP
		local.SIPListenPort = req.LocalNode.SignalingPort
		local.RTPPortStart = req.LocalNode.RTPPortStart
		local.RTPPortEnd = req.LocalNode.RTPPortEnd
		updatedLocal, err := d.nodeStore.UpdateLocalNode(local)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}

		peerID := strings.TrimSpace(req.PeerNode.DeviceID)
		peerCfg := nodeconfig.PeerNodeConfig{
			PeerNodeID:           peerID,
			PeerName:             peerID,
			PeerSignalingIP:      strings.TrimSpace(req.PeerNode.NodeIP),
			PeerSignalingPort:    req.PeerNode.SignalingPort,
			PeerMediaIP:          strings.TrimSpace(req.PeerNode.NodeIP),
			PeerMediaPortStart:   req.LocalNode.RTPPortStart,
			PeerMediaPortEnd:     req.LocalNode.RTPPortEnd,
			SupportedNetworkMode: updatedLocal.NetworkMode,
			Enabled:              true,
		}

		existingPeers := d.nodeStore.ListPeers()
		found := false
		for _, item := range existingPeers {
			if item.PeerNodeID == peerID {
				found = true
				break
			}
		}
		if found {
			if _, err := d.nodeStore.UpdatePeer(peerCfg); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
				return
			}
		} else {
			if _, err := d.nodeStore.CreatePeer(peerCfg); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
				return
			}
		}

		resp := NodeConfigPayload{
			LocalNode: NodeConfigEndpoint{NodeIP: updatedLocal.SIPListenIP, SignalingPort: updatedLocal.SIPListenPort, DeviceID: updatedLocal.NodeID, RTPPortStart: updatedLocal.RTPPortStart, RTPPortEnd: updatedLocal.RTPPortEnd},
			PeerNode:  NodeConfigEndpoint{NodeIP: peerCfg.PeerSignalingIP, SignalingPort: peerCfg.PeerSignalingPort, DeviceID: peerCfg.PeerNodeID},
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_NODE_CONFIG_AND_RESTART_TUNNEL", resp)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "节点配置已保存并重启隧道", Data: map[string]any{"config": resp, "tunnel_restarted": true}})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func defaultTunnelConfigPayload(mode config.NetworkMode) TunnelConfigPayload {
	normalized := mode.Normalize()
	capability := config.DeriveCapability(normalized)
	now := time.Now().UTC().Format(time.RFC3339)
	return TunnelConfigPayload{
		ChannelProtocol:          "GB/T 28181",
		ConnectionInitiator:      "LOCAL",
		HeartbeatIntervalSec:     60,
		RegisterRetryCount:       3,
		RegisterRetryIntervalSec: 10,
		RegistrationStatus:       "unregistered",
		LastRegisterTime:         "",
		LastHeartbeatTime:        now,
		HeartbeatStatus:          "unknown",
		SupportedCapabilities:    capabilityDescriptions(capability),
		RequestChannel:           "SIP",
		ResponseChannel:          "RTP",
		NetworkMode:              normalized,
		Capability:               capability,
		CapabilityItems:          capability.Matrix(),
	}
}

func capabilityDescriptions(capability config.Capability) []string {
	desc := make([]string, 0, 6)
	if capability.SupportsSmallRequestBody {
		desc = append(desc, "支持小请求体（典型 SIP JSON 负载）")
	}
	if capability.SupportsLargeRequestBody {
		desc = append(desc, "支持大请求体上传")
	}
	if capability.SupportsLargeResponseBody {
		desc = append(desc, "支持大响应体回传")
	}
	if capability.SupportsStreamingResponse {
		desc = append(desc, "支持流式响应")
	}
	if capability.SupportsBidirectionalHTTPTunnel {
		desc = append(desc, "支持双向 HTTP 隧道")
	}
	if capability.SupportsTransparentHTTPProxy {
		desc = append(desc, "支持透明代理")
	}
	if len(desc) == 0 {
		desc = append(desc, "当前网络模式下暂无可用扩展能力")
	}
	return desc
}

func (d *handlerDeps) upsertTunnelConfig(req TunnelConfigUpdatePayload) (TunnelConfigPayload, error) {
	channelProtocol := strings.ToUpper(strings.TrimSpace(req.ChannelProtocol))
	connectionInitiator := strings.ToUpper(strings.TrimSpace(req.ConnectionInitiator))
	if channelProtocol == "" {
		return TunnelConfigPayload{}, fmt.Errorf("channel_protocol is required")
	}
	if connectionInitiator != "LOCAL" && connectionInitiator != "PEER" {
		return TunnelConfigPayload{}, fmt.Errorf("connection_initiator must be LOCAL or PEER")
	}
	if req.HeartbeatIntervalSec <= 0 {
		return TunnelConfigPayload{}, fmt.Errorf("heartbeat_interval_sec must be greater than 0")
	}
	if req.RegisterRetryCount < 0 {
		return TunnelConfigPayload{}, fmt.Errorf("register_retry_count must be greater than or equal to 0")
	}
	if req.RegisterRetryIntervalSec <= 0 {
		return TunnelConfigPayload{}, fmt.Errorf("register_retry_interval_sec must be greater than 0")
	}
	mode := req.NetworkMode.Normalize()
	if err := mode.Validate(); err != nil {
		return TunnelConfigPayload{}, err
	}
	capability := config.DeriveCapability(mode)
	session := d.sessionMgr.Snapshot()
	updated := TunnelConfigPayload{
		ChannelProtocol:          channelProtocol,
		ConnectionInitiator:      connectionInitiator,
		HeartbeatIntervalSec:     req.HeartbeatIntervalSec,
		RegisterRetryCount:       req.RegisterRetryCount,
		RegisterRetryIntervalSec: req.RegisterRetryIntervalSec,
		RegistrationStatus:       session.RegistrationStatus,
		LastRegisterTime:         session.LastRegisterTime,
		LastHeartbeatTime:        session.LastHeartbeatTime,
		HeartbeatStatus:          session.HeartbeatStatus,
		LastFailureReason:        session.LastFailureReason,
		NextRetryTime:            session.NextRetryTime,
		ConsecutiveHBTimeout:     session.ConsecutiveHeartbeatTimeout,
		SupportedCapabilities:    capabilityDescriptions(capability),
		RequestChannel:           "SIP",
		ResponseChannel:          "RTP",
		NetworkMode:              mode,
		Capability:               capability,
		CapabilityItems:          capability.Matrix(),
	}
	d.mu.Lock()
	d.tunnelConfig = updated
	d.mu.Unlock()
	return updated, nil
}

func (d handlerDeps) derivedLocalDeviceID() string {
	if d.nodeStore == nil {
		return ""
	}
	return strings.TrimSpace(d.nodeStore.GetLocalNode().NodeID)
}

func (d handlerDeps) derivedPeerDeviceID() string {
	if d.nodeStore == nil {
		return ""
	}
	peers := d.nodeStore.ListPeers()
	if len(peers) == 0 {
		return ""
	}
	return strings.TrimSpace(peers[0].PeerNodeID)
}

func (d *handlerDeps) handleTunnelConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		resp := d.tunnelConfig
		d.mu.RUnlock()
		if resp.NetworkMode.Normalize() == "" {
			resp = defaultTunnelConfigPayload(config.DefaultNetworkMode())
		}
		session := d.sessionMgr.Snapshot()
		resp.RegistrationStatus = session.RegistrationStatus
		resp.HeartbeatStatus = session.HeartbeatStatus
		resp.LastRegisterTime = session.LastRegisterTime
		resp.LastHeartbeatTime = session.LastHeartbeatTime
		resp.LastFailureReason = session.LastFailureReason
		resp.NextRetryTime = session.NextRetryTime
		resp.ConsecutiveHBTimeout = session.ConsecutiveHeartbeatTimeout
		resp.LocalDeviceID = d.derivedLocalDeviceID()
		resp.PeerDeviceID = d.derivedPeerDeviceID()
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
	case http.MethodPost:
		var req TunnelConfigUpdatePayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		updated, err := d.upsertTunnelConfig(req)
		if err == nil {
			updated.LocalDeviceID = d.derivedLocalDeviceID()
			updated.PeerDeviceID = d.derivedPeerDeviceID()
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.sessionMgr.ApplyConfig(updated)
		d.recordOpsAudit(r, readOperator(r), "UPDATE_TUNNEL_CONFIG", updated)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: updated})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleTunnelSessionActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var req tunnelSessionActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "register_now":
		d.sessionMgr.TriggerRegister()
	case "reregister":
		d.sessionMgr.TriggerReregister()
	case "heartbeat_once":
		d.sessionMgr.TriggerHeartbeat()
	default:
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "action must be register_now, reregister or heartbeat_once")
		return
	}
	state := d.sessionMgr.Snapshot()
	d.recordOpsAudit(r, readOperator(r), "TUNNEL_SESSION_ACTION", map[string]any{"action": action, "state": state})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: tunnelSessionActionResponse{Action: action, State: state}})
}

func (d handlerDeps) handlePeers(w http.ResponseWriter, r *http.Request) {
	if d.nodeStore == nil {
		writeError(w, http.StatusNotImplemented, "NODE_STORE_NOT_ENABLED", "node config store not configured")
		return
	}
	peerID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/peers/"))
	hasID := peerID != "" && r.URL.Path != "/api/peers"
	switch r.Method {
	case http.MethodGet:
		if hasID {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET /api/peers/{id} not supported; use GET /api/peers")
			return
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": d.nodeStore.ListPeers()}})
	case http.MethodPost:
		if hasID {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "POST must target /api/peers")
			return
		}
		var req nodeconfig.PeerNodeConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		status := d.networkStatusFunc(r.Context())
		if req.SupportedNetworkMode.Normalize() != status.NetworkMode.Normalize() {
			writeError(w, http.StatusBadRequest, "PEER_NETWORK_MODE_INCOMPATIBLE", fmt.Sprintf("peer supported_network_mode=%s incompatible with current_network_mode=%s", req.SupportedNetworkMode.Normalize(), status.NetworkMode.Normalize()))
			return
		}
		created, err := d.nodeStore.CreatePeer(req)
		if err != nil {
			code := http.StatusBadRequest
			errCode := "INVALID_ARGUMENT"
			if errors.Is(err, filerepo.ErrPeerAlreadyExists) {
				code = http.StatusConflict
				errCode = "PEER_ALREADY_EXISTS"
			}
			writeError(w, code, errCode, err.Error())
			return
		}
		d.recordOpsAudit(r, readOperator(r), "CREATE_PEER_NODE", created)
		writeJSON(w, http.StatusCreated, responseEnvelope{Code: "OK", Message: "success", Data: created})
	case http.MethodPut:
		if !hasID {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "PUT must target /api/peers/{peer_node_id}")
			return
		}
		var req nodeconfig.PeerNodeConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		req.PeerNodeID = peerID
		status := d.networkStatusFunc(r.Context())
		if req.SupportedNetworkMode.Normalize() != status.NetworkMode.Normalize() {
			writeError(w, http.StatusBadRequest, "PEER_NETWORK_MODE_INCOMPATIBLE", fmt.Sprintf("peer supported_network_mode=%s incompatible with current_network_mode=%s", req.SupportedNetworkMode.Normalize(), status.NetworkMode.Normalize()))
			return
		}
		updated, err := d.nodeStore.UpdatePeer(req)
		if err != nil {
			code := http.StatusBadRequest
			errCode := "INVALID_ARGUMENT"
			if errors.Is(err, filerepo.ErrPeerNotFound) {
				code = http.StatusNotFound
				errCode = "PEER_NOT_FOUND"
			}
			writeError(w, code, errCode, err.Error())
			return
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_PEER_NODE", updated)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: updated})
	case http.MethodDelete:
		if !hasID {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "DELETE must target /api/peers/{peer_node_id}")
			return
		}
		if err := d.nodeStore.DeletePeer(peerID); err != nil {
			if errors.Is(err, filerepo.ErrPeerNotFound) {
				writeError(w, http.StatusNotFound, "PEER_NOT_FOUND", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		d.recordOpsAudit(r, readOperator(r), "DELETE_PEER_NODE", map[string]string{"peer_node_id": peerID})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]string{"peer_node_id": peerID}})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d handlerDeps) handleAudits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	page, pageSize := parsePagination(r)
	query := readAuditQuery(r)
	allQuery := query
	allQuery.Limit = 10000
	all, err := d.audit.List(r.Context(), allQuery)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "query audit failed")
		return
	}
	start := (page - 1) * pageSize
	if start > len(all) {
		start = len(all)
	}
	end := len(all)
	if start+pageSize < end {
		end = start + pageSize
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: listData[observability.AuditEvent]{Items: all[start:end], Pagination: pagination{Page: page, PageSize: pageSize, Total: len(all)}}})
}

func (d handlerDeps) recordOpsAudit(r *http.Request, operator string, action string, payload any) {
	fields := observability.CoreFieldsFromContext(r.Context())
	fields.ResultCode = "OK"
	_ = observability.NewAuditLogger(d.logger, d.audit).Record(r.Context(), observability.AuditEvent{
		Who:               operator,
		When:              time.Now().UTC(),
		RequestType:       "ops",
		ValidationPassed:  true,
		LocalServiceRoute: "gateway-server",
		FinalResult:       "OK",
		OpsAction:         action,
		Core:              fields,
	})
	_ = payload
}

func readAuditQuery(r *http.Request) observability.AuditQuery {
	query := observability.AuditQuery{
		RequestID: r.URL.Query().Get("request_id"),
		APICode:   r.URL.Query().Get("api_code"),
		Who:       r.URL.Query().Get("who"),
		TraceID:   r.URL.Query().Get("trace_id"),
		Rule:      strings.TrimSpace(r.URL.Query().Get("rule")),
	}
	query.ErrorOnly = parseBoolFlag(r.URL.Query().Get("error_only"))
	query.StartTime = parseRFC3339Time(r.URL.Query().Get("start_time"))
	query.EndTime = parseRFC3339Time(r.URL.Query().Get("end_time"))
	return query
}

func parseBoolFlag(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	return raw == "1" || raw == "true" || raw == "yes"
}

func parseRFC3339Time(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func parsePagination(r *http.Request) (int, int) {
	page := parseIntDefault(r.URL.Query().Get("page"), 1)
	pageSize := parseIntDefault(r.URL.Query().Get("page_size"), 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func parseTaskPath(path string) (taskID string, action string, ok bool) {
	trimmed := strings.TrimPrefix(path, "/api/tasks/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" {
		if parts[1] == "retry" || parts[1] == "cancel" {
			return parts[0], parts[1], true
		}
	}
	return "", "", false
}

func parseIntDefault(raw string, dv int) int {
	if raw == "" {
		return dv
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return dv
	}
	return v
}

func readOperator(r *http.Request) string {
	if op := r.Header.Get("X-Initiator"); op != "" {
		return op
	}
	return "system"
}

func (d handlerDeps) handleSelfCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if d.selfCheckProvider == nil {
		writeError(w, http.StatusNotImplemented, "SELF_CHECK_NOT_ENABLED", "self-check provider not configured")
		return
	}
	report := d.selfCheckProvider(r.Context())
	if d.nodeStore != nil {
		compat := d.compatibilitySnapshot(r.Context())
		report.Items = append(report.Items,
			selfcheck.Item{Name: "local_node_config_valid", Level: selfcheck.Level(compat.LocalNodeCheck.Level), Message: compat.LocalNodeCheck.Message, Suggestion: compat.LocalNodeCheck.Suggestion, ActionHint: compat.LocalNodeCheck.ActionHint},
			selfcheck.Item{Name: "peer_node_config_valid", Level: selfcheck.Level(compat.PeerNodeCheck.Level), Message: compat.PeerNodeCheck.Message, Suggestion: compat.PeerNodeCheck.Suggestion, ActionHint: compat.PeerNodeCheck.ActionHint},
			selfcheck.Item{Name: "network_mode_compatibility", Level: selfcheck.Level(compat.CompatibilityCheck.Level), Message: compat.CompatibilityCheck.Message, Suggestion: compat.CompatibilityCheck.Suggestion, ActionHint: compat.CompatibilityCheck.ActionHint},
		)
	}
	mappingValidation := d.validateMappingsAgainstCapability(d.mappings.List())
	mappingLevel := selfcheck.LevelInfo
	mappingMessage := "all mappings are compatible with current capability"
	mappingSuggestion := "继续按当前 network_mode 维护映射配置"
	mappingHint := "每次变更后复核 /api/mappings 与 /api/selfcheck。"
	if mappingValidation.HasErrors() {
		mappingLevel = selfcheck.LevelError
		mappingMessage = strings.Join(mappingValidation.Errors, "; ")
		mappingSuggestion = "调整 mapping 参数（body 限制/方法/流式要求）或切换 network_mode。"
		mappingHint = "修复后重新执行 /api/selfcheck。"
	} else if len(mappingValidation.Warnings) > 0 {
		mappingLevel = selfcheck.LevelWarn
		mappingMessage = strings.Join(mappingValidation.Warnings, "; ")
		mappingSuggestion = "建议收敛 mapping 配置以降低在受限模式下的不稳定风险。"
		mappingHint = "根据 warnings 逐条确认并保留运行记录。"
	}
	report.Items = append(report.Items, selfcheck.Item{Name: "mappings_capability_validation", Level: mappingLevel, Message: mappingMessage, Suggestion: mappingSuggestion, ActionHint: mappingHint})
	if d.nodeStore != nil {
		if binding, err := d.currentPeerBinding(); err != nil {
			report.Items = append(report.Items, selfcheck.Item{Name: "mapping_peer_binding", Level: selfcheck.LevelError, Message: err.Error(), Suggestion: "在 peer 配置页保持仅一个启用的对端节点", ActionHint: "新增/禁用 peer 后重新执行 /api/selfcheck。"})
		} else {
			report.Items = append(report.Items, selfcheck.Item{Name: "mapping_peer_binding", Level: selfcheck.LevelInfo, Message: fmt.Sprintf("mappings are bound to peer %s (%s:%d)", binding.PeerNodeID, binding.PeerSignalingIP, binding.PeerSignalingPort), Suggestion: "映射规则默认绑定该对端节点（只读）", ActionHint: "如需切换，请先在 peer 配置页调整唯一启用对端。"})
		}
	}
	if d.nodeStore != nil {
		report.Overall, report.Summary = summarizeSelfCheckItems(report.Items)
	}
	if level := strings.TrimSpace(r.URL.Query().Get("level")); level != "" {
		report.Items = filterSelfCheckItemsByLevel(report.Items, level)
		report.Overall, report.Summary = summarizeSelfCheckItems(report.Items)
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: report})
}

func filterSelfCheckItemsByLevel(items []selfcheck.Item, raw string) []selfcheck.Item {
	allowed := map[selfcheck.Level]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		l := selfcheck.Level(strings.ToLower(strings.TrimSpace(part)))
		if l == selfcheck.LevelInfo || l == selfcheck.LevelWarn || l == selfcheck.LevelError {
			allowed[l] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return items
	}
	out := make([]selfcheck.Item, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.Level]; ok {
			out = append(out, item)
		}
	}
	return out
}

func summarizeSelfCheckItems(items []selfcheck.Item) (selfcheck.Level, selfcheck.Summary) {
	summary := selfcheck.Summary{}
	overall := selfcheck.LevelInfo
	for _, item := range items {
		switch item.Level {
		case selfcheck.LevelError:
			summary.Error++
			overall = selfcheck.LevelError
		case selfcheck.LevelWarn:
			summary.Warn++
			if overall != selfcheck.LevelError {
				overall = selfcheck.LevelWarn
			}
		default:
			summary.Info++
		}
	}
	return overall, summary
}

func (d handlerDeps) handleNodeNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	status := d.networkStatusFunc(r.Context())
	if d.nodeStore != nil {
		compat := d.compatibilitySnapshot(r.Context())
		status.CurrentNetworkMode = compat.CurrentNetworkMode
		status.CurrentCapability = compat.CurrentCapability
		status.CompatibilityStatus = compat.CompatibilityCheck
		binding, bindErr := d.currentPeerBinding()
		status.BoundPeer = binding
		if bindErr != nil {
			status.PeerBindingError = bindErr.Error()
		}
	}
	d.recordOpsAudit(r, readOperator(r), "QUERY_NODE_NETWORK_STATUS", map[string]any{"path": r.URL.Path})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: status})
}

func (d handlerDeps) handleStartupSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if d.startupSummaryFn == nil {
		writeError(w, http.StatusNotImplemented, "STARTUP_SUMMARY_NOT_ENABLED", "startup summary provider not configured")
		return
	}
	summary := d.startupSummaryFn(r.Context())
	nodeSource := d.nodeConfigSource
	if strings.TrimSpace(nodeSource) == "" {
		nodeSource = dataSourceLabel("", "node_config.json")
	}
	mappingSource := d.mappingSource
	if strings.TrimSpace(mappingSource) == "" {
		mappingSource = dataSourceLabel("", "tunnel_mappings.json")
	}
	if strings.TrimSpace(summary.DataSources.NodeConfig) == "" {
		summary.DataSources.NodeConfig = nodeSource
	}
	if strings.TrimSpace(summary.DataSources.Peers) == "" {
		summary.DataSources.Peers = nodeSource
	}
	if strings.TrimSpace(summary.DataSources.Mappings) == "" {
		summary.DataSources.Mappings = mappingSource
	}
	if strings.TrimSpace(summary.DataSources.Mode) == "" {
		summary.DataSources.Mode = "runtime_network_config"
	}
	if strings.TrimSpace(summary.DataSources.Capability) == "" {
		summary.DataSources.Capability = "derived_from_network_mode"
	}
	session := d.sessionMgr.Snapshot()
	summary.RegistrationStatus = session.RegistrationStatus
	summary.HeartbeatStatus = session.HeartbeatStatus
	summary.LastRegisterTime = session.LastRegisterTime
	summary.LastHeartbeatTime = session.LastHeartbeatTime
	summary.LastFailureReason = session.LastFailureReason
	summary.NextRetryTime = session.NextRetryTime
	if d.nodeStore != nil {
		compat := d.compatibilitySnapshot(r.Context())
		summary.CurrentNetworkMode = compat.CurrentNetworkMode
		summary.CurrentCapability = compat.CurrentCapability
		summary.CompatibilityStatus = compat.CompatibilityCheck
		binding, bindErr := d.currentPeerBinding()
		if binding != nil {
			summary.BoundPeer = &startupsummary.PeerBinding{PeerNodeID: binding.PeerNodeID, PeerName: binding.PeerName, PeerSignalingIP: binding.PeerSignalingIP, PeerSignalingPort: binding.PeerSignalingPort}
		}
		if bindErr != nil {
			summary.PeerBindingError = bindErr.Error()
		}
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: summary})
}

func deriveTunnelStatus(status NodeNetworkStatus) (string, string) {
	if status.SIP.ListenPort <= 0 || status.RTP.PortStart <= 0 || status.RTP.PortEnd <= 0 {
		return "disconnected", "SIP 或 RTP 监听未就绪"
	}
	if status.RTP.AvailablePorts <= 0 {
		return "degraded", "RTP 端口池已耗尽"
	}
	if len(status.RecentNetworkErrors) > 0 {
		return "degraded", status.RecentNetworkErrors[0]
	}
	return "connected", "SIP 控制面与 RTP 文件面链路正常"
}

func (d handlerDeps) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	status := d.networkStatusFunc(r.Context())
	tunnelStatus, reason := deriveTunnelStatus(status)
	items := d.decorateMappings(d.mappings.List())
	mappingTotal := len(items)
	mappingAbnormalTotal := 0
	latestMappingErrorReason := ""
	for _, item := range items {
		if item.LinkStatus == mappingStateStartFailed || item.LinkStatus == mappingStateInterrupted {
			mappingAbnormalTotal++
			if latestMappingErrorReason == "" {
				latestMappingErrorReason = fmt.Sprintf("%s：%s", item.MappingID, item.StatusReason)
			}
		}
	}
	if status.PeerBindingError != "" {
		mappingAbnormalTotal = mappingTotal
		latestMappingErrorReason = status.PeerBindingError
	} else if tunnelStatus != "connected" && mappingAbnormalTotal == 0 {
		mappingAbnormalTotal = mappingTotal
		latestMappingErrorReason = reason
	}
	session := d.sessionMgr.Snapshot()
	resp := SystemStatusResponse{
		TunnelStatus:             tunnelStatus,
		ConnectionReason:         reason,
		NetworkMode:              status.NetworkMode,
		RegistrationStatus:       session.RegistrationStatus,
		HeartbeatStatus:          session.HeartbeatStatus,
		LastRegisterTime:         session.LastRegisterTime,
		LastHeartbeatTime:        session.LastHeartbeatTime,
		LastFailureReason:        session.LastFailureReason,
		NextRetryTime:            session.NextRetryTime,
		MappingTotal:             mappingTotal,
		MappingAbnormalTotal:     mappingAbnormalTotal,
		LatestMappingErrorReason: latestMappingErrorReason,
		BoundPeer:                status.BoundPeer,
		PeerBindingError:         status.PeerBindingError,
		Capability: SystemStatusCapability{
			SupportsSmallRequestBody:        status.Capability.SupportsSmallRequestBody,
			SupportsLargeResponseBody:       status.Capability.SupportsLargeResponseBody,
			SupportsStreamingResponse:       status.Capability.SupportsStreamingResponse,
			SupportsLargeFileUpload:         status.Capability.SupportsLargeRequestBody,
			SupportsBidirectionalHTTPTunnel: status.Capability.SupportsBidirectionalHTTPTunnel,
		},
	}
	d.recordOpsAudit(r, readOperator(r), "QUERY_SYSTEM_STATUS", map[string]any{"path": r.URL.Path})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
}

func (d handlerDeps) handleDiagnosticsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	requestID := strings.TrimSpace(r.URL.Query().Get("request_id"))
	traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))
	status := d.networkStatusFunc(r.Context())
	compat := nodeconfig.CompatibilityStatus{}
	if d.nodeStore != nil {
		compat = d.compatibilitySnapshot(r.Context())
	}
	nodeID := "gateway"
	if d.nodeStore != nil {
		if id := strings.TrimSpace(d.nodeStore.GetLocalNode().NodeID); id != "" {
			nodeID = id
		}
	}
	generatedAt := time.Now().UTC()
	jobID := generatedAt.Format("150405")
	nodeToken := safeExportToken(nodeID)
	prefix := fmt.Sprintf("diag_%s_%s", nodeToken, generatedAt.Format("20060102T150405Z"))
	if requestID != "" {
		prefix += "_req_" + safeExportToken(requestID)
	}
	if traceID != "" {
		prefix += "_trace_" + safeExportToken(traceID)
	}
	prefix += "_" + jobID

	tasks, _ := d.repo.ListTasks(r.Context(), repository.TaskFilter{Status: repository.TaskStatusFailed, RequestID: requestID, TraceID: traceID, Limit: 20})
	taskSummary := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		taskSummary = append(taskSummary, map[string]any{
			"id":          task.ID,
			"request_id":  task.RequestID,
			"trace_id":    task.TraceID,
			"api_code":    task.APICode,
			"status":      task.Status,
			"result_code": task.ResultCode,
			"updated_at":  task.UpdatedAt.UTC().Format(time.RFC3339),
			"last_error":  maskText(task.LastError),
		})
	}

	audits, _ := d.audit.List(r.Context(), observability.AuditQuery{RequestID: requestID, TraceID: traceID, Limit: 50})
	rateLimitHits := make([]map[string]any, 0, 20)
	for _, evt := range audits {
		if !strings.Contains(strings.ToUpper(evt.FinalResult), "RATE_LIMIT") {
			continue
		}
		rateLimitHits = append(rateLimitHits, map[string]any{
			"when":       evt.When.UTC().Format(time.RFC3339),
			"request_id": evt.Core.RequestID,
			"trace_id":   evt.Core.TraceID,
			"api_code":   evt.Core.APICode,
			"result":     maskText(evt.FinalResult),
		})
	}

	readmeLines := []string{
		"诊断包文件说明（字段已脱敏，可用于人工排障）",
		"00_startup_summary.json: 统一启动与运行摘要，供日志/API/UI/诊断复用。",
		"01_transport_config.json: 当前 NetworkMode/Capability/TunnelTransportPlan + SIP/RTP transport 与关键网络参数快照。",
		"01_transport_config.json.data_sources: 明确 node/peers/mappings/mode/capability 的真实来源。",
		"02_connection_stats_snapshot.json: SIP/RTP 连接计数与错误累计值。",
		"03_port_pool_status.json: RTP 端口池使用情况与分配失败累计值。",
		"04_transport_error_summary.json: 最近 transport 绑定/网络错误摘要。",
		"05_task_failure_summary.json: 最近失败任务摘要，支持 request_id/trace_id 定向过滤。",
		"06_rate_limit_hit_summary.json: 最近 rate limit 命中记录，支持 request_id/trace_id 定向过滤。",
		"07_profile_entry.json: pprof 采集入口与启用状态（不包含凭据）。",
	}
	nodeSource := d.nodeConfigSource
	if strings.TrimSpace(nodeSource) == "" {
		nodeSource = dataSourceLabel("", "node_config.json")
	}
	mappingSource := d.mappingSource
	if strings.TrimSpace(mappingSource) == "" {
		mappingSource = dataSourceLabel("", "tunnel_mappings.json")
	}

	files := []DiagFile{
		{Name: "00_startup_summary.json", Description: "统一启动与运行摘要", Content: d.startupSummaryFn(r.Context())},
		{Name: "README.md", Description: "诊断包文件说明", Content: map[string]any{"filters": map[string]any{"request_id": requestID, "trace_id": traceID}, "files": readmeLines}},
		{Name: "01_transport_config.json", Description: "当前 transport 配置", Content: map[string]any{"network_mode": status.NetworkMode, "capability": status.Capability, "capability_summary": status.CapabilitySummary, "transport_plan": status.TransportPlan, "current_network_mode": compat.CurrentNetworkMode, "current_capability": compat.CurrentCapability, "compatibility_status": compat.CompatibilityCheck, "sip": map[string]any{"listen_ip": status.SIP.ListenIP, "listen_port": status.SIP.ListenPort, "transport": status.SIP.Transport}, "rtp": map[string]any{"listen_ip": status.RTP.ListenIP, "port_start": status.RTP.PortStart, "port_end": status.RTP.PortEnd, "transport": status.RTP.Transport}, "mappings_capability_validation": d.validateMappingsAgainstCapability(d.mappings.List()), "data_sources": map[string]any{"node_config": nodeSource, "peers": nodeSource, "mappings": mappingSource, "mode": "runtime_network_config", "capability": "derived_from_network_mode"}}},
		{Name: "02_connection_stats_snapshot.json", Description: "连接统计快照", Content: map[string]any{"sip": map[string]any{"current_sessions": status.SIP.CurrentSessions, "current_connections": status.SIP.CurrentConnections, "accepted_connections_total": status.SIP.AcceptedConnectionsTotal, "closed_connections_total": status.SIP.ClosedConnectionsTotal, "read_timeout_total": status.SIP.ReadTimeoutTotal, "write_timeout_total": status.SIP.WriteTimeoutTotal, "connection_error_total": status.SIP.ConnectionErrorTotal}, "rtp": map[string]any{"active_transfers": status.RTP.ActiveTransfers, "rtp_tcp_sessions_current": status.RTP.TCPSessionsCurrent, "rtp_tcp_sessions_total": status.RTP.TCPSessionsTotal, "rtp_tcp_read_errors_total": status.RTP.TCPReadErrorsTotal, "rtp_tcp_write_errors_total": status.RTP.TCPWriteErrorsTotal}}},
		{Name: "03_port_pool_status.json", Description: "端口池状态", Content: map[string]any{"used_ports": status.RTP.UsedPorts, "available_ports": status.RTP.AvailablePorts, "rtp_port_pool_total": status.RTP.PortPoolTotal, "rtp_port_pool_used": status.RTP.PortPoolUsed, "rtp_port_alloc_fail_total": status.RTP.PortAllocFailTotal}},
		{Name: "04_transport_error_summary.json", Description: "最近 transport 错误摘要", Content: map[string]any{"recent_bind_errors": maskStringSlice(status.RecentBindErrors), "recent_network_errors": maskStringSlice(status.RecentNetworkErrors)}},
		{Name: "05_task_failure_summary.json", Description: "最近 task failure 摘要", Content: taskSummary},
		{Name: "06_rate_limit_hit_summary.json", Description: "最近 rate limit 命中摘要", Content: rateLimitHits},
		{Name: "07_profile_entry.json", Description: "profile 采集入口信息（如果启用）", Content: map[string]any{"enabled": strings.EqualFold(strings.TrimSpace(os.Getenv("GATEWAY_PPROF_ENABLED")), "true") || strings.TrimSpace(os.Getenv("GATEWAY_PPROF_ENABLED")) == "1", "listen_address": strings.TrimSpace(os.Getenv("GATEWAY_PPROF_LISTEN_ADDR")), "profile_url": "/debug/pprof/profile"}},
	}
	d.recordOpsAudit(r, readOperator(r), "EXPORT_DIAGNOSTICS", map[string]any{"request_id": requestID, "trace_id": traceID})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: DiagnosticExportData{GeneratedAt: generatedAt, JobID: jobID, NodeID: nodeID, RequestID: requestID, TraceID: traceID, FileName: prefix + ".zip", OutputDir: prefix, Files: files}})
}

func maskText(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) <= 12 {
		return "***"
	}
	return v[:12] + "***"
}

func maskStringSlice(values []string) []string {
	masked := make([]string, 0, len(values))
	for _, item := range values {
		masked = append(masked, maskText(item))
	}
	return masked
}

func safeExportToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "na"
	}
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		if r == '-' || r == '_' {
			return '_'
		}
		return '_'
	}, v)
	return strings.Trim(safe, "_")
}

func (d handlerDeps) validateMappingAgainstCapability(mapping TunnelMapping) MappingCapabilityValidation {
	status := d.networkStatusFunc(context.Background())
	return tunnelmapping.ValidateMappingCapability(mapping, status.NetworkMode, status.Capability)
}

func (d handlerDeps) validateMappingsAgainstCapability(mappings []TunnelMapping) MappingCapabilityValidation {
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
