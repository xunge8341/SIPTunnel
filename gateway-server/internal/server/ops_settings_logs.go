package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"siptunnel/internal/repository"
)

type dashboardOpsSummary struct {
	TopMappings         []summaryItem `json:"top_mappings"`
	TopSourceIPs        []summaryItem `json:"top_source_ips"`
	TopFailedMappings   []summaryItem `json:"top_failed_mappings"`
	TopFailedSourceIPs  []summaryItem `json:"top_failed_source_ips"`
	RateLimitStatus     string        `json:"rate_limit_status"`
	CircuitBreakerState string        `json:"circuit_breaker_state"`
	ProtectionStatus    string        `json:"protection_status"`
}

type summaryItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func defaultSystemSettings(d handlerDeps, sqlitePath string) SystemSettingsPayload {
	if strings.TrimSpace(sqlitePath) == "" {
		sqlitePath = "./data/final/gateway.db"
	}
	return SystemSettingsPayload{
		SQLitePath:           sqlitePath,
		LogCleanupCron:       "*/30 * * * *",
		MaxTaskAgeDays:       7,
		MaxTaskRecords:       20000,
		MaxAccessLogAgeDays:  7,
		MaxAccessLogRecords:  20000,
		MaxAuditAgeDays:      30,
		MaxAuditRecords:      50000,
		MaxDiagnosticAgeDays: 15,
		MaxDiagnosticRecords: 2000,
		MaxLoadtestAgeDays:   15,
		MaxLoadtestRecords:   2000,
		AdminAllowCIDR:       "127.0.0.1/32",
		AdminRequireMFA:      false,
		CleanerLastResult:    "未执行",
	}
}

func (d *handlerDeps) handleSystemSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		resp := d.systemSettings
		d.mu.RUnlock()
		if d.cleaner != nil {
			resp.CleanerLastRunAt = d.cleaner.LastRunAt
			resp.CleanerLastResult = d.cleaner.LastResult
			resp.CleanerLastRemovedRecords = d.cleaner.LastRemoved
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
	case http.MethodPut:
		var req SystemSettingsPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		req.SQLitePath = strings.TrimSpace(req.SQLitePath)
		if req.SQLitePath == "" {
			req.SQLitePath = d.systemSettings.SQLitePath
		}
		d.mu.Lock()
		d.systemSettings = req
		d.mu.Unlock()
		_ = saveJSON(d.systemPath, req)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: req})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleAccessLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize = 50
	}
	status := repository.TaskStatus(strings.TrimSpace(q.Get("status")))
	tasks, _ := d.repo.ListTasks(r.Context(), repository.TaskFilter{TaskType: repository.TaskTypeCommand, Status: status, RequestID: q.Get("request_id"), TraceID: q.Get("trace_id"), Limit: pageSize, Offset: (page - 1) * pageSize})
	items := make([]AccessLogEntry, 0, len(tasks))
	for _, t := range tasks {
		method := "POST"
		path := "/api/" + strings.TrimSpace(t.APICode)
		if strings.TrimSpace(t.APICode) == "" {
			path = "/unknown"
		}
		statusCode := 200
		if t.Status == repository.TaskStatusFailed || t.Status == repository.TaskStatusDeadLettered {
			statusCode = 500
		}
		items = append(items, AccessLogEntry{ID: t.ID, OccurredAt: t.UpdatedAt.UTC().Format(time.RFC3339), MappingName: defaultString(t.APICode, "未命名映射"), SourceIP: defaultString(t.SourceSystem, "unknown"), Method: method, Path: path, StatusCode: statusCode, DurationMS: t.UpdatedAt.Sub(t.CreatedAt).Milliseconds(), FailureReason: t.LastError, RequestID: t.RequestID, TraceID: t.TraceID})
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: listData[AccessLogEntry]{Items: items, Pagination: pagination{Page: page, PageSize: pageSize, Total: len(items)}}})
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (d *handlerDeps) handleDashboardOpsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	tasks, _ := d.repo.ListTasks(r.Context(), repository.TaskFilter{TaskType: repository.TaskTypeCommand, Limit: 1000})
	countByMapping := map[string]int{}
	countBySource := map[string]int{}
	failedByMapping := map[string]int{}
	failedBySource := map[string]int{}
	for _, t := range tasks {
		m := defaultString(t.APICode, "未命名映射")
		s := defaultString(t.SourceSystem, "unknown")
		countByMapping[m]++
		countBySource[s]++
		if t.Status == repository.TaskStatusFailed || t.Status == repository.TaskStatusDeadLettered {
			failedByMapping[m]++
			failedBySource[s]++
		}
	}
	summary := dashboardOpsSummary{TopMappings: topN(countByMapping, 5), TopSourceIPs: topN(countBySource, 5), TopFailedMappings: topN(failedByMapping, 5), TopFailedSourceIPs: topN(failedBySource, 5), RateLimitStatus: "正常", CircuitBreakerState: "关闭", ProtectionStatus: "未触发保护"}
	if d.limits.RPS < 50 {
		summary.RateLimitStatus = "限流阈值较低"
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: summary})
}

func topN(m map[string]int, n int) []summaryItem {
	items := make([]summaryItem, 0, len(m))
	for k, v := range m {
		items = append(items, summaryItem{Name: k, Count: v})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > n {
		items = items[:n]
	}
	return items
}
