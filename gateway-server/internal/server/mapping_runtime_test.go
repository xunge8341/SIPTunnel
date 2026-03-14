package server

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMappingRuntimeManager_ForwardsHTTPRequestsE2E(t *testing.T) {
	var seenMethod string
	var seenPath string
	var seenQuery string
	var seenHeader string
	var seenBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(r.Body)
		seenMethod = r.Method
		seenPath = r.URL.Path
		seenQuery = r.URL.RawQuery
		seenHeader = r.Header.Get("X-Test")
		seenBody = string(body)
		w.Header().Set("X-Upstream", "ok")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("upstream-response"))
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}
	host, portStr, err := net.SplitHostPort(target.Host)
	if err != nil {
		t.Fatalf("split upstream host: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	manager := newMappingRuntimeManager(nil)
	defer func() { _ = manager.Close() }()

	localPort := findFreePort(t)
	manager.SyncMappings([]TunnelMapping{{
		MappingID:            "map-e2e",
		Enabled:              true,
		LocalBindIP:          "127.0.0.1",
		LocalBindPort:        localPort,
		LocalBasePath:        "/proxy",
		RemoteTargetIP:       host,
		RemoteTargetPort:     port,
		RemoteBasePath:       "/api",
		AllowedMethods:       []string{"POST"},
		ConnectTimeoutMS:     500,
		RequestTimeoutMS:     1500,
		ResponseTimeoutMS:    1500,
		MaxRequestBodyBytes:  2048,
		MaxResponseBodyBytes: 2048,
	}})

	waitMappingState(t, manager, "map-e2e", mappingStateListening)

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/proxy/orders?id=42", localPort), strings.NewReader("demo-body"))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-Test", "demo")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request local mapping: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("X-Upstream"); got != "ok" {
		t.Fatalf("expected upstream header, got %q", got)
	}
	if string(body) != "upstream-response" {
		t.Fatalf("expected upstream body, got %q", string(body))
	}
	if seenMethod != http.MethodPost || seenPath != "/api/orders" || seenQuery != "id=42" || seenHeader != "demo" || seenBody != "demo-body" {
		t.Fatalf("unexpected forwarded request method=%s path=%s query=%s header=%s body=%s", seenMethod, seenPath, seenQuery, seenHeader, seenBody)
	}

	waitMappingState(t, manager, "map-e2e", mappingStateListening)
	snapshot := manager.Snapshot()["map-e2e"]
	if !strings.Contains(snapshot.Reason, "最近转发成功") {
		t.Fatalf("expected success reason, got %q", snapshot.Reason)
	}
}

func TestMappingRuntimeManager_UpdatesDegradedWhenPrepareFails(t *testing.T) {
	manager := newMappingRuntimeManager(nil)
	defer func() { _ = manager.Close() }()

	localPort := findFreePort(t)
	manager.SyncMappings([]TunnelMapping{{
		MappingID:            "map-prepare-fail",
		Enabled:              true,
		LocalBindIP:          "127.0.0.1",
		LocalBindPort:        localPort,
		LocalBasePath:        "/proxy",
		RemoteTargetIP:       "127.0.0.1",
		RemoteTargetPort:     1,
		RemoteBasePath:       "/api",
		AllowedMethods:       []string{"POST"},
		ConnectTimeoutMS:     100,
		RequestTimeoutMS:     200,
		ResponseTimeoutMS:    200,
		MaxRequestBodyBytes:  512,
		MaxResponseBodyBytes: 512,
	}})
	waitMappingState(t, manager, "map-prepare-fail", mappingStateListening)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/proxy/ping", localPort))
	if err != nil {
		t.Fatalf("request local mapping: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}

	waitMappingState(t, manager, "map-prepare-fail", mappingStateDegraded)
	snapshot := manager.Snapshot()["map-prepare-fail"]
	if !strings.Contains(snapshot.Reason, "转发准备失败") {
		t.Fatalf("expected prepare failure reason, got %q", snapshot.Reason)
	}
}

func TestMappingRuntimeManager_UpdatesDegradedWhenForwardFails(t *testing.T) {
	manager := newMappingRuntimeManager(nil)
	defer func() { _ = manager.Close() }()

	localPort := findFreePort(t)
	manager.SyncMappings([]TunnelMapping{{
		MappingID:            "map-degraded",
		Enabled:              true,
		LocalBindIP:          "127.0.0.1",
		LocalBindPort:        localPort,
		LocalBasePath:        "/proxy",
		RemoteTargetIP:       "127.0.0.1",
		RemoteTargetPort:     1,
		RemoteBasePath:       "/api",
		AllowedMethods:       []string{"GET"},
		ConnectTimeoutMS:     100,
		RequestTimeoutMS:     200,
		ResponseTimeoutMS:    200,
		MaxRequestBodyBytes:  512,
		MaxResponseBodyBytes: 512,
	}})
	waitMappingState(t, manager, "map-degraded", mappingStateListening)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/proxy/ping", localPort))
	if err != nil {
		t.Fatalf("request local mapping: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", resp.StatusCode)
	}

	waitMappingState(t, manager, "map-degraded", mappingStateDegraded)
}

func waitMappingState(t *testing.T, manager *mappingRuntimeManager, mappingID, expected string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := manager.Snapshot()[mappingID].State; got == expected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("mapping %s state did not become %s, got %+v", mappingID, expected, manager.Snapshot()[mappingID])
}

func findFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free port: %v", err)
	}
	defer ln.Close()
	_, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi port: %v", err)
	}
	return port
}
