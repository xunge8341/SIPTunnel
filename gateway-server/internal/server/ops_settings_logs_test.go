package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setLoopbackRemoteAddr(req *http.Request) {
	if req != nil {
		req.RemoteAddr = "127.0.0.1:12345"
	}
}

func TestContractEndpoints(t *testing.T) {
	h, _, _ := buildTestHandler(t)

	for _, path := range []string{"/api/access-logs?page=1&page_size=10&slow_only=true", "/api/system/settings", "/api/dashboard/ops-summary", "/api/dashboard/summary", "/api/protection/state", "/api/security/state", "/api/node-tunnel/workspace", "/api/tunnel/catalog", "/api/tunnel/gb28181/state"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s expected 200 got %d body=%s", path, rr.Code, rr.Body.String())
		}
	}
}

func TestSystemSettingsSaveAndReload(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	payload := map[string]any{
		"sqlite_path":                                 "./tmp.db",
		"log_cleanup_cron":                            "*/5 * * * *",
		"max_task_age_days":                           1,
		"max_task_records":                            100,
		"max_access_log_age_days":                     1,
		"max_access_log_records":                      100,
		"max_audit_age_days":                          1,
		"max_audit_records":                           100,
		"max_diagnostic_age_days":                     1,
		"max_diagnostic_records":                      100,
		"max_loadtest_age_days":                       1,
		"max_loadtest_records":                        100,
		"admin_allow_cidr":                            "127.0.0.1/32",
		"admin_require_mfa":                           false,
		"generic_download_total_mbps":                 24,
		"generic_download_per_transfer_mbps":          8,
		"generic_download_window_mb":                  2,
		"adaptive_hot_cache_mb":                       32,
		"adaptive_hot_window_mb":                      16,
		"generic_download_segment_concurrency":        2,
		"generic_download_rtp_reorder_window_packets": 512,
		"generic_download_rtp_loss_tolerance_packets": 128,
		"generic_download_rtp_gap_timeout_ms":         1200,
		"generic_download_rtp_fec_enabled":            true,
		"generic_download_rtp_fec_group_packets":      8,
		"cleaner_last_run_at":                         "",
		"cleaner_last_result":                         "ok",
		"cleaner_last_removed_records":                0,
	}
	body, _ := json.Marshal(payload)
	saveReq := httptest.NewRequest(http.MethodPost, "/api/system/settings", bytes.NewReader(body))
	setLoopbackRemoteAddr(saveReq)
	saveRR := httptest.NewRecorder()
	h.ServeHTTP(saveRR, saveReq)
	if saveRR.Code != http.StatusOK {
		t.Fatalf("save expected 200 got %d", saveRR.Code)
	}
	getReq := httptest.NewRequest(http.MethodGet, "/api/system/settings", nil)
	setLoopbackRemoteAddr(getReq)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK || !bytes.Contains(getRR.Body.Bytes(), []byte("./tmp.db")) {
		t.Fatalf("reload failed body=%s", getRR.Body.String())
	}
	for _, needle := range []string{"\"generic_download_total_mbps\":24", "\"generic_download_rtp_fec_enabled\":true", "\"generic_download_rtp_reorder_window_packets\":512"} {
		if !strings.Contains(getRR.Body.String(), needle) {
			t.Fatalf("expected reload body to contain %s, got %s", needle, getRR.Body.String())
		}
	}
}

