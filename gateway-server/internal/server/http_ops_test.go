package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"siptunnel/internal/config"
	"siptunnel/internal/observability"
	"siptunnel/internal/repository"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/selfcheck"
	"strings"
	"testing"
	"time"
)

func TestMetricsEndpointExportsCoreMetrics(t *testing.T) {
	deps := handlerDeps{
		logger: observability.NewStructuredLogger(nil),
		repo:   memrepo.NewTaskRepository(),
		selfCheckProvider: func(_ context.Context) selfcheck.Report {
			return selfcheck.Report{Overall: selfcheck.LevelWarn, Summary: selfcheck.Summary{Info: 1, Warn: 2, Error: 0}}
		},
		networkStatusFunc: func(_ context.Context) NodeNetworkStatus {
			capability := config.DeriveCapability(config.NetworkModeSenderSIPReceiverRTP)
			return NodeNetworkStatus{
				NetworkMode: config.NetworkModeSenderSIPReceiverRTP,
				Capability:  capability,
				SIP:         SIPNetworkStatus{ListenIP: "127.0.0.1", ListenPort: 5060, Transport: "TCP", CurrentConnections: 3, ConnectionErrorTotal: 7, ReadTimeoutTotal: 2, WriteTimeoutTotal: 1},
				RTP:         RTPNetworkStatus{ListenIP: "127.0.0.1", PortStart: 20000, PortEnd: 20010, Transport: "UDP", ActiveTransfers: 2, PortPoolTotal: 11, PortPoolUsed: 4, AvailablePorts: 7, PortAllocFailTotal: 5},
			}
		},
		accessLogStore: newAccessLogStore(7, 200, nil),
		runtime:        newMappingRuntimeManager(nil),
		limits:         OpsLimits{RPS: 50, Burst: 80, MaxConcurrent: 20},
	}
	deps.protectionRuntime = newProtectionRuntime(deps.limits)
	deps.protectionRuntime.mu.Lock()
	deps.protectionRuntime.global.rateLimitHitsTotal = 4
	deps.protectionRuntime.global.allowedTotal = 9
	deps.protectionRuntime.global.active = 1
	deps.protectionRuntime.mu.Unlock()
	deps.runtime.status["map-1"] = mappingRuntimeStatus{State: mappingStateListening, Reason: "ok"}
	deps.runtime.status["map-2"] = mappingRuntimeStatus{State: mappingStateInterrupted, Reason: "recovery failed"}
	deps.runtime.recoveryFailedTotal.Store(3)
	deps.accessLogStore.Add(AccessLogEntry{ID: "1", OccurredAt: formatTimestamp(time.Now().UTC()), MappingName: "orders", SourceIP: "10.0.0.1", Method: http.MethodGet, Path: "/orders", StatusCode: 200, DurationMS: 120})
	deps.accessLogStore.Add(AccessLogEntry{ID: "2", OccurredAt: formatTimestamp(time.Now().UTC()), MappingName: "orders", SourceIP: "10.0.0.1", Method: http.MethodGet, Path: "/orders", StatusCode: 502, DurationMS: 780, FailureReason: "upstream timeout"})
	if _, err := deps.repo.CreateTask(t.Context(), repository.Task{ID: "t-1", Status: repository.TaskStatusFailed, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	newMux(deps).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, token := range []string{
		"# HELP siptunnel_sip_tcp_connection_errors_total",
		"siptunnel_sip_tcp_connection_errors_total 7",
		"siptunnel_requests_total 2",
		"siptunnel_requests_failed_total 1",
		"siptunnel_rate_limit_hits_total 4",
		"siptunnel_transport_recovery_failed_total 3",
		"siptunnel_task_total{status=\"failed\"} 1",
		"siptunnel_selfcheck_items{level=\"warn\"} 2",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("expected metrics body to contain %q\n%s", token, body)
		}
	}
}

func TestRoutesDeprecatedCompatibility(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/routes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Deprecation") != "true" {
		t.Fatalf("expected deprecation header")
	}
}

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	NewHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload responseEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if payload.Code != "OK" {
		t.Fatalf("expected OK code, got %s", payload.Code)
	}
}

