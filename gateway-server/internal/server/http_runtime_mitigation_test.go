package server

import (
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestRuntimeHTTPKeepAliveScopeEnv(t *testing.T) {
	if got := runtimeHTTPKeepAliveScopeEnv("gateway-http"); got != "GATEWAY_DISABLE_HTTP_KEEPALIVES_GATEWAY_HTTP" {
		t.Fatalf("unexpected env name: %s", got)
	}
}
func TestShouldDisableHTTPKeepAlivesForRuntimeEnvOverride(t *testing.T) {
	key := runtimeHTTPKeepAliveScopeEnv("gateway-http")
	old := os.Getenv(key)
	defer os.Setenv(key, old)
	if err := os.Setenv(key, "false"); err != nil {
		t.Fatal(err)
	}
	if shouldDisableHTTPKeepAlivesForRuntime("gateway-http") {
		t.Fatal("expected explicit false env to disable mitigation")
	}
}
func TestApplyRuntimeHTTPMitigationsExplicitFalse(t *testing.T) {
	key := runtimeHTTPKeepAliveScopeEnv("gateway-http")
	old := os.Getenv(key)
	defer os.Setenv(key, old)
	if err := os.Setenv(key, "false"); err != nil {
		t.Fatal(err)
	}
	ApplyRuntimeHTTPMitigations("gateway-http", &http.Server{})
}
func TestApplyRuntimeHTTPTransportMitigationsExplicitTrue(t *testing.T) {
	key := runtimeHTTPKeepAliveScopeEnv(mappingForwardClientScope)
	old := os.Getenv(key)
	defer os.Setenv(key, old)
	if err := os.Setenv(key, "true"); err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{MaxIdleConns: 16, MaxIdleConnsPerHost: 8}
	decision := ApplyRuntimeHTTPTransportMitigations(mappingForwardClientScope, transport)
	if !decision.Disable || !transport.DisableKeepAlives || transport.MaxIdleConns != 0 || transport.MaxIdleConnsPerHost != 0 {
		t.Fatalf("unexpected transport mitigation: %+v / %+v", decision, transport)
	}
}

func TestRuntimeHTTPKeepAlivePolicy_AutoExemptsMappingForwardClientOnWindowsGo126(t *testing.T) {
	key := runtimeHTTPKeepAliveScopeEnv(mappingForwardClientScope)
	old := os.Getenv(key)
	defer os.Setenv(key, old)
	_ = os.Unsetenv(key)
	oldGlobal := os.Getenv("GATEWAY_DISABLE_HTTP_KEEPALIVES")
	defer os.Setenv("GATEWAY_DISABLE_HTTP_KEEPALIVES", oldGlobal)
	_ = os.Unsetenv("GATEWAY_DISABLE_HTTP_KEEPALIVES")

	decision := runtimeHTTPKeepAlivePolicy(mappingForwardClientScope)
	if runtime.GOOS == "windows" && strings.HasPrefix(runtime.Version(), "go1.26") {
		if decision.Disable {
			t.Fatalf("expected auto exemption to preserve keep-alives, got %+v", decision)
		}
		if decision.Source != "auto_exempt" {
			t.Fatalf("expected auto_exempt source, got %+v", decision)
		}
	}
}

func TestRuntimeHTTPKeepAlivePolicy_AutoExemptsMappingRuntimeOnWindowsGo126(t *testing.T) {
	key := runtimeHTTPKeepAliveScopeEnv(mappingRuntimeScope)
	old := os.Getenv(key)
	defer os.Setenv(key, old)
	_ = os.Unsetenv(key)
	oldGlobal := os.Getenv("GATEWAY_DISABLE_HTTP_KEEPALIVES")
	defer os.Setenv("GATEWAY_DISABLE_HTTP_KEEPALIVES", oldGlobal)
	_ = os.Unsetenv("GATEWAY_DISABLE_HTTP_KEEPALIVES")

	decision := runtimeHTTPKeepAlivePolicy(mappingRuntimeScope)
	if runtime.GOOS == "windows" && strings.HasPrefix(runtime.Version(), "go1.26") {
		if decision.Disable {
			t.Fatalf("expected auto exemption to preserve keep-alives, got %+v", decision)
		}
		if decision.Source != "auto_exempt" {
			t.Fatalf("expected auto_exempt source, got %+v", decision)
		}
	}
}
