package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"siptunnel/internal/config"
	"siptunnel/internal/observability"
	filerepo "siptunnel/internal/repository/file"
	memrepo "siptunnel/internal/repository/memory"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/service"
	"siptunnel/internal/service/taskengine"
	"strings"
	"testing"
	"time"
)

func TestNodeConfigEndpointSaveAndLoad(t *testing.T) {
	h, _, _ := buildTestHandler(t)

	postBody := `{"local_node":{"node_ip":"10.10.1.11","signaling_port":6060,"device_id":"34020000002000000031","rtp_port_start":21000,"rtp_port_end":21099},"peer_node":{"node_ip":"10.20.1.11","signaling_port":7060,"device_id":"34020000002000000032"}}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/node/config", bytes.NewBufferString(postBody))
	postRR := httptest.NewRecorder()
	h.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK || !strings.Contains(postRR.Body.String(), "tunnel_restarted") {
		t.Fatalf("POST /api/node/config failed code=%d body=%s", postRR.Code, postRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/node/config", nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK || !strings.Contains(getRR.Body.String(), "34020000002000000031") || !strings.Contains(getRR.Body.String(), "34020000002000000032") {
		t.Fatalf("GET /api/node/config failed code=%d body=%s", getRR.Code, getRR.Body.String())
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

func TestNodeAndPeerEndpoints(t *testing.T) {
	h, _, _ := buildTestHandler(t)

	getNode := httptest.NewRequest(http.MethodGet, "/api/node", nil)
	getNodeRR := httptest.NewRecorder()
	h.ServeHTTP(getNodeRR, getNode)
	if getNodeRR.Code != http.StatusOK {
		t.Fatalf("GET /api/node expected 200 got %d body=%s", getNodeRR.Code, getNodeRR.Body.String())
	}
	if !strings.Contains(getNodeRR.Body.String(), "current_network_mode") || !strings.Contains(getNodeRR.Body.String(), "compatibility_status") {
		t.Fatalf("GET /api/node missing compatibility fields: %s", getNodeRR.Body.String())
	}

	putNodeBody := `{"node_id":"34020000002000000001","node_name":"Gateway-A-Updated","node_role":"gateway","network_mode":"SENDER_SIP__RECEIVER_RTP","sip_listen_ip":"10.10.1.10","sip_listen_port":5060,"sip_transport":"TCP","rtp_listen_ip":"10.10.1.10","rtp_port_start":30000,"rtp_port_end":30100,"rtp_transport":"UDP"}`
	putNode := httptest.NewRequest(http.MethodPut, "/api/node", bytes.NewBufferString(putNodeBody))
	putNodeRR := httptest.NewRecorder()
	h.ServeHTTP(putNodeRR, putNode)
	if putNodeRR.Code != http.StatusOK {
		t.Fatalf("PUT /api/node expected 200 got %d body=%s", putNodeRR.Code, putNodeRR.Body.String())
	}

	incompatiblePeerBody := `{"peer_node_id":"34020000002000000024","peer_name":"Peer B Bad","peer_signaling_ip":"10.20.0.20","peer_signaling_port":5060,"peer_media_ip":"10.20.0.20","peer_media_port_start":32000,"peer_media_port_end":32100,"supported_network_mode":"SENDER_SIP_RTP__RECEIVER_SIP_RTP","enabled":true}`
	incompatiblePeer := httptest.NewRequest(http.MethodPost, "/api/peers", bytes.NewBufferString(incompatiblePeerBody))
	incompatiblePeerRR := httptest.NewRecorder()
	h.ServeHTTP(incompatiblePeerRR, incompatiblePeer)
	if incompatiblePeerRR.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/peers incompatible expected 400 got %d body=%s", incompatiblePeerRR.Code, incompatiblePeerRR.Body.String())
	}
	createPeerBody := `{"peer_node_id":"34020000002000000021","peer_name":"Peer B","peer_signaling_ip":"10.20.0.20","peer_signaling_port":5060,"peer_media_ip":"10.20.0.20","peer_media_port_start":32000,"peer_media_port_end":32100,"supported_network_mode":"SENDER_SIP__RECEIVER_RTP","enabled":true}`
	createPeer := httptest.NewRequest(http.MethodPost, "/api/peers", bytes.NewBufferString(createPeerBody))
	createPeerRR := httptest.NewRecorder()
	h.ServeHTTP(createPeerRR, createPeer)
	if createPeerRR.Code != http.StatusCreated {
		t.Fatalf("POST /api/peers expected 201 got %d body=%s", createPeerRR.Code, createPeerRR.Body.String())
	}

	listPeers := httptest.NewRequest(http.MethodGet, "/api/peers", nil)
	listPeersRR := httptest.NewRecorder()
	h.ServeHTTP(listPeersRR, listPeers)
	if listPeersRR.Code != http.StatusOK || !strings.Contains(listPeersRR.Body.String(), "34020000002000000002") {
		t.Fatalf("GET /api/peers failed code=%d body=%s", listPeersRR.Code, listPeersRR.Body.String())
	}

	updatePeerBody := `{"peer_name":"Peer B2","peer_signaling_ip":"10.20.0.20","peer_signaling_port":5061,"peer_media_ip":"10.20.0.21","peer_media_port_start":32010,"peer_media_port_end":32110,"supported_network_mode":"SENDER_SIP__RECEIVER_RTP","enabled":false}`
	updatePeer := httptest.NewRequest(http.MethodPut, "/api/peers/34020000002000000021", bytes.NewBufferString(updatePeerBody))
	updatePeerRR := httptest.NewRecorder()
	h.ServeHTTP(updatePeerRR, updatePeer)
	if updatePeerRR.Code != http.StatusOK {
		t.Fatalf("PUT /api/peers/{id} expected 200 got %d body=%s", updatePeerRR.Code, updatePeerRR.Body.String())
	}

	deletePeer := httptest.NewRequest(http.MethodDelete, "/api/peers/34020000002000000021", nil)
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

	putNodeBody := `{"node_id":"34020000002000000001","node_name":"Persisted Node","node_role":"gateway","network_mode":"SENDER_SIP__RECEIVER_RTP","sip_listen_ip":"10.10.1.11","sip_listen_port":5060,"sip_transport":"TCP","rtp_listen_ip":"10.10.1.11","rtp_port_start":30000,"rtp_port_end":30010,"rtp_transport":"UDP"}`
	rrPut := httptest.NewRecorder()
	h1.ServeHTTP(rrPut, httptest.NewRequest(http.MethodPut, "/api/node", bytes.NewBufferString(putNodeBody)))
	if rrPut.Code != http.StatusOK {
		t.Fatalf("update node failed: %d body=%s", rrPut.Code, rrPut.Body.String())
	}

	createPeerBody := `{"peer_node_id":"persist-peer","peer_name":"Persist Peer","peer_signaling_ip":"10.20.0.30","peer_signaling_port":5060,"peer_media_ip":"10.20.0.30","peer_media_port_start":33000,"peer_media_port_end":33010,"supported_network_mode":"SENDER_SIP__RECEIVER_RTP","enabled":true}`
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

func TestNodesReadFromNodeStoreNotHardcoded(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "34020000002000000001") {
		t.Fatalf("expected node id from node store, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "data_source") || !strings.Contains(rr.Body.String(), "node_config.json") {
		t.Fatalf("expected node data source annotation, got %s", rr.Body.String())
	}
}

func TestTunnelConfigEndpointGetAndPost(t *testing.T) {
	h, _, _ := buildTestHandler(t)

	getReq := httptest.NewRequest(http.MethodGet, "/api/tunnel/config", nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK || !strings.Contains(getRR.Body.String(), "capability_items") {
		t.Fatalf("GET /api/tunnel/config failed code=%d body=%s", getRR.Code, getRR.Body.String())
	}

	postBody := `{"channel_protocol":"GB/T 28181","connection_initiator":"LOCAL","heartbeat_interval_sec":30,"register_retry_count":5,"register_retry_interval_sec":10,"network_mode":"SENDER_SIP_RTP__RECEIVER_SIP_RTP"}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/tunnel/config", bytes.NewBufferString(postBody))
	postRR := httptest.NewRecorder()
	h.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK || !strings.Contains(postRR.Body.String(), "supports_large_request_body") {
		t.Fatalf("POST /api/tunnel/config failed code=%d body=%s", postRR.Code, postRR.Body.String())
	}

	getAfterReq := httptest.NewRequest(http.MethodGet, "/api/tunnel/config", nil)
	getAfterRR := httptest.NewRecorder()
	h.ServeHTTP(getAfterRR, getAfterReq)
	if getAfterRR.Code != http.StatusOK || !strings.Contains(getAfterRR.Body.String(), "SENDER_SIP_RTP__RECEIVER_SIP_RTP") {
		t.Fatalf("GET after POST /api/tunnel/config failed code=%d body=%s", getAfterRR.Code, getAfterRR.Body.String())
	}
	if !strings.Contains(getAfterRR.Body.String(), "\"local_device_id\":\"34020000002000000001\"") {
		t.Fatalf("expected local_device_id derived from node config, body=%s", getAfterRR.Body.String())
	}
	if !strings.Contains(getAfterRR.Body.String(), "\"peer_device_id\":\"34020000002000000002\"") {
		t.Fatalf("expected peer_device_id derived from node config, body=%s", getAfterRR.Body.String())
	}
}

