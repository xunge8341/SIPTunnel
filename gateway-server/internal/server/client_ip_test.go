package server

import (
	"net/http/httptest"
	"testing"
)

func TestRequestClientIPIgnoresForwardedHeadersFromUntrustedPeers(t *testing.T) {
	t.Setenv("GATEWAY_TRUSTED_PROXY_CIDRS", "")
	req := httptest.NewRequest("GET", "/api/system/settings", nil)
	req.RemoteAddr = "198.51.100.10:5060"
	req.Header.Set("X-Forwarded-For", "203.0.113.77")
	if got := requestClientIP(req); got != "198.51.100.10" {
		t.Fatalf("expected remote addr, got %s", got)
	}
}

func TestRequestClientIPAcceptsForwardedChainFromTrustedProxy(t *testing.T) {
	t.Setenv("GATEWAY_TRUSTED_PROXY_CIDRS", "10.0.0.0/8,127.0.0.0/8")
	req := httptest.NewRequest("GET", "/api/system/settings", nil)
	req.RemoteAddr = "10.10.0.5:5060"
	req.Header.Set("X-Forwarded-For", "203.0.113.77, 10.1.1.8")
	if got := requestClientIP(req); got != "203.0.113.77" {
		t.Fatalf("expected forwarded client ip, got %s", got)
	}
}
