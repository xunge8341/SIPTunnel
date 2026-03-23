package server

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"siptunnel/internal/config"
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

func TestMappingRuntimeManager_SuppressesBrowserAncillaryRequests(t *testing.T) {
	upstreamHit := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		w.WriteHeader(http.StatusOK)
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
		MappingID:            "map-browser-probe",
		Enabled:              true,
		LocalBindIP:          "127.0.0.1",
		LocalBindPort:        localPort,
		LocalBasePath:        "/",
		RemoteTargetIP:       host,
		RemoteTargetPort:     port,
		RemoteBasePath:       "/",
		AllowedMethods:       []string{"GET"},
		ConnectTimeoutMS:     500,
		RequestTimeoutMS:     1500,
		ResponseTimeoutMS:    1500,
		MaxRequestBodyBytes:  2048,
		MaxResponseBodyBytes: 2048,
	}})
	waitMappingState(t, manager, "map-browser-probe", mappingStateListening)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/favicon.ico", localPort), nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,*/*")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request local mapping: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	if upstreamHit {
		t.Fatal("expected browser ancillary request to be suppressed locally")
	}
	waitMappingState(t, manager, "map-browser-probe", mappingStateListening)
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

func TestBuildTargetURL_AutocorrectsRootPathToConfiguredBase(t *testing.T) {
	mapping := TunnelMapping{
		MappingID:        "map-root-fallback",
		LocalBasePath:    "/web",
		RemoteTargetIP:   "10.0.0.9",
		RemoteTargetPort: 8080,
		RemoteBasePath:   "/api/web",
	}
	target, err := buildTargetURL(mapping, &url.URL{Path: "/", RawQuery: "a=1"})
	if err != nil {
		t.Fatalf("buildTargetURL error: %v", err)
	}
	if target.Path != "/api/web" {
		t.Fatalf("target.Path=%q, want /api/web", target.Path)
	}
	if target.RawQuery != "a=1" {
		t.Fatalf("target.RawQuery=%q, want a=1", target.RawQuery)
	}
}

func TestBuildTargetURL_AutocorrectsIndexHTMLToConfiguredBase(t *testing.T) {
	mapping := TunnelMapping{
		MappingID:        "map-index-fallback",
		LocalBasePath:    "/player",
		RemoteTargetIP:   "10.0.0.9",
		RemoteTargetPort: 8080,
		RemoteBasePath:   "/video/index",
	}
	target, err := buildTargetURL(mapping, &url.URL{Path: "/index.html"})
	if err != nil {
		t.Fatalf("buildTargetURL error: %v", err)
	}
	if target.Path != "/video/index" {
		t.Fatalf("target.Path=%q, want /video/index", target.Path)
	}
}

func TestBuildTargetURL_AutocorrectsBaseIndexDocumentToConfiguredBase(t *testing.T) {
	mapping := TunnelMapping{
		MappingID:        "map-base-index-fallback",
		LocalBasePath:    "/player",
		RemoteTargetIP:   "10.0.0.9",
		RemoteTargetPort: 8080,
		RemoteBasePath:   "/video/index",
	}
	target, err := buildTargetURL(mapping, &url.URL{Path: "/player/index.html"})
	if err != nil {
		t.Fatalf("buildTargetURL error: %v", err)
	}
	if target.Path != "/video/index" {
		t.Fatalf("target.Path=%q, want /video/index", target.Path)
	}
}

func TestBuildTargetURL_AutocorrectsBaseDefaultDocumentToConfiguredBase(t *testing.T) {
	mapping := TunnelMapping{
		MappingID:        "map-base-default-fallback",
		LocalBasePath:    "/archive",
		RemoteTargetIP:   "10.0.0.9",
		RemoteTargetPort: 8080,
		RemoteBasePath:   "/records",
	}
	target, err := buildTargetURL(mapping, &url.URL{Path: "/archive/default.html"})
	if err != nil {
		t.Fatalf("buildTargetURL error: %v", err)
	}
	if target.Path != "/records" {
		t.Fatalf("target.Path=%q, want /records", target.Path)
	}
}

