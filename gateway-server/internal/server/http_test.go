package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	NewHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Fatalf("unexpected body: %s", got)
	}
}

func TestDemoProcessAndAuditQuery(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest(http.MethodPost, "/demo/process", strings.NewReader("{}"))
	req.Header.Set("X-Api-Code", "asset.sync")
	req.Header.Set("X-Source-System", "b-system")
	req.Header.Set("X-Initiator", "ops-user")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	queryReq := httptest.NewRequest(http.MethodGet, "/audit/events?who=ops-user", nil)
	queryRR := httptest.NewRecorder()
	h.ServeHTTP(queryRR, queryReq)
	if queryRR.Code != http.StatusOK {
		t.Fatalf("expected 200 from audit query, got %d", queryRR.Code)
	}

	var payload struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(queryRR.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if len(payload.Events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(payload.Events))
	}
}

func TestDemoProcessValidationFail(t *testing.T) {
	h := NewHandler()
	req := httptest.NewRequest(http.MethodPost, "/demo/process", strings.NewReader("{}"))
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
