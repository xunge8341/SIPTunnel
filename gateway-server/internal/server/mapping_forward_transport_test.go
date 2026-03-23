package server

import (
	"os"
	"testing"
	"time"
)

func TestCachedMappingTransportReusesTransportByTimeoutProfile(t *testing.T) {
	left := cachedMappingTransport(2*time.Second, 5*time.Second)
	right := cachedMappingTransport(2*time.Second, 5*time.Second)
	if left != right {
		t.Fatal("expected cached transport reuse")
	}
	other := cachedMappingTransport(3*time.Second, 5*time.Second)
	if other == left {
		t.Fatal("expected distinct transport for distinct timeout profile")
	}
}
func TestCachedMappingHTTPClientReusesClientByTimeoutProfile(t *testing.T) {
	left := cachedMappingHTTPClient(2*time.Second, 5*time.Second)
	right := cachedMappingHTTPClient(2*time.Second, 5*time.Second)
	if left != right {
		t.Fatal("expected cached client reuse")
	}
	other := cachedMappingHTTPClient(3*time.Second, 5*time.Second)
	if other == left {
		t.Fatal("expected distinct client for distinct timeout profile")
	}
}
func TestCachedMappingTransportSeparatesKeepAlivePolicy(t *testing.T) {
	key := runtimeHTTPKeepAliveScopeEnv(mappingForwardClientScope)
	old := os.Getenv(key)
	defer os.Setenv(key, old)
	if err := os.Setenv(key, "true"); err != nil {
		t.Fatal(err)
	}
	disabled := cachedMappingTransport(2*time.Second, 5*time.Second)
	if !disabled.DisableKeepAlives {
		t.Fatal("expected disabled keep-alives transport")
	}
	if err := os.Setenv(key, "false"); err != nil {
		t.Fatal(err)
	}
	enabled := cachedMappingTransport(2*time.Second, 5*time.Second)
	if enabled.DisableKeepAlives || enabled == disabled {
		t.Fatal("expected distinct enabled transport")
	}
}
