package diagnostics

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadPprofConfigFromEnvDefaults(t *testing.T) {
	t.Setenv(envPprofEnabled, "")
	t.Setenv(envPprofListenAddress, "")
	t.Setenv(envPprofToken, "")
	t.Setenv(envPprofAllowedCIDRs, "")

	cfg := LoadPprofConfigFromEnv()
	if cfg.Enabled {
		t.Fatalf("enabled=%v, want false", cfg.Enabled)
	}
	if cfg.ListenAddress != "127.0.0.1:6060" {
		t.Fatalf("listen=%q", cfg.ListenAddress)
	}
	if got := len(cfg.AllowedCIDRs); got != 2 {
		t.Fatalf("allowed cidrs len=%d", got)
	}
}

func TestPprofConfigValidate(t *testing.T) {
	cfg := PprofConfig{Enabled: true, ListenAddress: "127.0.0.1:6060", AllowedCIDRs: []string{"127.0.0.1/32"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("want error for empty token")
	}

	cfg.AuthToken = "token"
	cfg.AllowedCIDRs = []string{"bad-cidr"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("want error for invalid cidr")
	}

	cfg.AllowedCIDRs = []string{"127.0.0.1/32"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate error: %v", err)
	}
}

func TestPprofGuard(t *testing.T) {
	cfg := PprofConfig{Enabled: true, AuthToken: "secret", AllowedCIDRs: []string{"127.0.0.1/32"}}
	guard, err := newPprofGuard(cfg)
	if err != nil {
		t.Fatalf("newPprofGuard error: %v", err)
	}

	h := guard(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	okReq := httptest.NewRequest(http.MethodGet, "http://example.com/debug/pprof/heap", nil)
	okReq.RemoteAddr = "127.0.0.1:12345"
	okReq.Header.Set("Authorization", "Bearer secret")
	okResp := httptest.NewRecorder()
	h.ServeHTTP(okResp, okReq)
	if okResp.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", okResp.Code)
	}

	badTokenReq := httptest.NewRequest(http.MethodGet, "http://example.com/debug/pprof/heap", nil)
	badTokenReq.RemoteAddr = "127.0.0.1:12345"
	badTokenReq.Header.Set("Authorization", "Bearer wrong")
	badTokenResp := httptest.NewRecorder()
	h.ServeHTTP(badTokenResp, badTokenReq)
	if badTokenResp.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", badTokenResp.Code)
	}

	badIPReq := httptest.NewRequest(http.MethodGet, "http://example.com/debug/pprof/heap", nil)
	badIPReq.RemoteAddr = "10.0.0.1:12345"
	badIPReq.Header.Set("Authorization", "Bearer secret")
	badIPResp := httptest.NewRecorder()
	h.ServeHTTP(badIPResp, badIPReq)
	if badIPResp.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", badIPResp.Code)
	}
}
