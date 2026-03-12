package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/observability"
	"siptunnel/internal/repository"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/service"
	"siptunnel/internal/service/taskengine"
)

type handlerDeps struct {
	logger *observability.StructuredLogger
	audit  observability.AuditStore
	repo   repository.TaskRepository
	engine *taskengine.Engine

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

type updateLimitsRequest struct {
	RPS           int `json:"rps"`
	Burst         int `json:"burst"`
	MaxConcurrent int `json:"max_concurrent"`
}

type updateRoutesRequest struct {
	Routes []OpsRoute `json:"routes"`
}

func NewHandler() http.Handler {
	repo := memrepo.NewTaskRepository()
	engine := taskengine.NewEngine(repo, service.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second})
	deps := handlerDeps{
		logger: observability.NewStructuredLogger(nil),
		audit:  observability.NewInMemoryAuditStore(),
		repo:   repo,
		engine: engine,
		limits: OpsLimits{RPS: 200, Burst: 400, MaxConcurrent: 100},
		routes: map[string]OpsRoute{
			"asset.sync":   {APICode: "asset.sync", HTTPMethod: "POST", HTTPPath: "/v1/assets/sync", Enabled: true},
			"asset.delete": {APICode: "asset.delete", HTTPMethod: "DELETE", HTTPPath: "/v1/assets", Enabled: true},
		},
		nodes: []OpsNode{{NodeID: "gateway-a-01", Role: "gateway", Status: "ready", Endpoint: "10.0.0.11:18080"}},
	}
	return newMux(deps)
}

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

func writeJSON(w http.ResponseWriter, status int, payload responseEnvelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, responseEnvelope{Code: code, Message: message})
}
