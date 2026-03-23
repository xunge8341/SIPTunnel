package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"siptunnel/internal/config"
	"siptunnel/internal/observability"
	filerepo "siptunnel/internal/repository/file"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/service"
	"siptunnel/internal/service/taskengine"
	"strings"
	"testing"
	"time"
)

func TestMappingsCRUD(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	port := reserveFreeMappingPort(t)
	body := fmt.Sprintf(`{"mapping_id":"map-2","name":"orders","enabled":true,"local_bind_ip":"127.0.0.1","local_bind_port":%d,"local_base_path":"/orders","remote_target_ip":"10.0.0.2","remote_target_port":8090,"remote_base_path":"/api/orders","allowed_methods":["GET","POST"],"connect_timeout_ms":500,"request_timeout_ms":3000,"response_timeout_ms":3000,"max_request_body_bytes":1048576,"max_response_body_bytes":1048576,"description":"orders mapping"}`,
		port,
	)

	createReq := httptest.NewRequest(http.MethodPost, "/api/mappings", bytes.NewBufferString(body))
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create mapping expected 201 got %d body=%s", createRR.Code, createRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/mappings", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK || !strings.Contains(listRR.Body.String(), "map-2") {
		t.Fatalf("list mappings failed code=%d body=%s", listRR.Code, listRR.Body.String())
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/mappings/map-2", bytes.NewBufferString(strings.ReplaceAll(body, "orders", "orders-v2")))
	updateRR := httptest.NewRecorder()
	h.ServeHTTP(updateRR, updateReq)
	if updateRR.Code != http.StatusOK || !strings.Contains(updateRR.Body.String(), "orders-v2") {
		t.Fatalf("update mapping failed code=%d body=%s", updateRR.Code, updateRR.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/mappings/map-2", nil)
	deleteRR := httptest.NewRecorder()
	h.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("delete mapping expected 200 got %d body=%s", deleteRR.Code, deleteRR.Body.String())
	}
}

func TestMappingRuntimeEnableDisableAndStatusWriteback(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	port := reserveFreeMappingPort(t)
	body := fmt.Sprintf(`{"mapping_id":"map-runtime","name":"runtime","enabled":true,"local_bind_ip":"127.0.0.1","local_bind_port":%d,"local_base_path":"/runtime","remote_target_ip":"10.0.0.9","remote_target_port":8090,"remote_base_path":"/api/runtime","allowed_methods":["POST"],"connect_timeout_ms":500,"request_timeout_ms":3000,"response_timeout_ms":3000,"max_request_body_bytes":1024,"max_response_body_bytes":1024,"description":"runtime"}`,
		port,
	)

	createReq := httptest.NewRequest(http.MethodPost, "/api/mappings", bytes.NewBufferString(body))
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create mapping expected 201 got %d body=%s", createRR.Code, createRR.Body.String())
	}

	if _, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
		t.Fatalf("expected runtime to occupy port %d when enabled", port)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/mappings", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list mappings expected 200 got %d body=%s", listRR.Code, listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), `"mapping_id":"map-runtime"`) || !strings.Contains(listRR.Body.String(), `"link_status":"listening"`) {
		t.Fatalf("expected runtime status writeback in list body=%s", listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), `"link_status_text":"监听中"`) || !strings.Contains(listRR.Body.String(), `"suggested_action"`) {
		t.Fatalf("expected chinese status diagnostics in list body=%s", listRR.Body.String())
	}

	disableBody := strings.Replace(body, `"enabled":true`, `"enabled":false`, 1)
	updateReq := httptest.NewRequest(http.MethodPut, "/api/mappings/map-runtime", bytes.NewBufferString(disableBody))
	updateRR := httptest.NewRecorder()
	h.ServeHTTP(updateRR, updateReq)
	if updateRR.Code != http.StatusOK {
		t.Fatalf("disable mapping expected 200 got %d body=%s", updateRR.Code, updateRR.Body.String())
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("expected runtime to release port %d when disabled, err=%v", port, err)
	}
	_ = ln.Close()
}

func TestMappingRuntimePortConflict(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	conflictPort := reserveFreeMappingPort(t)
	occupied, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", conflictPort))
	if err != nil {
		t.Fatalf("prepare conflict listener failed: %v", err)
	}
	defer occupied.Close()

	body := fmt.Sprintf(`{"mapping_id":"map-conflict","name":"conflict","enabled":true,"local_bind_ip":"127.0.0.1","local_bind_port":%d,"local_base_path":"/conflict","remote_target_ip":"10.0.0.3","remote_target_port":8091,"remote_base_path":"/api/conflict","allowed_methods":["GET"],"connect_timeout_ms":500,"request_timeout_ms":3000,"response_timeout_ms":3000,"max_request_body_bytes":1024,"max_response_body_bytes":1024,"description":"conflict"}`,
		conflictPort,
	)

	createReq := httptest.NewRequest(http.MethodPost, "/api/mappings", bytes.NewBufferString(body))
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create mapping expected 201 got %d body=%s", createRR.Code, createRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/mappings", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list mappings expected 200 got %d body=%s", listRR.Code, listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), `"mapping_id":"map-conflict"`) || !strings.Contains(listRR.Body.String(), `"link_status":"start_failed"`) {
		t.Fatalf("expected start_failed status on conflict body=%s", listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), "端口冲突") && !strings.Contains(listRR.Body.String(), "启动监听失败") {
		t.Fatalf("expected conflict reason body=%s", listRR.Body.String())
	}
}