func TestTunnelSessionActionEndpoint(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	body := `{"action":"register_now"}`
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/tunnel/session/actions", bytes.NewBufferString(body)))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "register_now") {
		t.Fatalf("unexpected response code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSelfCheckNoPeerWithoutMappingsRemainsNonBlocking(t *testing.T) {
	repo := memrepo.NewTaskRepository()
	audit := observability.NewInMemoryAuditStore()
	nodeStore, err := filerepo.NewNodeConfigStore(t.TempDir() + "/node_config.json")
	if err != nil {
		t.Fatalf("new node config store failed: %v", err)
	}
	mappingStore, err := filerepo.NewTunnelMappingStore(t.TempDir() + "/tunnel_mappings.json")
	if err != nil {
		t.Fatalf("new tunnel mapping store failed: %v", err)
	}
	h := newMux(handlerDeps{
		logger:    observability.NewStructuredLogger(nil),
		audit:     audit,
		repo:      repo,
		engine:    taskengine.NewEngine(repo, service.RetryPolicy{MaxAttempts: 3, BaseBackoff: time.Second}),
		limits:    OpsLimits{RPS: 100, Burst: 200, MaxConcurrent: 50},
		mappings:  mappingStore,
		nodeStore: nodeStore,
		selfCheckProvider: func(_ context.Context) selfcheck.Report {
			return selfcheck.Report{Overall: selfcheck.LevelInfo, Summary: selfcheck.Summary{Info: 1}, Items: []selfcheck.Item{{Name: "base", Level: selfcheck.LevelInfo, Message: "ok"}}}
		},
		networkStatusFunc: func(_ context.Context) NodeNetworkStatus {
			return NodeNetworkStatus{NetworkMode: config.NetworkModeSenderSIPReceiverRTP, SIP: SIPNetworkStatus{ListenIP: "127.0.0.1", ListenPort: 5060, Transport: "UDP"}, RTP: RTPNetworkStatus{ListenIP: "127.0.0.1", PortStart: 20000, PortEnd: 20101, Transport: "UDP", AvailablePorts: 102, PortPoolTotal: 102}}
		},
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/selfcheck", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected selfcheck success, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, `"overall":"error"`) {
		t.Fatalf("expected non-blocking selfcheck overall, got %s", body)
	}
	if !strings.Contains(body, `"name":"mapping_peer_binding"`) {
		t.Fatalf("expected mapping_peer_binding item, got %s", body)
	}
	if !strings.Contains(body, `"level":"info"`) {
		t.Fatalf("expected info level for empty peer binding state, got %s", body)
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
	if payload.Data.CurrentNetworkMode != config.NetworkModeSenderSIPReceiverRTP || payload.Data.CompatibilityStatus.Level == "" {
		t.Fatalf("compatibility fields missing in network status: %+v", payload.Data)
	}
	if payload.Data.SIP.ListenIP != "10.10.1.10" || payload.Data.RTP.AvailablePorts != 15 || payload.Data.RTP.PortAllocFailTotal != 2 {
		t.Fatalf("unexpected network status payload: %+v", payload.Data)
	}
	if payload.Data.NetworkMode != config.NetworkModeSenderSIPReceiverRTP || !payload.Data.Capability.SupportsLargeResponseBody || payload.Data.Capability.SupportsLargeRequestBody {
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

func TestGatewayRestartEndpoint(t *testing.T) {
	t.Setenv("SIPTUNNEL_RESTART_COMMAND", "true")
	h, _, audit := buildTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/gateway/restart", nil)
	req.Header.Set("X-Initiator", "ops-admin")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "accepted") || !strings.Contains(rr.Body.String(), "scheduled_at") {
		t.Fatalf("unexpected body: %s", rr.Body.String())
	}
	events, err := audit.List(t.Context(), observability.AuditQuery{Who: "ops-admin", Limit: 10})
	if err != nil {
		t.Fatalf("query audit failed: %v", err)
	}
	if len(events) == 0 || events[0].OpsAction != "RESTART_GATEWAY" {
		t.Fatalf("unexpected audit events: %+v", events)
	}
}
