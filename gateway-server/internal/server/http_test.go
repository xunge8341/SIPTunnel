package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"siptunnel/internal/observability"
	"siptunnel/internal/repository"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/service"
	"siptunnel/internal/service/taskengine"
)

func buildTestHandler(t *testing.T) (http.Handler, repository.TaskRepository, observability.AuditStore) {
	t.Helper()
	repo := memrepo.NewTaskRepository()
	audit := observability.NewInMemoryAuditStore()
	deps := handlerDeps{
		logger: observability.NewStructuredLogger(nil),
		audit:  audit,
		repo:   repo,
		engine: taskengine.NewEngine(repo, service.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second}),
		limits: OpsLimits{RPS: 100, Burst: 200, MaxConcurrent: 50},
		routes: map[string]OpsRoute{"asset.sync": {APICode: "asset.sync", HTTPMethod: "POST", HTTPPath: "/sync", Enabled: true}},
		nodes:  []OpsNode{{NodeID: "n1", Role: "gateway", Status: "ready", Endpoint: "127.0.0.1:18080"}},
		selfCheckProvider: func(_ context.Context) selfcheck.Report {
			return selfcheck.Report{Overall: selfcheck.LevelInfo, Summary: selfcheck.Summary{Info: 1}, Items: []selfcheck.Item{{Name: "sample", Level: selfcheck.LevelInfo, Message: "ok", Suggestion: "none"}}}
		},
		networkStatusFunc: func(_ context.Context) NodeNetworkStatus {
			return NodeNetworkStatus{
				SIP:                 SIPNetworkStatus{ListenIP: "10.10.1.10", ListenPort: 5060, Transport: "TCP", CurrentSessions: 12, CurrentConnections: 7},
				RTP:                 RTPNetworkStatus{ListenIP: "10.10.1.10", PortStart: 30000, PortEnd: 30020, Transport: "UDP", ActiveTransfers: 3, UsedPorts: 6, AvailablePorts: 15, PortPoolTotal: 21, PortPoolUsed: 6, PortAllocFailTotal: 2},
				RecentBindErrors:    []string{"sip: bind 10.10.1.10:5061 failed"},
				RecentNetworkErrors: []string{"rtp: write timeout to 10.20.1.20:30001"},
			}
		},
	}
	return newMux(deps), repo, audit
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

func TestNodeNetworkStatusEndpointAndAudit(t *testing.T) {
	h, _, audit := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/node/network-status", nil)
	req.Header.Set("X-Initiator", "net-ops")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Code string            `json:"code"`
		Data NodeNetworkStatus `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if payload.Code != "OK" {
		t.Fatalf("unexpected code: %s", payload.Code)
	}
	if payload.Data.SIP.ListenIP != "10.10.1.10" || payload.Data.RTP.AvailablePorts != 15 || payload.Data.RTP.PortAllocFailTotal != 2 {
		t.Fatalf("unexpected network status payload: %+v", payload.Data)
	}

	events, err := audit.List(t.Context(), observability.AuditQuery{Who: "net-ops", Limit: 10})
	if err != nil {
		t.Fatalf("query audit failed: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected audit event for network status query")
	}
	if events[0].OpsAction != "QUERY_NODE_NETWORK_STATUS" {
		t.Fatalf("unexpected ops action: %s", events[0].OpsAction)
	}
}

func TestCapacityRecommendationEndpoint(t *testing.T) {
	h, _, audit := buildTestHandler(t)
	summary := `{"run_id":"r1","generated_at":"2026-01-01T00:00:00Z","config":{"Targets":null,"Concurrency":0,"QPS":0,"Duration":0,"FileSize":0,"ChunkSize":0,"TransferMode":"","SIPAddress":"","RTPAddress":"","HTTPURL":"","OutputDir":"","Timeout":0},"summaries":{"sip-command-create":{"target":"sip-command-create","total":1000,"success":998,"failed":2,"success_rate":0.998,"throughput_qps":220,"p50_ms":30,"p95_ms":120,"p99_ms":180,"error_types":{},"elapsed_ms":10000,"concurrency":120,"configured_qps":300},"rtp-udp-upload":{"target":"rtp-udp-upload","total":600,"success":598,"failed":2,"success_rate":0.996,"throughput_qps":80,"p50_ms":100,"p95_ms":220,"p99_ms":350,"error_types":{},"elapsed_ms":10000,"concurrency":50,"configured_qps":0}},"result_file":"/tmp/results.jsonl"}`
	path := t.TempDir() + "/summary.json"
	if err := os.WriteFile(path, []byte(summary), 0o644); err != nil {
		t.Fatalf("write summary failed: %v", err)
	}
	body := `{"summary_file":"` + path + `","current":{"command_max_concurrent":90,"file_transfer_max_concurrent":40,"rtp_port_pool_size":120,"max_connections":150,"rate_limit_rps":280,"rate_limit_burst":420}}`
	req := httptest.NewRequest(http.MethodPost, "/api/capacity/recommendation", bytes.NewBufferString(body))
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
	if !strings.Contains(payload.Data.FileName, "req_req-d") || !strings.Contains(payload.Data.OutputDir, "trace_trace-d") {
		t.Fatalf("unexpected naming: file=%s dir=%s", payload.Data.FileName, payload.Data.OutputDir)
	}
	if len(payload.Data.Files) < 7 {
		t.Fatalf("expected diagnostic files, got %d", len(payload.Data.Files))
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
}
