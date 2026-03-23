package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"siptunnel/internal/repository"
	"siptunnel/internal/tunnelmapping"
)

func (d *handlerDeps) handleTasks(w http.ResponseWriter, r *http.Request) {
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

func (d *handlerDeps) handleTaskByID(w http.ResponseWriter, r *http.Request) {
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

func (d *handlerDeps) performTaskAction(w http.ResponseWriter, r *http.Request, taskID string, action string) {
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

func (d *handlerDeps) handleLimits(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		limits := normalizeOpsLimits(d.limits)
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
		d.limits = normalizeOpsLimits(OpsLimits(req))
		d.baselineLimits = d.limits
		updated := d.limits
		protector := d.protectionRuntime
		d.mu.Unlock()
		if protector != nil {
			protector.UpdateLimits(updated)
		}
		if d.sqliteStore != nil {
			_ = d.sqliteStore.SaveSystemConfig(r.Context(), "ops.limits", updated)
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_LIMITS", updated)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: updated})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleRoutes(w http.ResponseWriter, r *http.Request) {
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
		d.onLocalCatalogChanged()
		d.recordOpsAudit(r, readOperator(r), "UPDATE_ROUTES", map[string]any{"count": len(req.Routes)})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": req.Routes, "warnings": validation.Warnings}})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