func TestTasksListAndGetWithFilters(t *testing.T) {
	h, repo, _ := buildTestHandler(t)
	now := time.Now().UTC()
	_, _ = repo.CreateTask(t.Context(), repository.Task{ID: "t1", TaskType: repository.TaskTypeCommand, Status: repository.TaskStatusPending, RequestID: "req-1", TraceID: "trace-1", CreatedAt: now, UpdatedAt: now})
	_, _ = repo.CreateTask(t.Context(), repository.Task{ID: "t2", TaskType: repository.TaskTypeCommand, Status: repository.TaskStatusFailed, RequestID: "req-2", TraceID: "trace-2", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)})

	listReq := httptest.NewRequest(http.MethodGet, "/api/tasks?request_id=req-1&page=1&page_size=10", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRR.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/tasks/t1", nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for get, got %d", getRR.Code)
	}
}

func TestTaskRetryAndCancelWritesAudit(t *testing.T) {
	h, repo, audit := buildTestHandler(t)
	now := time.Now().UTC()
	_, _ = repo.CreateTask(t.Context(), repository.Task{ID: "t-retry", TaskType: repository.TaskTypeCommand, Status: repository.TaskStatusFailed, RequestID: "req-r", TraceID: "trace-r", CreatedAt: now, UpdatedAt: now})
	_, _ = repo.CreateTask(t.Context(), repository.Task{ID: "t-cancel", TaskType: repository.TaskTypeCommand, Status: repository.TaskStatusPending, RequestID: "req-c", TraceID: "trace-c", CreatedAt: now, UpdatedAt: now})

	retryReq := httptest.NewRequest(http.MethodPost, "/api/tasks/t-retry/retry", bytes.NewBufferString(`{"operator":"ops"}`))
	retryReq.Header.Set("X-Initiator", "ops")
	retryRR := httptest.NewRecorder()
	h.ServeHTTP(retryRR, retryReq)
	if retryRR.Code != http.StatusOK {
		t.Fatalf("expected retry 200, got %d body=%s", retryRR.Code, retryRR.Body.String())
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/tasks/t-cancel/cancel", bytes.NewBufferString(`{"operator":"ops"}`))
	cancelReq.Header.Set("X-Initiator", "ops")
	cancelRR := httptest.NewRecorder()
	h.ServeHTTP(cancelRR, cancelReq)
	if cancelRR.Code != http.StatusOK {
		t.Fatalf("expected cancel 200, got %d body=%s", cancelRR.Code, cancelRR.Body.String())
	}

	events, err := audit.List(t.Context(), observability.AuditQuery{Who: "ops", Limit: 10})
	if err != nil {
		t.Fatalf("query audit failed: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected audit events for ops actions, got %d", len(events))
	}
}

func TestLimitsRoutesNodesAndAudits(t *testing.T) {
	h, _, audit := buildTestHandler(t)
	_ = audit.Record(t.Context(), observability.AuditEvent{Who: "ops", When: time.Now().UTC(), OpsAction: "UPDATE_LIMITS", Core: observability.CoreFields{RequestID: "r1", TraceID: "t1", APICode: "asset.sync"}})

	cases := []struct {
		method string
		target string
		body   string
		code   int
	}{
		{http.MethodGet, "/api/limits", "", http.StatusOK},
		{http.MethodPut, "/api/limits", `{"rps":80,"burst":120,"max_concurrent":30}`, http.StatusOK},
		{http.MethodGet, "/api/routes", "", http.StatusOK},
		{http.MethodPut, "/api/routes", `{"routes":[{"api_code":"asset.sync","http_method":"POST","http_path":"/sync","enabled":true}]}`, http.StatusOK},
		{http.MethodGet, "/api/nodes", "", http.StatusOK},
		{http.MethodGet, "/api/startup-summary", "", http.StatusOK},
		{http.MethodGet, "/api/system/status", "", http.StatusOK},
		{http.MethodGet, "/api/audits?trace_id=t1&page=1&page_size=10", "", http.StatusOK},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.target, bytes.NewBufferString(tc.body))
		req.Header.Set("X-Initiator", "ops")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != tc.code {
			t.Fatalf("%s %s expected %d got %d body=%s", tc.method, tc.target, tc.code, rr.Code, rr.Body.String())
		}
	}
}

