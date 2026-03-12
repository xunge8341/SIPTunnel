package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"siptunnel/internal/observability"
)

type handlerDeps struct {
	logger *observability.StructuredLogger
	audit  observability.AuditStore
}

func NewHandler() http.Handler {
	deps := handlerDeps{
		logger: observability.NewStructuredLogger(nil),
		audit:  observability.NewInMemoryAuditStore(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", deps.healthz)
	mux.HandleFunc("/demo/process", deps.demoProcess)
	mux.HandleFunc("/audit/events", deps.listAuditEvents)
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	d.logger.Info(r.Context(), "healthz_ok", fields)
}

func (d handlerDeps) demoProcess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	w.Header().Set("Content-Type", "application/json")
	observability.InjectTraceContext(ctx, w.Header())
	status := http.StatusOK
	if !validated {
		status = http.StatusBadRequest
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result_code": fields.ResultCode,
		"core":        fields,
	})
}

func (d handlerDeps) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	query := observability.AuditQuery{
		RequestID: r.URL.Query().Get("request_id"),
		APICode:   r.URL.Query().Get("api_code"),
		Who:       r.URL.Query().Get("who"),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			query.Limit = n
		}
	}

	events, err := d.audit.List(r.Context(), query)
	if err != nil {
		http.Error(w, "query audit failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": events})
}