func TestSystemSettingsRejectsRequireMFAWithoutConfiguredCode(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	payload := map[string]any{
		"sqlite_path":                  "./tmp.db",
		"log_cleanup_cron":             "*/5 * * * *",
		"max_task_age_days":            1,
		"max_task_records":             100,
		"max_access_log_age_days":      1,
		"max_access_log_records":       100,
		"max_audit_age_days":           1,
		"max_audit_records":            100,
		"max_diagnostic_age_days":      1,
		"max_diagnostic_records":       100,
		"max_loadtest_age_days":        1,
		"max_loadtest_records":         100,
		"admin_allow_cidr":             "127.0.0.1/32",
		"admin_require_mfa":            true,
		"cleaner_last_run_at":          "",
		"cleaner_last_result":          "ok",
		"cleaner_last_removed_records": 0,
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/system/settings", bytes.NewReader(body))
	setLoopbackRemoteAddr(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("save expected 400 got %d body=%s", rr.Code, rr.Body.String())
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("GATEWAY_ADMIN_MFA_CODE")) {
		t.Fatalf("expected MFA config hint body=%s", rr.Body.String())
	}
}

func TestNodeTunnelWorkspacePersistsTransportAndRelayMode(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	body := `{"local_node":{"node_ip":"10.10.1.50","signaling_port":5080,"device_id":"34020000002000000050","rtp_port_start":31000,"rtp_port_end":31020},"peer_node":{"node_ip":"10.20.1.50","signaling_port":6080,"device_id":"34020000002000000002","rtp_port_start":32000,"rtp_port_end":32020},"network_mode":"SENDER_SIP__RECEIVER_SIP","sip_capability":{"transport":"UDP"},"rtp_capability":{"transport":"TCP"},"session_settings":{"channel_protocol":"GB/T 28181","connection_initiator":"PEER","mapping_relay_mode":"SIP_ONLY","heartbeat_interval_sec":30,"register_retry_count":2,"register_retry_interval_sec":5},"security_settings":{"signer":"HMAC-SHA256","encryption":"AES","verify_interval_min":30,"admin_allow_cidr":"127.0.0.1/32","admin_require_mfa":false},"encryption_settings":{"algorithm":"AES"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/node-tunnel/workspace", bytes.NewBufferString(body))
	setLoopbackRemoteAddr(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/node-tunnel/workspace expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	for _, needle := range []string{"\"transport\":\"UDP\"", "\"transport\":\"TCP\"", "\"mapping_relay_mode\":\"SIP_ONLY\"", "\"network_mode\":\"SENDER_SIP__RECEIVER_SIP\""} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("POST response missing %s body=%s", needle, rr.Body.String())
		}
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/node-tunnel/workspace", nil)
	setLoopbackRemoteAddr(getReq)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET /api/node-tunnel/workspace expected 200 got %d body=%s", getRR.Code, getRR.Body.String())
	}
	for _, needle := range []string{"\"transport\":\"UDP\"", "\"transport\":\"TCP\"", "\"mapping_relay_mode\":\"SIP_ONLY\"", "\"network_mode\":\"SENDER_SIP__RECEIVER_SIP\"", "\"response_channel\":\"SIP\""} {
		if !strings.Contains(getRR.Body.String(), needle) {
			t.Fatalf("GET response missing %s body=%s", needle, getRR.Body.String())
		}
	}
}

func TestNodeTunnelWorkspaceSwitchesModeAndRebindsEnabledPeerAtomically(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	body := `{"local_node":{"node_ip":"10.10.1.50","signaling_port":5080,"device_id":"34020000002000000050","rtp_port_start":31000,"rtp_port_end":31020},"peer_node":{"node_ip":"10.20.1.50","signaling_port":6080,"device_id":"34020000002000000002","rtp_port_start":32000,"rtp_port_end":32020},"network_mode":"SENDER_SIP__RECEIVER_SIP","sip_capability":{"transport":"UDP"},"rtp_capability":{"transport":"TCP"},"session_settings":{"channel_protocol":"GB/T 28181","connection_initiator":"PEER","mapping_relay_mode":"SIP_ONLY","heartbeat_interval_sec":30,"register_retry_count":2,"register_retry_interval_sec":5},"security_settings":{"signer":"HMAC-SHA256","encryption":"AES","verify_interval_min":30,"admin_allow_cidr":"127.0.0.1/32","admin_require_mfa":false},"encryption_settings":{"algorithm":"AES"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/node-tunnel/workspace", bytes.NewBufferString(body))
	setLoopbackRemoteAddr(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/node-tunnel/workspace expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"network_mode":"SENDER_SIP__RECEIVER_SIP"`) {
		t.Fatalf("expected response to contain switched network_mode body=%s", rr.Body.String())
	}

	nodeReq := httptest.NewRequest(http.MethodGet, "/api/node", nil)
	setLoopbackRemoteAddr(nodeReq)
	nodeRR := httptest.NewRecorder()
	h.ServeHTTP(nodeRR, nodeReq)
	if nodeRR.Code != http.StatusOK {
		t.Fatalf("GET /api/node expected 200 got %d body=%s", nodeRR.Code, nodeRR.Body.String())
	}
	for _, needle := range []string{`"current_network_mode":"SENDER_SIP__RECEIVER_SIP"`, `"node_id":"34020000002000000050"`} {
		if !strings.Contains(nodeRR.Body.String(), needle) {
			t.Fatalf("node response missing %s body=%s", needle, nodeRR.Body.String())
		}
	}

	peerReq := httptest.NewRequest(http.MethodGet, "/api/peers", nil)
	setLoopbackRemoteAddr(peerReq)
	peerRR := httptest.NewRecorder()
	h.ServeHTTP(peerRR, peerReq)
	if peerRR.Code != http.StatusOK {
		t.Fatalf("GET /api/peers expected 200 got %d body=%s", peerRR.Code, peerRR.Body.String())
	}
	for _, needle := range []string{`"peer_node_id":"34020000002000000002"`, `"supported_network_mode":"SENDER_SIP__RECEIVER_SIP"`, `"enabled":true`} {
		if !strings.Contains(peerRR.Body.String(), needle) {
			t.Fatalf("peer response missing %s body=%s", needle, peerRR.Body.String())
		}
	}
}

func TestNodeTunnelWorkspacePersistsRegisterAuthAndCatalogSettings(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	body := `{"local_node":{"node_ip":"10.10.1.60","signaling_port":5090,"device_id":"34020000002000000060","rtp_port_start":31100,"rtp_port_end":31120},"peer_node":{"node_ip":"10.20.1.60","signaling_port":6090,"device_id":"34020000002000000060","rtp_port_start":32100,"rtp_port_end":32120},"network_mode":"SENDER_SIP__RECEIVER_RTP","sip_capability":{"transport":"TCP"},"rtp_capability":{"transport":"UDP"},"session_settings":{"channel_protocol":"GB/T 28181","connection_initiator":"LOCAL","mapping_relay_mode":"AUTO","heartbeat_interval_sec":30,"register_retry_count":2,"register_retry_interval_sec":5,"register_auth_enabled":true,"register_auth_username":"34020000002000000001","register_auth_password":"secret","register_auth_realm":"3402000000","register_auth_algorithm":"MD5","catalog_subscribe_expires_sec":1200},"security_settings":{"signer":"HMAC-SHA256","encryption":"AES","verify_interval_min":30,"admin_allow_cidr":"127.0.0.1/32","admin_require_mfa":false},"encryption_settings":{"algorithm":"AES"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/node-tunnel/workspace", bytes.NewBufferString(body))
	setLoopbackRemoteAddr(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/node-tunnel/workspace expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	for _, needle := range []string{`"register_auth_enabled":true`, `"register_auth_username":"34020000002000000001"`, `"catalog_subscribe_expires_sec":1200`} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("POST response missing %s body=%s", needle, rr.Body.String())
		}
	}

	stateReq := httptest.NewRequest(http.MethodGet, "/api/tunnel/gb28181/state", nil)
	setLoopbackRemoteAddr(stateReq)
	stateRR := httptest.NewRecorder()
	h.ServeHTTP(stateRR, stateReq)
	if stateRR.Code != http.StatusOK {
		t.Fatalf("GET /api/tunnel/gb28181/state expected 200 got %d body=%s", stateRR.Code, stateRR.Body.String())
	}
	for _, needle := range []string{`"register_auth_enabled":true`, `"catalog_subscribe_expires_sec":1200`} {
		if !strings.Contains(stateRR.Body.String(), needle) {
			t.Fatalf("state response missing %s body=%s", needle, stateRR.Body.String())
		}
	}
}

func TestNodeTunnelWorkspacePersistsMappingPortRangeRoundTrip(t *testing.T) {
	h, _, _ := buildTestHandler(t)
	body := `{"local_node":{"node_ip":"10.10.1.70","signaling_port":5070,"device_id":"34020000002000000070","rtp_port_start":31200,"rtp_port_end":31220,"mapping_port_start":18123,"mapping_port_end":18145},"peer_node":{"node_ip":"10.20.1.70","signaling_port":6070,"device_id":"34020000002000000071","rtp_port_start":32200,"rtp_port_end":32220},"network_mode":"SENDER_SIP__RECEIVER_SIP_RTP","sip_capability":{"transport":"UDP"},"rtp_capability":{"transport":"UDP"},"session_settings":{"channel_protocol":"GB/T 28181","connection_initiator":"LOCAL","mapping_relay_mode":"AUTO","heartbeat_interval_sec":30,"register_retry_count":2,"register_retry_interval_sec":5},"security_settings":{"signer":"HMAC-SHA256","encryption":"AES","verify_interval_min":30,"admin_allow_cidr":"127.0.0.1/32","admin_require_mfa":false},"encryption_settings":{"algorithm":"AES"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/node-tunnel/workspace", bytes.NewBufferString(body))
	setLoopbackRemoteAddr(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/node-tunnel/workspace expected 200 got %d body=%s", rr.Code, rr.Body.String())
	}
	for _, needle := range []string{`"mapping_port_start":18123`, `"mapping_port_end":18145`} {
		if !strings.Contains(rr.Body.String(), needle) {
			t.Fatalf("POST response missing %s body=%s", needle, rr.Body.String())
		}
	}
	getReq := httptest.NewRequest(http.MethodGet, "/api/node-tunnel/workspace", nil)
	setLoopbackRemoteAddr(getReq)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET /api/node-tunnel/workspace expected 200 got %d body=%s", getRR.Code, getRR.Body.String())
	}
	for _, needle := range []string{`"mapping_port_start":18123`, `"mapping_port_end":18145`} {
		if !strings.Contains(getRR.Body.String(), needle) {
			t.Fatalf("GET response missing %s body=%s", needle, getRR.Body.String())
		}
	}
}
