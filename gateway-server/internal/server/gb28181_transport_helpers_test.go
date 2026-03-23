package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCompactTunnelRequestHeaders_UDPDropsAcceptButKeepsContentType(t *testing.T) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json,text/plain,*/*")
	headers.Set("Authorization", "Bearer token")
	got := compactTunnelRequestHeaders(headers, "UDP")
	if got.Get("Content-Type") != "application/json" {
		t.Fatalf("expected Content-Type to be preserved, got %q", got.Get("Content-Type"))
	}
	if got.Get("Authorization") != "Bearer token" {
		t.Fatalf("expected Authorization to be preserved, got %q", got.Get("Authorization"))
	}
	if got.Get("Accept") != "" {
		t.Fatalf("expected Accept to be dropped for UDP, got %q", got.Get("Accept"))
	}
}

func TestCompactTunnelRequestHeaders_UDPPreservesDownloadTransferID(t *testing.T) {
	headers := http.Header{}
	headers.Set(downloadTransferIDHeader, "outer-download-req-123")
	got := compactTunnelRequestHeaders(headers, "UDP")
	if got.Get(downloadTransferIDHeader) != "outer-download-req-123" {
		t.Fatalf("expected %s to be preserved for UDP, got %q", downloadTransferIDHeader, got.Get(downloadTransferIDHeader))
	}
}

func TestCompactTunnelRequestHeaders_TCPKeepsAccept(t *testing.T) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json,text/plain,*/*")
	got := compactTunnelRequestHeaders(headers, "TCP")
	if got.Get("Accept") == "" {
		t.Fatalf("expected Accept to be preserved for TCP")
	}
}

func TestCompactTunnelRequestHeadersForUDPBudget_TrimsLoginCookiesAndValidators(t *testing.T) {
	cookie := strings.Join([]string{
		"analytics_id=abcdefghijklmnopqrstuvwxyz0123456789",
		"JSESSIONID=session-1234567890",
		"theme=dark",
		"XSRF-TOKEN=csrf-abcdef",
		"tracking_id=abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789",
	}, "; ")
	selected := http.Header{}
	selected.Set("Content-Type", "application/x-www-form-urlencoded")
	selected.Set("Cookie", cookie)
	selected.Set("If-None-Match", "etag-v1")
	selected.Set("Cache-Control", "max-age=0")
	selected.Set("Range", "bytes=0-")
	selected.Set("If-Range", "etag-v1")
	prepared := &mappingForwardRequest{
		Method:    http.MethodPost,
		TargetURL: &url.URL{Path: "/api/gmvcs/uap/cas/login"},
		Body:      []byte("username=a&password=b"),
	}
	got := compactTunnelRequestHeadersForUDPBudget(prepared, selected)
	if got.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("expected Content-Type to remain, got %q", got.Get("Content-Type"))
	}
	if got.Get("If-None-Match") != "" || got.Get("Cache-Control") != "" {
		t.Fatalf("expected validators to be dropped, got If-None-Match=%q Cache-Control=%q", got.Get("If-None-Match"), got.Get("Cache-Control"))
	}
	if got.Get("Range") != "" || got.Get("If-Range") != "" {
		t.Fatalf("expected range headers to be dropped for login rescue, got Range=%q If-Range=%q", got.Get("Range"), got.Get("If-Range"))
	}
	compactCookie := got.Get("Cookie")
	if compactCookie == "" {
		t.Fatalf("expected Cookie to remain after compaction")
	}
	if !strings.Contains(compactCookie, "JSESSIONID=") {
		t.Fatalf("expected session cookie to be preserved, got %q", compactCookie)
	}
	if !strings.Contains(compactCookie, "XSRF-TOKEN=") {
		t.Fatalf("expected csrf cookie to be preserved, got %q", compactCookie)
	}
	if strings.Contains(compactCookie, "analytics_id=") || strings.Contains(compactCookie, "tracking_id=") {
		t.Fatalf("expected nonessential cookies to be removed, got %q", compactCookie)
	}
	if len(compactCookie) > 96 {
		t.Fatalf("expected aggressively compacted cookie length <= 96, got %d (%q)", len(compactCookie), compactCookie)
	}
}

func TestCompactTunnelRequestHeadersForUDPBudget_DropsAuthorizationForCookieBackedLoginRescue(t *testing.T) {
	selected := http.Header{}
	selected.Set("Authorization", "Bearer should-drop")
	selected.Set("Cookie", "JSESSIONID=session-1234567890; XSRF-TOKEN=csrf-abcdef")
	prepared := &mappingForwardRequest{
		Method:    http.MethodPost,
		TargetURL: &url.URL{Path: "/api/gmvcs/uap/cas/login"},
		Body:      []byte("username=a&password=b"),
	}
	got := compactTunnelRequestHeadersForUDPBudget(prepared, selected)
	if got.Get("Authorization") != "" {
		t.Fatalf("expected Authorization to be dropped during login budget rescue, got %q", got.Get("Authorization"))
	}
	if got.Get("Cookie") == "" {
		t.Fatal("expected Cookie to remain for session-backed login rescue")
	}
}

