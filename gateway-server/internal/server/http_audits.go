package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"siptunnel/internal/observability"
)

func (d *handlerDeps) listAuditEvents(w http.ResponseWriter, r *http.Request) {
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

func (d *handlerDeps) handleAudits(w http.ResponseWriter, r *http.Request) {
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

func (d *handlerDeps) recordOpsAudit(r *http.Request, operator string, action string, payload any) {
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
	if session, ok := adminSessionFromContext(r.Context()); ok && strings.TrimSpace(session.Operator) != "" {
		return strings.TrimSpace(session.Operator)
	}
	if op := strings.TrimSpace(r.Header.Get("X-Admin-Operator")); op != "" {
		return op
	}
	if op := strings.TrimSpace(r.Header.Get("X-Initiator")); op != "" {
		return op
	}
	return "system"
}
