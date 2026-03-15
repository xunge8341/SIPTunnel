package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccessLogsAndSystemSettingsEndpoints(t *testing.T) {
	h, _, _ := buildTestHandler(t)

	accessReq := httptest.NewRequest(http.MethodGet, "/api/access-logs?page=1&page_size=10", nil)
	accessRR := httptest.NewRecorder()
	h.ServeHTTP(accessRR, accessReq)
	if accessRR.Code != http.StatusOK {
		t.Fatalf("GET /api/access-logs expected 200 got %d body=%s", accessRR.Code, accessRR.Body.String())
	}

	settingsReq := httptest.NewRequest(http.MethodGet, "/api/system/settings", nil)
	settingsRR := httptest.NewRecorder()
	h.ServeHTTP(settingsRR, settingsReq)
	if settingsRR.Code != http.StatusOK {
		t.Fatalf("GET /api/system/settings expected 200 got %d body=%s", settingsRR.Code, settingsRR.Body.String())
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "/api/dashboard/ops-summary", nil)
	summaryRR := httptest.NewRecorder()
	h.ServeHTTP(summaryRR, summaryReq)
	if summaryRR.Code != http.StatusOK {
		t.Fatalf("GET /api/dashboard/ops-summary expected 200 got %d body=%s", summaryRR.Code, summaryRR.Body.String())
	}
}