func TestParseContentRangeHeader(t *testing.T) {
	spec, ok := parseContentRangeHeader("bytes 6815744-10485759/104857600")
	if !ok {
		t.Fatalf("expected content-range to parse")
	}
	if spec.start != 6815744 || spec.end != 10485759 || !spec.hasTotal || spec.total != 104857600 {
		t.Fatalf("unexpected content-range: %+v", spec)
	}
}

func TestBuildPreparedResumeRequest(t *testing.T) {
	prepared := &mappingForwardRequest{
		Method:               http.MethodGet,
		TargetURL:            &url.URL{Scheme: "http", Host: "example.com", Path: "/video.mp4"},
		Headers:              http.Header{},
		Mapping:              TunnelMapping{MappingID: "video", ResponseMode: "RTP"},
		MaxResponseBodyBytes: 64 << 20,
	}
	resp := &http.Response{StatusCode: http.StatusPartialContent, Header: http.Header{"Content-Range": []string{"bytes 6815744-10485759/104857600"}, "Etag": []string{"\"v1\""}}}
	clone, nextStart, rangeHeader, err := buildPreparedResumeRequest(prepared, resp, 4096)
	if err != nil {
		t.Fatalf("build resume request: %v", err)
	}
	if nextStart != 6819840 {
		t.Fatalf("unexpected next start: %d", nextStart)
	}
	if rangeHeader != "bytes=6819840-" {
		t.Fatalf("unexpected range header: %s", rangeHeader)
	}
	if got := clone.Headers.Get("Range"); got != rangeHeader {
		t.Fatalf("expected range header %q, got %q", rangeHeader, got)
	}
	if got := clone.Headers.Get("If-Range"); got != "\"v1\"" {
		t.Fatalf("expected If-Range to be preserved, got %q", got)
	}
	if clone.MaxResponseBodyBytes < mappingRTPResumeHardLimitBytes {
		t.Fatalf("expected max response body bytes override, got %d", clone.MaxResponseBodyBytes)
	}
}

func TestBuildPreparedResumeRequestWithLimit_UsesClosedRangeWhenBounded(t *testing.T) {
	prepared := &mappingForwardRequest{Headers: make(http.Header), Method: http.MethodGet}
	resp := &http.Response{StatusCode: http.StatusPartialContent, Header: make(http.Header), ContentLength: 1024}
	resp.Header.Set("Content-Range", "bytes 8388608-12582911/395753466")
	resumeEnd := int64(12582911)
	got, nextStart, rangeHeader, err := buildPreparedResumeRequestWithLimit(prepared, resp, 1540096, &resumeEnd)
	if err != nil {
		t.Fatalf("buildPreparedResumeRequestWithLimit error: %v", err)
	}
	if nextStart != 9928704 {
		t.Fatalf("nextStart=%d, want 9928704", nextStart)
	}
	if rangeHeader != "bytes=9928704-12582911" {
		t.Fatalf("rangeHeader=%q, want bounded range", rangeHeader)
	}
	if got.Headers.Get("Range") != rangeHeader {
		t.Fatalf("prepared range=%q, want %q", got.Headers.Get("Range"), rangeHeader)
	}
}

func TestSegmentedDownloadProfileForResponse_UsesBoundaryHTTPInSecureBoundaryMode(t *testing.T) {
	ApplyTransportTuning(config.DefaultTransportTuningConfig())
	resp := &http.Response{Header: make(http.Header)}
	profile := segmentedDownloadProfileForResponse(&mappingForwardRequest{}, httptest.NewRequest(http.MethodGet, "http://example.com/file", nil), resp)
	if profile.name != "boundary-http" {
		t.Fatalf("profile=%q, want boundary-http", profile.name)
	}
}

func TestClassifyRecoverableRTPReadError_PendingGapTimeout(t *testing.T) {
	if got := classifyRecoverableRTPReadError(errors.New("rtp pending gap timeout expected=100 pending=3 gap=2 wait_ms=1500")); got != "rtp_gap_timeout" {
		t.Fatalf("classifyRecoverableRTPReadError=%q, want %q", got, "rtp_gap_timeout")
	}
}

