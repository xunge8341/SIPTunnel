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

	"siptunnel/internal/config"
	"siptunnel/internal/observability"
	"siptunnel/internal/repository"
	filerepo "siptunnel/internal/repository/file"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/service"
	"siptunnel/internal/service/taskengine"
	"siptunnel/internal/startupsummary"
)

func buildTestHandler(t *testing.T) (http.Handler, repository.TaskRepository, observability.AuditStore) {
	t.Helper()
	repo := memrepo.NewTaskRepository()
	audit := observability.NewInMemoryAuditStore()
	nodeStore, err := filerepo.NewNodeConfigStore(t.TempDir() + "/node_config.json")
	if err != nil {
		t.Fatalf("new node config store failed: %v", err)
	}
	deps := handlerDeps{
		logger:    observability.NewStructuredLogger(nil),
		audit:     audit,
		repo:      repo,
		engine:    taskengine.NewEngine(repo, service.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second}),
		limits:    OpsLimits{RPS: 100, Burst: 200, MaxConcurrent: 50},
		routes:    map[string]OpsRoute{"asset.sync": {APICode: "asset.sync", HTTPMethod: "POST", HTTPPath: "/sync", Enabled: true}},
		nodes:     []OpsNode{{NodeID: "n1", Role: "gateway", Status: "ready", Endpoint: "127.0.0.1:18080"}},
		nodeStore: nodeStore,
		selfCheckProvider: func(_ context.Context) selfcheck.Report {
			return selfcheck.Report{Overall: selfcheck.LevelInfo, Summary: selfcheck.Summary{Info: 1, Warn: 1, Error: 1}, Items: []selfcheck.Item{{Name: "sample-info", Level: selfcheck.LevelInfo, Message: "ok", Suggestion: "none", ActionHint: "keep"}, {Name: "sample-warn", Level: selfcheck.LevelWarn, Message: "warn", Suggestion: "check", ActionHint: "verify"}, {Name: "sample-error", Level: selfcheck.LevelError, Message: "err", Suggestion: "fix", ActionHint: "recover"}}}
		},
		startupSummaryFn: func(_ context.Context) startupsummary.Summary {
			capability := config.DeriveCapability(config.NetworkModeAToBSIPBToARTP)
			return startupsummary.Summary{NodeID: "n1", NetworkMode: config.NetworkModeAToBSIPBToARTP, Capability: capability, CapabilitySummary: startupsummary.CapabilitySummary{Supported: capability.SupportedFeatures(), Unsupported: capability.UnsupportedFeatures(), Items: capability.Matrix()}, TransportPlan: config.ResolveTransportPlan(config.NetworkModeAToBSIPBToARTP, capability), ConfigPath: "./configs/config.yaml", ConfigSource: "cli", UIMode: "embedded", UIURL: "http://127.0.0.1:18080/", APIURL: "http://127.0.0.1:18080/api"}
		},
		networkStatusFunc: func(_ context.Context) NodeNetworkStatus {
			capability := config.DeriveCapability(config.NetworkModeAToBSIPBToARTP)
			return NodeNetworkStatus{
				NetworkMode:         config.NetworkModeAToBSIPBToARTP,
				Capability:          capability,
				CapabilitySummary:   startupsummary.CapabilitySummary{Supported: capability.SupportedFeatures(), Unsupported: capability.UnsupportedFeatures(), Items: capability.Matrix()},
				TransportPlan:       config.ResolveTransportPlan(config.NetworkModeAToBSIPBToARTP, capability),
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
		{http.MethodGet, "/api/startup-summary", "", http.StatusOK},
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
	if payload.Data.NetworkMode != config.NetworkModeAToBSIPBToARTP || !payload.Data.Capability.SupportsLargeResponseBody || payload.Data.Capability.SupportsLargeRequestBody {
		t.Fatalf("unexpected mode/capability payload: %+v", payload.Data)
	}
	if payload.Data.TransportPlan.RequestBodyTransport != config.TransportSIPBodyOnly || payload.Data.TransportPlan.ResponseBodyTransport != config.TransportRTPStream {
		t.Fatalf("unexpected transport plan payload: %+v", payload.Data.TransportPlan)
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
	if !strings.Contains(body, `"warn":1`) || !strings.Contains(body, `"error":1`) {
		t.Fatalf("expected filtered summary counts: %s", body)
	}
}

func TestLinkTestEndpoint(t *testing.T) {
	h, _, audit := buildTestHandler(t)

	runReq := httptest.NewRequest(http.MethodPost, "/api/ops/link-test", nil)
	runReq.Header.Set("X-Initiator", "ops-link")
	runRR := httptest.NewRecorder()
	h.ServeHTTP(runRR, runReq)
	if runRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", runRR.Code, runRR.Body.String())
	}
	if !strings.Contains(runRR.Body.String(), "request_id") || !strings.Contains(runRR.Body.String(), "trace_id") {
		t.Fatalf("missing ids in response: %s", runRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/ops/link-test", nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRR.Code, getRR.Body.String())
	}
	if !strings.Contains(getRR.Body.String(), "http_downstream") {
		t.Fatalf("unexpected response: %s", getRR.Body.String())
	}

	events, err := audit.List(t.Context(), observability.AuditQuery{Who: "ops-link", Limit: 10})
	if err != nil {
		t.Fatalf("query audit failed: %v", err)
	}
	if len(events) == 0 || events[0].OpsAction != "RUN_LINK_TEST" {
		t.Fatalf("unexpected audit events: %+v", events)
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

func TestNodeAndPeerEndpoints(t *testing.T) {
	h, _, _ := buildTestHandler(t)

	getNode := httptest.NewRequest(http.MethodGet, "/api/node", nil)
	getNodeRR := httptest.NewRecorder()
	h.ServeHTTP(getNodeRR, getNode)
	if getNodeRR.Code != http.StatusOK {
		t.Fatalf("GET /api/node expected 200 got %d body=%s", getNodeRR.Code, getNodeRR.Body.String())
	}

	putNodeBody := `{"node_id":"gateway-a-01","node_name":"Gateway-A-Updated","node_role":"gateway","network_mode":"A_TO_B_SIP__B_TO_A_RTP","sip_listen_ip":"10.10.1.10","sip_listen_port":5060,"sip_transport":"TCP","rtp_listen_ip":"10.10.1.10","rtp_port_start":30000,"rtp_port_end":30100,"rtp_transport":"UDP"}`
	putNode := httptest.NewRequest(http.MethodPut, "/api/node", bytes.NewBufferString(putNodeBody))
	putNodeRR := httptest.NewRecorder()
	h.ServeHTTP(putNodeRR, putNode)
	if putNodeRR.Code != http.StatusOK {
		t.Fatalf("PUT /api/node expected 200 got %d body=%s", putNodeRR.Code, putNodeRR.Body.String())
	}

	createPeerBody := `{"peer_node_id":"peer-b-01","peer_name":"Peer B","peer_signaling_ip":"10.20.0.20","peer_signaling_port":5060,"peer_media_ip":"10.20.0.20","peer_media_port_start":32000,"peer_media_port_end":32100,"supported_network_mode":"A_TO_B_SIP__B_TO_A_RTP","enabled":true}`
	createPeer := httptest.NewRequest(http.MethodPost, "/api/peers", bytes.NewBufferString(createPeerBody))
	createPeerRR := httptest.NewRecorder()
	h.ServeHTTP(createPeerRR, createPeer)
	if createPeerRR.Code != http.StatusCreated {
		t.Fatalf("POST /api/peers expected 201 got %d body=%s", createPeerRR.Code, createPeerRR.Body.String())
	}

	listPeers := httptest.NewRequest(http.MethodGet, "/api/peers", nil)
	listPeersRR := httptest.NewRecorder()
	h.ServeHTTP(listPeersRR, listPeers)
	if listPeersRR.Code != http.StatusOK || !strings.Contains(listPeersRR.Body.String(), "peer-b-01") {
		t.Fatalf("GET /api/peers failed code=%d body=%s", listPeersRR.Code, listPeersRR.Body.String())
	}

	updatePeerBody := `{"peer_name":"Peer B2","peer_signaling_ip":"10.20.0.20","peer_signaling_port":5061,"peer_media_ip":"10.20.0.21","peer_media_port_start":32010,"peer_media_port_end":32110,"supported_network_mode":"A_TO_B_SIP__B_TO_A_RTP","enabled":false}`
	updatePeer := httptest.NewRequest(http.MethodPut, "/api/peers/peer-b-01", bytes.NewBufferString(updatePeerBody))
	updatePeerRR := httptest.NewRecorder()
	h.ServeHTTP(updatePeerRR, updatePeer)
	if updatePeerRR.Code != http.StatusOK {
		t.Fatalf("PUT /api/peers/{id} expected 200 got %d body=%s", updatePeerRR.Code, updatePeerRR.Body.String())
	}

	deletePeer := httptest.NewRequest(http.MethodDelete, "/api/peers/peer-b-01", nil)
	deletePeerRR := httptest.NewRecorder()
	h.ServeHTTP(deletePeerRR, deletePeer)
	if deletePeerRR.Code != http.StatusOK {
		t.Fatalf("DELETE /api/peers/{id} expected 200 got %d body=%s", deletePeerRR.Code, deletePeerRR.Body.String())
	}
}

func TestNodePeerPersistenceAcrossHandlerRestart(t *testing.T) {
	dataDir := t.TempDir()
	h1, closer1, err := NewHandlerWithOptions(HandlerOptions{DataDir: dataDir})
	if err != nil {
		t.Fatalf("new handler1 failed: %v", err)
	}
	if closer1 != nil {
		defer closer1.Close()
	}

	putNodeBody := `{"node_id":"gateway-a-01","node_name":"Persisted Node","node_role":"gateway","network_mode":"A_TO_B_SIP__B_TO_A_RTP","sip_listen_ip":"10.10.1.11","sip_listen_port":5060,"sip_transport":"TCP","rtp_listen_ip":"10.10.1.11","rtp_port_start":30000,"rtp_port_end":30010,"rtp_transport":"UDP"}`
	rrPut := httptest.NewRecorder()
	h1.ServeHTTP(rrPut, httptest.NewRequest(http.MethodPut, "/api/node", bytes.NewBufferString(putNodeBody)))
	if rrPut.Code != http.StatusOK {
		t.Fatalf("update node failed: %d body=%s", rrPut.Code, rrPut.Body.String())
	}

	createPeerBody := `{"peer_node_id":"persist-peer","peer_name":"Persist Peer","peer_signaling_ip":"10.20.0.30","peer_signaling_port":5060,"peer_media_ip":"10.20.0.30","peer_media_port_start":33000,"peer_media_port_end":33010,"supported_network_mode":"A_TO_B_SIP__B_TO_A_RTP","enabled":true}`
	rrCreate := httptest.NewRecorder()
	h1.ServeHTTP(rrCreate, httptest.NewRequest(http.MethodPost, "/api/peers", bytes.NewBufferString(createPeerBody)))
	if rrCreate.Code != http.StatusCreated {
		t.Fatalf("create peer failed: %d body=%s", rrCreate.Code, rrCreate.Body.String())
	}

	h2, closer2, err := NewHandlerWithOptions(HandlerOptions{DataDir: dataDir})
	if err != nil {
		t.Fatalf("new handler2 failed: %v", err)
	}
	if closer2 != nil {
		defer closer2.Close()
	}

	rrNode := httptest.NewRecorder()
	h2.ServeHTTP(rrNode, httptest.NewRequest(http.MethodGet, "/api/node", nil))
	if rrNode.Code != http.StatusOK || !strings.Contains(rrNode.Body.String(), "Persisted Node") {
		t.Fatalf("persisted node not found code=%d body=%s", rrNode.Code, rrNode.Body.String())
	}
	rrPeers := httptest.NewRecorder()
	h2.ServeHTTP(rrPeers, httptest.NewRequest(http.MethodGet, "/api/peers", nil))
	if rrPeers.Code != http.StatusOK || !strings.Contains(rrPeers.Body.String(), "persist-peer") {
		t.Fatalf("persisted peers not found code=%d body=%s", rrPeers.Code, rrPeers.Body.String())
	}
}
