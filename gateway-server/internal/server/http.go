package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/observability"
	"siptunnel/internal/repository"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/service"
	"siptunnel/internal/service/taskengine"
	"siptunnel/loadtest"
)

type handlerDeps struct {
	logger            *observability.StructuredLogger
	selfCheckProvider func(context.Context) selfcheck.Report
	networkStatusFunc func(context.Context) NodeNetworkStatus
	audit             observability.AuditStore
	repo              repository.TaskRepository
	engine            *taskengine.Engine

	mu     sync.RWMutex
	limits OpsLimits
	routes map[string]OpsRoute
	nodes  []OpsNode
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
	APICode    string `json:"api_code"`
	HTTPMethod string `json:"http_method"`
	HTTPPath   string `json:"http_path"`
	Enabled    bool   `json:"enabled"`
}

type OpsNode struct {
	NodeID   string `json:"node_id"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	Endpoint string `json:"endpoint"`
}

type opsActionRequest struct {
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

type NodeNetworkStatus struct {
	SIP                 SIPNetworkStatus `json:"sip"`
	RTP                 RTPNetworkStatus `json:"rtp"`
	RecentBindErrors    []string         `json:"recent_bind_errors"`
	RecentNetworkErrors []string         `json:"recent_network_errors"`
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
	LogDir            string
	AuditDir          string
	SelfCheckProvider func(context.Context) selfcheck.Report
	NetworkStatusFunc func(context.Context) NodeNetworkStatus
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
	deps := handlerDeps{
		logger: logger,
		audit:  audit,
		repo:   repo,
		engine: engine,
		limits: OpsLimits{RPS: 200, Burst: 400, MaxConcurrent: 100},
		routes: map[string]OpsRoute{
			"asset.sync":   {APICode: "asset.sync", HTTPMethod: "POST", HTTPPath: "/v1/assets/sync", Enabled: true},
			"asset.delete": {APICode: "asset.delete", HTTPMethod: "DELETE", HTTPPath: "/v1/assets", Enabled: true},
		},
		nodes:             []OpsNode{{NodeID: "gateway-a-01", Role: "gateway", Status: "ready", Endpoint: "10.0.0.11:18080"}},
		selfCheckProvider: opts.SelfCheckProvider,
		networkStatusFunc: opts.NetworkStatusFunc,
	}
	if deps.networkStatusFunc == nil {
		defaults := config.DefaultNetworkConfig()
		deps.networkStatusFunc = func(context.Context) NodeNetworkStatus {
			availablePorts := defaults.RTP.PortEnd - defaults.RTP.PortStart + 1
			if availablePorts < 0 {
				availablePorts = 0
			}
			return NodeNetworkStatus{
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
	return newMux(deps), joinClosers(logCloser, auditCloser), nil
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
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", deps.healthz)
	mux.HandleFunc("/demo/process", deps.demoProcess)
	mux.HandleFunc("/audit/events", deps.listAuditEvents)
	mux.HandleFunc("/api/tasks", deps.handleTasks)
	mux.HandleFunc("/api/tasks/", deps.handleTaskByID)
	mux.HandleFunc("/api/limits", deps.handleLimits)
	mux.HandleFunc("/api/routes", deps.handleRoutes)
	mux.HandleFunc("/api/nodes", deps.handleNodes)
	mux.HandleFunc("/api/audits", deps.handleAudits)
	mux.HandleFunc("/api/selfcheck", deps.handleSelfCheck)
	mux.HandleFunc("/api/node/network-status", deps.handleNodeNetworkStatus)
	mux.HandleFunc("/api/diagnostics/export", deps.handleDiagnosticsExport)
	mux.HandleFunc("/api/capacity/recommendation", deps.handleCapacityRecommendation)
	return deps.withObservability(mux)
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
	query := observability.AuditQuery{RequestID: r.URL.Query().Get("request_id"), APICode: r.URL.Query().Get("api_code"), Who: r.URL.Query().Get("who")}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			query.Limit = n
		}
	}
	if v := r.URL.Query().Get("trace_id"); v != "" {
		query.TraceID = v
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
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		items := make([]OpsRoute, 0, len(d.routes))
		for _, route := range d.routes {
			items = append(items, route)
		}
		d.mu.RUnlock()
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": items}})
	case http.MethodPut:
		var req updateRoutesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		updated := make(map[string]OpsRoute, len(req.Routes))
		for _, route := range req.Routes {
			if route.APICode == "" || route.HTTPMethod == "" || route.HTTPPath == "" {
				writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "route fields are required")
				return
			}
			updated[route.APICode] = route
		}
		d.mu.Lock()
		d.routes = updated
		d.mu.Unlock()
		d.recordOpsAudit(r, readOperator(r), "UPDATE_ROUTES", map[string]any{"count": len(req.Routes)})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": req.Routes}})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
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

func fallbackPositive(v int, fallback int) int {
	if v > 0 {
		return v
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func (d handlerDeps) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	d.mu.RLock()
	nodes := append([]OpsNode(nil), d.nodes...)
	d.mu.RUnlock()
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": nodes}})
}

func (d handlerDeps) handleAudits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	page, pageSize := parsePagination(r)
	query := observability.AuditQuery{RequestID: r.URL.Query().Get("request_id"), APICode: r.URL.Query().Get("api_code"), Who: r.URL.Query().Get("who"), TraceID: r.URL.Query().Get("trace_id")}
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
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: report})
}

func (d handlerDeps) handleNodeNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	status := d.networkStatusFunc(r.Context())
	d.recordOpsAudit(r, readOperator(r), "QUERY_NODE_NETWORK_STATUS", map[string]any{"path": r.URL.Path})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: status})
}

func (d handlerDeps) handleDiagnosticsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	requestID := strings.TrimSpace(r.URL.Query().Get("request_id"))
	traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))
	status := d.networkStatusFunc(r.Context())
	nodeID := "gateway"
	if len(d.nodes) > 0 && strings.TrimSpace(d.nodes[0].NodeID) != "" {
		nodeID = d.nodes[0].NodeID
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
		"01_transport_config.json: 当前 SIP/RTP transport 与关键网络参数快照。",
		"02_connection_stats_snapshot.json: SIP/RTP 连接计数与错误累计值。",
		"03_port_pool_status.json: RTP 端口池使用情况与分配失败累计值。",
		"04_transport_error_summary.json: 最近 transport 绑定/网络错误摘要。",
		"05_task_failure_summary.json: 最近失败任务摘要，支持 request_id/trace_id 定向过滤。",
		"06_rate_limit_hit_summary.json: 最近 rate limit 命中记录，支持 request_id/trace_id 定向过滤。",
		"07_profile_entry.json: pprof 采集入口与启用状态（不包含凭据）。",
	}

	files := []DiagFile{
		{Name: "README.md", Description: "诊断包文件说明", Content: map[string]any{"filters": map[string]any{"request_id": requestID, "trace_id": traceID}, "files": readmeLines}},
		{Name: "01_transport_config.json", Description: "当前 transport 配置", Content: map[string]any{"sip": map[string]any{"listen_ip": status.SIP.ListenIP, "listen_port": status.SIP.ListenPort, "transport": status.SIP.Transport}, "rtp": map[string]any{"listen_ip": status.RTP.ListenIP, "port_start": status.RTP.PortStart, "port_end": status.RTP.PortEnd, "transport": status.RTP.Transport}}},
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

func writeJSON(w http.ResponseWriter, status int, payload responseEnvelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, responseEnvelope{Code: code, Message: message})
}