func TestBuildPreparedResumeRequestWithLimitRejectsBeyondWindow(t *testing.T) {
	prepared := &mappingForwardRequest{Method: http.MethodGet, Headers: make(http.Header)}
	resp := &http.Response{Header: make(http.Header)}
	resp.Header.Set("Content-Range", "bytes 0-99/100")
	resumeEnd := int64(50)

	_, _, _, err := buildPreparedResumeRequestWithLimit(prepared, resp, 60, &resumeEnd)
	if err == nil {
		t.Fatal("expected out-of-window resume rejection")
	}
	if got := classifyWindowRecoveryFailure(err); got != windowRecoveryFailureOutOfWindow {
		t.Fatalf("expected out_of_window, got %s", got)
	}
}

func TestValidatePreparedResumeResponseWithLimitRejectsOverflow(t *testing.T) {
	baseResp := &http.Response{Header: make(http.Header)}
	baseResp.Header.Set("Content-Type", "application/octet-stream")
	resumeResp := &http.Response{StatusCode: http.StatusPartialContent, Header: make(http.Header)}
	resumeResp.Header.Set("Content-Type", "application/octet-stream")
	resumeResp.Header.Set("Content-Range", "bytes 60-90/100")
	resumeEnd := int64(80)

	err := validatePreparedResumeResponseWithLimit(baseResp, resumeResp, 60, &resumeEnd)
	if err == nil {
		t.Fatal("expected overflow validation error")
	}
	if got := classifyWindowRecoveryFailure(err); got != windowRecoveryFailureOutOfWindow {
		t.Fatalf("expected out_of_window, got %s", got)
	}
}

func TestFixedWindowResumeAttemptLimitCapsWindowLocalRetries(t *testing.T) {
	ApplyTransportTuning(config.TransportTuningConfig{
		BoundaryResumePerRangeRetries: 7,
		BoundaryResumeMaxAttempts:     64,
	})
	defer ApplyTransportTuning(config.DefaultTransportTuningConfig())

	plan := fixedWindowPlan{segmentRetries: 8}
	if got := fixedWindowResumeAttemptLimit(plan); got != 3 {
		t.Fatalf("expected window-local resume limit 3, got %d", got)
	}
}

func TestWindowRecoveryStrategyForClass(t *testing.T) {
	if got := windowRecoveryStrategyForClass(windowRecoveryFailureOutOfWindow, false); got != windowRecoveryStrategyRestartWindow {
		t.Fatalf("expected restart for out_of_window, got %s", got)
	}
	if got := windowRecoveryStrategyForClass(windowRecoveryFailureTimeout, false); got != windowRecoveryStrategyResumeWithinWindow {
		t.Fatalf("expected resume for timeout, got %s", got)
	}
	if got := windowRecoveryStrategyForClass(windowRecoveryFailureTimeout, true); got != windowRecoveryStrategyRestartWindow {
		t.Fatalf("expected threshold switch to restart, got %s", got)
	}
}

func TestMappingRuntimeManager_PreservesDownloadTransferHeader(t *testing.T) {
	var seenTransferID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenTransferID = r.Header.Get(downloadTransferIDHeader)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
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
		MappingID:            "map-transfer-header",
		Enabled:              true,
		LocalBindIP:          "127.0.0.1",
		LocalBindPort:        localPort,
		LocalBasePath:        "/proxy",
		RemoteTargetIP:       host,
		RemoteTargetPort:     port,
		RemoteBasePath:       "/api",
		AllowedMethods:       []string{"GET"},
		ConnectTimeoutMS:     500,
		RequestTimeoutMS:     1500,
		ResponseTimeoutMS:    1500,
		MaxRequestBodyBytes:  2048,
		MaxResponseBodyBytes: 2048,
	}})

	waitMappingState(t, manager, "map-transfer-header", mappingStateListening)

	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/proxy/orders", localPort), nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(downloadTransferIDHeader, "outer-download-req-123")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request local mapping: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	if seenTransferID != "outer-download-req-123" {
		t.Fatalf("expected upstream to receive %s, got %q", downloadTransferIDHeader, seenTransferID)
	}
}
