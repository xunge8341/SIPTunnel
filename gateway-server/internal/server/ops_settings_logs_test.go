package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContractEndpoints(t *testing.T) {
	h, _, _ := buildTestHandler(t)

	for _, path := range []string{"/api/access-logs?page=1&page_size=10&slow_only=true", "/api/system/settings", "/api/dashboard/ops-summary", "/api/dashboard/summary", "/api/protection/state", "/api/security/state", "/api/node-tunnel/workspace"} {
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
	saveReq := httptest.NewRequest(http.MethodPost, "/api/system/settings", bytes.NewReader(body))
	saveRR := httptest.NewRecorder()
	h.ServeHTTP(saveRR, saveReq)
	if saveRR.Code != http.StatusOK {
		t.Fatalf("save expected 200 got %d", saveRR.Code)
	}
	getReq := httptest.NewRequest(http.MethodGet, "/api/system/settings", nil)
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK || !bytes.Contains(getRR.Body.Bytes(), []byte("./tmp.db")) {
		t.Fatalf("reload failed body=%s", getRR.Body.String())
	}
}