func TestCompactTunnelRequestHeadersForUDPBudget_PreservesDownloadTransferID(t *testing.T) {
	selected := http.Header{}
	selected.Set(downloadTransferIDHeader, "outer-download-req-456")
	selected.Set("Range", "bytes=0-")
	prepared := &mappingForwardRequest{
		Method:    http.MethodGet,
		TargetURL: &url.URL{Path: "/video/recording.mp4"},
	}
	got := compactTunnelRequestHeadersForUDPBudget(prepared, selected)
	if got.Get(downloadTransferIDHeader) != "outer-download-req-456" {
		t.Fatalf("expected %s to survive UDP budget compaction, got %q", downloadTransferIDHeader, got.Get(downloadTransferIDHeader))
	}
}

func TestCompactTunnelRequestHeadersForUDPBudget_PreservesRangeGET(t *testing.T) {
	selected := http.Header{}
	selected.Set("Range", "bytes=1048576-")
	selected.Set("If-Range", "etag-v2")
	prepared := &mappingForwardRequest{
		Method:    http.MethodGet,
		TargetURL: &url.URL{Path: "/video/recording.mp4"},
	}
	got := compactTunnelRequestHeadersForUDPBudget(prepared, selected)
	if got.Get("Range") != "bytes=1048576-" {
		t.Fatalf("expected Range to be preserved, got %q", got.Get("Range"))
	}
	if got.Get("If-Range") != "etag-v2" {
		t.Fatalf("expected If-Range to be preserved, got %q", got.Get("If-Range"))
	}
}

func TestCompactTunnelRequestHeadersForUDPBudget_CompactsContentTypeParameters(t *testing.T) {
	selected := http.Header{}
	selected.Set("Content-Type", "application/json; charset=UTF-8")
	prepared := &mappingForwardRequest{
		Method:    http.MethodPost,
		TargetURL: &url.URL{Path: "/api/gmvcs/uap/cas/login"},
		Body:      []byte(`{"username":"a"}`),
	}
	got := compactTunnelRequestHeadersForUDPBudget(prepared, selected)
	if got.Get("Content-Type") != "application/json" {
		t.Fatalf("expected compacted Content-Type application/json, got %q", got.Get("Content-Type"))
	}
}

func TestCompactTunnelRequestHeadersForUDPSevereBudget_PreservesDownloadTransferID(t *testing.T) {
	selected := http.Header{}
	selected.Set(downloadTransferIDHeader, "outer-download-req-789")
	selected.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	selected.Set("Cookie", "JSESSIONID=session-1234567890; XSRF-TOKEN=csrf-abcdef")
	prepared := &mappingForwardRequest{
		Method:    http.MethodPost,
		TargetURL: &url.URL{Path: "/api/gmvcs/uap/cas/login"},
		Body:      []byte("username=a&password=b"),
	}
	got := compactTunnelRequestHeadersForUDPSevereBudget(prepared, selected)
	if got.Get(downloadTransferIDHeader) != "outer-download-req-789" {
		t.Fatalf("expected %s to survive UDP severe budget compaction, got %q", downloadTransferIDHeader, got.Get(downloadTransferIDHeader))
	}
}

func TestCompactTunnelRequestHeadersForUDPSevereBudget_PrefersCookieBackedLoginRescue(t *testing.T) {
	cookie := strings.Join([]string{
		"JSESSIONID=session-1234567890",
		"XSRF-TOKEN=csrf-abcdef",
		"analytics_id=abcdefghijklmnopqrstuvwxyz0123456789",
		"tracking_id=abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz0123456789",
	}, "; ")
	selected := http.Header{}
	selected.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	selected.Set("Authorization", "Bearer should-drop")
	selected.Set("Cookie", cookie)
	prepared := &mappingForwardRequest{
		Method:    http.MethodPost,
		TargetURL: &url.URL{Path: "/api/gmvcs/uap/cas/login"},
		Body:      []byte("username=a&password=b"),
	}
	got := compactTunnelRequestHeadersForUDPSevereBudget(prepared, selected)
	if got.Get("Authorization") != "" {
		t.Fatalf("expected Authorization to be dropped in severe rescue, got %q", got.Get("Authorization"))
	}
	if got.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("expected compacted form Content-Type, got %q", got.Get("Content-Type"))
	}
	if cookie := got.Get("Cookie"); cookie == "" {
		t.Fatal("expected Cookie to remain for severe rescue")
	} else if len(cookie) > 64 {
		t.Fatalf("expected Cookie length <= 64, got %d (%q)", len(cookie), cookie)
	}
}