func TestAuditsEndpointAdvancedFilters(t *testing.T) {
	h, _, audit := buildTestHandler(t)
	base := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)
	_ = audit.Record(t.Context(), observability.AuditEvent{Who: "ops", When: base, RequestType: "ops", LocalServiceRoute: "gateway", OpsAction: "UPDATE_LIMITS", FinalResult: "OK", Core: observability.CoreFields{APICode: "asset.sync"}})
	_ = audit.Record(t.Context(), observability.AuditEvent{Who: "ops", When: base.Add(30 * time.Minute), RequestType: "demo.process", LocalServiceRoute: "gateway", OpsAction: "NONE", FinalResult: "UPSTREAM_TIMEOUT", Core: observability.CoreFields{APICode: "asset.upload"}})

	req := httptest.NewRequest(http.MethodGet, "/api/audits?rule=upload&error_only=true&start_time=2026-03-08T10:15:00Z&end_time=2026-03-08T10:45:00Z&page=1&page_size=10", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Code string `json:"code"`
		Data struct {
			Items []observability.AuditEvent `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if payload.Code != "OK" {
		t.Fatalf("unexpected code: %s", payload.Code)
	}
	if len(payload.Data.Items) != 1 {
		t.Fatalf("expected 1 filtered audit item, got %d", len(payload.Data.Items))
	}
	if payload.Data.Items[0].Core.APICode != "asset.upload" || payload.Data.Items[0].FinalResult != "UPSTREAM_TIMEOUT" {
		t.Fatalf("unexpected filtered audit item: %+v", payload.Data.Items[0])
	}
}

func TestCapacityRecommendationEndpoint(t *testing.T) {
	h, _, audit := buildTestHandler(t)
	summary := `{"run_id":"r1","generated_at":"2026-01-01T00:00:00Z","config":{"Targets":null,"Concurrency":0,"QPS":0,"Duration":0,"FileSize":0,"ChunkSize":0,"TransferMode":"","SIPAddress":"","RTPAddress":"","HTTPURL":"","OutputDir":"","Timeout":0},"summaries":{"sip-command-create":{"target":"sip-command-create","total":1000,"success":998,"failed":2,"success_rate":0.998,"throughput_qps":220,"p50_ms":30,"p95_ms":120,"p99_ms":180,"error_types":{},"elapsed_ms":10000,"concurrency":120,"configured_qps":300},"rtp-udp-upload":{"target":"rtp-udp-upload","total":600,"success":598,"failed":2,"success_rate":0.996,"throughput_qps":80,"p50_ms":100,"p95_ms":220,"p99_ms":350,"error_types":{},"elapsed_ms":10000,"concurrency":50,"configured_qps":0}},"result_file":"/tmp/results.jsonl"}`
	path := t.TempDir() + "/summary.json"
	if err := os.WriteFile(path, []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary failed: %v", err)
	}
	bodyBytes, _ := json.Marshal(map[string]any{"summary_file": path, "current": map[string]any{"command_max_concurrent": 90, "file_transfer_max_concurrent": 40, "rtp_port_pool_size": 120, "max_connections": 150, "rate_limit_rps": 280, "rate_limit_burst": 420}})
	req := httptest.NewRequest(http.MethodPost, "/api/capacity/recommendation", bytes.NewReader(bodyBytes))
	req.Header.Set("X-Initiator", "capacity-ops")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "recommended_command_max_concurrent") {
		t.Fatalf("unexpected response: %s", rr.Body.String())
	}
	events, err := audit.List(t.Context(), observability.AuditQuery{Who: "capacity-ops", Limit: 10})
	if err != nil {
		t.Fatalf("query audit failed: %v", err)
	}
	if len(events) == 0 || events[0].OpsAction != "QUERY_CAPACITY_RECOMMENDATION" {
		t.Fatalf("unexpected audit events: %+v", events)
	}
}

func TestSelfCheckEndpoint(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/selfcheck", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "sample") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "local_node_config_valid") || !strings.Contains(rr.Body.String(), "peer_node_config_valid") || !strings.Contains(rr.Body.String(), "network_mode_compatibility") {
		t.Fatalf("expected compatibility items in selfcheck: %s", rr.Body.String())
	}
}

func TestSelfCheckEndpointFilterByLevel(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/selfcheck?level=warn,error", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "sample-info") {
		t.Fatalf("info item should be filtered out: %s", body)
	}
	if !strings.Contains(body, "sample-warn") || !strings.Contains(body, "sample-error") {
		t.Fatalf("expected warn/error items: %s", body)
	}
	var payload struct {
		Data struct {
			Summary map[string]int `json:"summary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal selfcheck failed: %v body=%s", err, body)
	}
	if payload.Data.Summary["info"] != 0 || payload.Data.Summary["warn"] < 1 || payload.Data.Summary["error"] < 1 {
		t.Fatalf("expected filtered summary counts info=0 warn>=1 error>=1: %s", body)
	}
}