func TestMappingTestEndpoint(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/mapping/test", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/mapping/test expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Code string              `json:"code"`
		Data MappingTestResponse `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if payload.Code != "OK" {
		t.Fatalf("unexpected code: %s", payload.Code)
	}
	if strings.TrimSpace(payload.Data.SignalingRequest) == "" {
		t.Fatalf("expected signaling_request to be populated")
	}
	if payload.Data.ResponseChannel != "正常" && payload.Data.ResponseChannel != "异常" {
		t.Fatalf("unexpected response_channel=%s", payload.Data.ResponseChannel)
	}
	if payload.Data.RegistrationStatus != "未注册" && payload.Data.RegistrationStatus != "正常" {
		t.Fatalf("unexpected registration_status=%s", payload.Data.RegistrationStatus)
	}
	if len(payload.Data.Stages) != 6 {
		t.Fatalf("expected 6 staged checks, got %d", len(payload.Data.Stages))
	}
	if payload.Data.Stages[0].Key != "local_listening" || payload.Data.Stages[3].Key != "peer_reachability" || payload.Data.Stages[5].Key != "mapping_forward" {
		t.Fatalf("unexpected stage sequence: %+v", payload.Data.Stages)
	}
	if !payload.Data.Passed && (payload.Data.FailureStage == "" || payload.Data.FailureReason == "" || payload.Data.SuggestedAction == "") {
		t.Fatalf("expected stage diagnostics for failed result, got %+v", payload.Data)
	}
}

func TestMappingsCapabilityValidationErrorOnCreate(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	body := `{"mapping_id":"map-large","name":"orders","enabled":true,"local_bind_ip":"127.0.0.1","local_bind_port":21090,"local_base_path":"/orders","remote_target_ip":"10.0.0.2","remote_target_port":8090,"remote_base_path":"/api/orders","allowed_methods":["POST"],"connect_timeout_ms":500,"request_timeout_ms":3000,"response_timeout_ms":3000,"max_request_body_bytes":2097152,"max_response_body_bytes":1048576,"description":"orders mapping"}`

	req := httptest.NewRequest(http.MethodPost, "/api/mappings", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "MAPPING_CAPABILITY_INVALID") {
		t.Fatalf("expected capability error code, got %s", rr.Body.String())
	}
}

func TestMappingsCapabilityWarningsInAPIAndSelfCheck(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	body := `{"mapping_id":"map-warn","name":"orders","enabled":true,"local_bind_ip":"127.0.0.1","local_bind_port":21090,"local_base_path":"/orders","remote_target_ip":"10.0.0.2","remote_target_port":8090,"remote_base_path":"/api/orders","allowed_methods":["PUT"],"connect_timeout_ms":500,"request_timeout_ms":3000,"response_timeout_ms":3000,"max_request_body_bytes":1048576,"max_response_body_bytes":1048576,"description":"orders mapping"}`

	createReq := httptest.NewRequest(http.MethodPost, "/api/mappings", bytes.NewBufferString(body))
	createRR := httptest.NewRecorder()
	h.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected create success, got %d body=%s", createRR.Code, createRR.Body.String())
	}
	if !strings.Contains(createRR.Body.String(), "warnings") {
		t.Fatalf("expected warnings in create response: %s", createRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/mappings", nil)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected list success, got %d body=%s", listRR.Code, listRR.Body.String())
	}
	if !strings.Contains(listRR.Body.String(), "warnings") {
		t.Fatalf("expected warnings in list response: %s", listRR.Body.String())
	}

	selfReq := httptest.NewRequest(http.MethodGet, "/api/selfcheck", nil)
	selfRR := httptest.NewRecorder()
	h.ServeHTTP(selfRR, selfReq)
	if selfRR.Code != http.StatusOK {
		t.Fatalf("expected selfcheck success, got %d body=%s", selfRR.Code, selfRR.Body.String())
	}
	if !strings.Contains(selfRR.Body.String(), "mappings_capability_validation") {
		t.Fatalf("expected mappings capability selfcheck item: %s", selfRR.Body.String())
	}
}

func TestMappingsRejectWhenNoEnabledPeerConfigured(t *testing.T) {
	repo := memrepo.NewTaskRepository()
	audit := observability.NewInMemoryAuditStore()
	nodeStore, err := filerepo.NewNodeConfigStore(t.TempDir() + "/node_config.json")
	if err != nil {
		t.Fatalf("new node config store failed: %v", err)
	}
	mappingStore, err := filerepo.NewTunnelMappingStore(t.TempDir() + "/tunnel_mappings.json")
	if err != nil {
		t.Fatalf("new mapping store failed: %v", err)
	}
	deps := handlerDeps{
		logger:    observability.NewStructuredLogger(nil),
		audit:     audit,
		repo:      repo,
		engine:    taskengine.NewEngine(repo, service.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second}),
		mappings:  mappingStore,
		nodeStore: nodeStore,
		networkStatusFunc: func(_ context.Context) NodeNetworkStatus {
			capability := config.DeriveCapability(config.NetworkModeSenderSIPReceiverRTP)
			return NodeNetworkStatus{NetworkMode: config.NetworkModeSenderSIPReceiverRTP, Capability: capability}
		},
	}
	deps.sessionMgr = newTunnelSessionManager(&fakeRegistrar{registerCodes: []int{200}}, deps.tunnelConfig)
	h := newMux(deps)

	body := `{"mapping_id":"map-np","enabled":true,"local_bind_ip":"127.0.0.1","local_bind_port":21090,"local_base_path":"/orders","remote_target_ip":"10.0.0.2","remote_target_port":8090,"remote_base_path":"/api/orders","allowed_methods":["GET"],"connect_timeout_ms":500,"request_timeout_ms":3000,"response_timeout_ms":3000,"max_request_body_bytes":1024,"max_response_body_bytes":1024,"description":"orders mapping"}`
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/mappings", bytes.NewBufferString(body)))
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "PEER_BINDING_INVALID") {
		t.Fatalf("expected peer binding error, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMappingsRejectWhenMultipleEnabledPeersConfigured(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	createPeerBody := `{"peer_node_id":"34020000002000000022","peer_name":"Peer B2","peer_signaling_ip":"10.20.0.21","peer_signaling_port":5060,"peer_media_ip":"10.20.0.21","peer_media_port_start":32000,"peer_media_port_end":32100,"supported_network_mode":"SENDER_SIP__RECEIVER_RTP","enabled":true}`
	createPeer := httptest.NewRequest(http.MethodPost, "/api/peers", bytes.NewBufferString(createPeerBody))
	createPeerRR := httptest.NewRecorder()
	h.ServeHTTP(createPeerRR, createPeer)
	if createPeerRR.Code != http.StatusCreated {
		t.Fatalf("POST /api/peers expected 201 got %d body=%s", createPeerRR.Code, createPeerRR.Body.String())
	}

	body := `{"mapping_id":"map-multi","enabled":true,"local_bind_ip":"127.0.0.1","local_bind_port":21090,"local_base_path":"/orders","remote_target_ip":"10.0.0.2","remote_target_port":8090,"remote_base_path":"/api/orders","allowed_methods":["GET"],"connect_timeout_ms":500,"request_timeout_ms":3000,"response_timeout_ms":3000,"max_request_body_bytes":1024,"max_response_body_bytes":1024,"description":"orders mapping"}`
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/mappings", bytes.NewBufferString(body)))
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "PEER_BINDING_INVALID") {
		t.Fatalf("expected peer binding error, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMappingsListIncludesBoundPeerAndBindingErrors(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, httptest.NewRequest(http.MethodGet, "/api/mappings", nil))
	if listRR.Code != http.StatusOK || !strings.Contains(listRR.Body.String(), "bound_peer") || !strings.Contains(listRR.Body.String(), "34020000002000000002") {
		t.Fatalf("expected bound peer in list, got %d body=%s", listRR.Code, listRR.Body.String())
	}

	createPeerBody := `{"peer_node_id":"34020000002000000023","peer_name":"Peer B3","peer_signaling_ip":"10.20.0.22","peer_signaling_port":5060,"peer_media_ip":"10.20.0.22","peer_media_port_start":32000,"peer_media_port_end":32100,"supported_network_mode":"SENDER_SIP__RECEIVER_RTP","enabled":true}`
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/peers", bytes.NewBufferString(createPeerBody)))
	listConflictRR := httptest.NewRecorder()
	h.ServeHTTP(listConflictRR, httptest.NewRequest(http.MethodGet, "/api/mappings", nil))
	if listConflictRR.Code != http.StatusOK || !strings.Contains(listConflictRR.Body.String(), "binding_error") {
		t.Fatalf("expected binding_error in list, got %d body=%s", listConflictRR.Code, listConflictRR.Body.String())
	}
}
