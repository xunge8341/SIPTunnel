package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestShouldLogInboundRequestSkipsHealthProbe(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/healthz", bytes.NewBufferString(`{"payload":"x"}`))
	if shouldLogInboundRequest(req) {
		t.Fatalf("expected health probe request to skip inbound request log")
	}
}

func TestHealthzDrainsBodyAndWritesFastJSON(t *testing.T) {
	deps := &handlerDeps{}
	req := httptest.NewRequest(http.MethodPost, "/healthz", bytes.NewBuffer(make([]byte, 4096)))
	rr := httptest.NewRecorder()
	deps.healthz(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected Cache-Control=no-store, got %q", got)
	}
	if body := rr.Body.String(); body != string(healthzResponseBytes) {
		t.Fatalf("unexpected body: %q", body)
	}
	if req.Body != http.NoBody {
		t.Fatalf("expected request body replaced with http.NoBody")
	}
}