func TestDiagnosticsExportEndpointWithFilters(t *testing.T) {
	h, repo, audit := buildTestHandler(t)
	now := time.Now().UTC()
	_, _ = repo.CreateTask(t.Context(), repository.Task{ID: "tf-1", TaskType: repository.TaskTypeCommand, Status: repository.TaskStatusFailed, RequestID: "req-d", TraceID: "trace-d", APICode: "asset.sync", ResultCode: "UPSTREAM_RATE_LIMIT", LastError: "token=secret-value", UpdatedAt: now, CreatedAt: now})
	_ = audit.Record(t.Context(), observability.AuditEvent{Who: "ops", When: now, FinalResult: "UPSTREAM_RATE_LIMIT", Core: observability.CoreFields{RequestID: "req-d", TraceID: "trace-d", APICode: "asset.sync"}})

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics/export?request_id=req-d&trace_id=trace-d", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Code string               `json:"code"`
		Data DiagnosticExportData `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if payload.Data.RequestID != "req-d" || payload.Data.TraceID != "trace-d" {
		t.Fatalf("unexpected filters: %+v", payload.Data)
	}
	if payload.Data.JobID == "" || !strings.Contains(payload.Data.FileName, payload.Data.JobID) || !strings.Contains(payload.Data.OutputDir, payload.Data.JobID) || !strings.Contains(payload.Data.FileName, "req_req_d") || !strings.Contains(payload.Data.OutputDir, "trace_trace_d") {
		t.Fatalf("unexpected naming: file=%s dir=%s", payload.Data.FileName, payload.Data.OutputDir)
	}
	if len(payload.Data.Files) < 9 {
		t.Fatalf("expected diagnostic files, got %d", len(payload.Data.Files))
	}
	if payload.Data.Files[0].Name != "00_startup_summary.json" {
		t.Fatalf("expected first file to be startup summary, got %s", payload.Data.Files[0].Name)
	}
	var taskFile DiagFile
	for _, f := range payload.Data.Files {
		if f.Name == "05_task_failure_summary.json" {
			taskFile = f
			break
		}
	}
	items, ok := taskFile.Content.([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("task summary missing: %#v", taskFile.Content)
	}
	first, _ := items[0].(map[string]any)
	if got, _ := first["last_error"].(string); got == "token=secret-value" || got == "" {
		t.Fatalf("last_error should be masked, got=%q", got)
	}
	var rateLimitFile DiagFile
	for _, f := range payload.Data.Files {
		if f.Name == "06_rate_limit_hit_summary.json" {
			rateLimitFile = f
			break
		}
	}
	rateItems, ok := rateLimitFile.Content.([]any)
	if !ok || len(rateItems) == 0 {
		t.Fatalf("rate limit summary missing: %#v", rateLimitFile.Content)
	}
	rateFirst, _ := rateItems[0].(map[string]any)
	if got, _ := rateFirst["result"].(string); got == "UPSTREAM_RATE_LIMIT" || got == "" {
		t.Fatalf("rate limit result should be masked, got=%q", got)
	}

	var transportFile DiagFile
	for _, f := range payload.Data.Files {
		if f.Name == "01_transport_config.json" {
			transportFile = f
			break
		}
	}
	transportContent, ok := transportFile.Content.(map[string]any)
	if !ok || transportContent["network_mode"] == nil || transportContent["capability"] == nil {
		t.Fatalf("transport file missing mode/capability: %#v", transportFile.Content)
	}

	var readmeFile DiagFile
	for _, f := range payload.Data.Files {
		if f.Name == "README.md" {
			readmeFile = f
			break
		}
	}
	readme, ok := readmeFile.Content.(map[string]any)
	if !ok {
		t.Fatalf("readme content type mismatch: %#v", readmeFile.Content)
	}
	filters, ok := readme["filters"].(map[string]any)
	if !ok || filters["request_id"] != "req-d" || filters["trace_id"] != "trace-d" {
		t.Fatalf("readme filters mismatch: %#v", readmeFile.Content)
	}
}

func TestStartupSummaryIncludesCompatibilityFields(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/startup-summary", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "current_network_mode") || !strings.Contains(rr.Body.String(), "compatibility_status") {
		t.Fatalf("startup summary missing compatibility fields: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "data_sources") || !strings.Contains(rr.Body.String(), "node_config") {
		t.Fatalf("startup summary missing data source fields: %s", rr.Body.String())
	}
}

func TestSystemStatusEndpoint(t *testing.T) {
	h, _, audit := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/system/status", nil)
	req.Header.Set("X-Initiator", "ops-system")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Code string               `json:"code"`
		Data SystemStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if payload.Code != "OK" {
		t.Fatalf("unexpected code: %s", payload.Code)
	}
	if payload.Data.TunnelStatus != "degraded" {
		t.Fatalf("unexpected tunnel status: %+v", payload.Data)
	}
	if payload.Data.NetworkMode != config.NetworkModeSenderSIPReceiverRTP {
		t.Fatalf("unexpected network mode: %s", payload.Data.NetworkMode)
	}
	if !payload.Data.Capability.SupportsSmallRequestBody || !payload.Data.Capability.SupportsLargeResponseBody || payload.Data.Capability.SupportsLargeFileUpload {
		t.Fatalf("unexpected capability matrix: %+v", payload.Data.Capability)
	}
	if payload.Data.RegistrationStatus != "" && payload.Data.RegistrationStatus != "unregistered" && payload.Data.RegistrationStatus != "registered" {
		t.Fatalf("unexpected registration status: %s", payload.Data.RegistrationStatus)
	}
	if payload.Data.HeartbeatStatus != "" && payload.Data.HeartbeatStatus != "unknown" && payload.Data.HeartbeatStatus != "healthy" {
		t.Fatalf("unexpected heartbeat status: %s", payload.Data.HeartbeatStatus)
	}
	if payload.Data.MappingTotal <= 0 || payload.Data.MappingAbnormalTotal != payload.Data.MappingTotal {
		t.Fatalf("unexpected mapping stats: total=%d abnormal=%d", payload.Data.MappingTotal, payload.Data.MappingAbnormalTotal)
	}
	if strings.TrimSpace(payload.Data.LatestMappingErrorReason) == "" {
		t.Fatalf("expected latest mapping error reason")
	}

	events, err := audit.List(t.Context(), observability.AuditQuery{Who: "ops-system", Limit: 10})
	if err != nil {
		t.Fatalf("query audit failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected audit event for system status query")
	}
	if events[0].OpsAction != "QUERY_SYSTEM_STATUS" {
		t.Fatalf("unexpected ops action: %s", events[0].OpsAction)
	}
}

func TestProtectionRestrictionsEndpoint(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	postReq := httptest.NewRequest(http.MethodPost, "/api/protection/restrictions", bytes.NewBufferString(`{"scope":"source","target":"10.0.0.9","minutes":15,"reason":"hot ip"}`))
	postReq.Header.Set("Content-Type", "application/json")
	postRR := httptest.NewRecorder()
	h.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("expected post 200 got %d body=%s", postRR.Code, postRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/protection/restrictions", nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected get 200 got %d body=%s", getRR.Code, getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), "10.0.0.9") {
		t.Fatalf("expected restrictions response to contain target, got %s", getRR.Body.String())
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/protection/restrictions", bytes.NewBufferString(`{"scope":"source","target":"10.0.0.9"}`))
	delReq.Header.Set("Content-Type", "application/json")
	delRR := httptest.NewRecorder()
	h.ServeHTTP(delRR, delReq)
	if delRR.Code != http.StatusOK {
		t.Fatalf("expected delete 200 got %d body=%s", delRR.Code, delRR.Body.String())
	}
}

func TestSystemResourceUsageEndpoint(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/system/resource-usage", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	for _, needle := range []string{"cpu_cores", "configured_generic_download_mbps", "configured_generic_download_window_mb", "configured_generic_rtp_reorder_window_packets", "configured_generic_rtp_fec_enabled", "status_color", "recommended_profile", "recommended_max_concurrent"} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("expected body to contain %s, got %s", needle, rr.Body.String())
		}
	}
}
