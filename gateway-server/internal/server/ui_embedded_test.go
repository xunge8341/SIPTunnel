package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedUIFallbackServesIndexAndAssets(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ping" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("pong"))
			return
		}
		http.NotFound(w, r)
	})
	h, err := NewEmbeddedUIFallbackHandler(api, EmbeddedUIOptions{BasePath: "/"})
	if err != nil {
		t.Fatalf("NewEmbeddedUIFallbackHandler() error=%v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/dashboard status=%d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("index cache-control=%q, want no-store", got)
	}
	if !strings.Contains(rr.Body.String(), "<div id=\"app\"></div>") {
		t.Fatalf("index body should contain Vue app mount node, got=%q", rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "pong" {
		t.Fatalf("/api/ping got status=%d body=%q", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/favicon.svg status=%d, want 200", rr.Code)
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=604800, immutable" {
		t.Fatalf("favicon cache-control=%q", got)
	}
}

func TestEmbeddedUIFallbackRespectsBasePath(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	h, err := NewEmbeddedUIFallbackHandler(api, EmbeddedUIOptions{BasePath: "/ops"})
	if err != nil {
		t.Fatalf("NewEmbeddedUIFallbackHandler() error=%v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ops/dashboard", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/ops/dashboard status=%d, want 200", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("/dashboard status=%d, want 404", rr.Code)
	}
}

func TestEmbeddedUIFallbackInjectsRouterBaseForNonRoot(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	h, err := NewEmbeddedUIFallbackHandler(api, EmbeddedUIOptions{BasePath: "/ops"})
	if err != nil {
		t.Fatalf("NewEmbeddedUIFallbackHandler() error=%v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ops/dashboard", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("/ops/dashboard status=%d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `meta name="siptunnel-ui-base-path" content="/ops/"`) {
		t.Fatalf("expected injected base path meta, got=%q", rr.Body.String())
	}
}

func TestEmbeddedUIFallbackFriendly404(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	h, err := NewEmbeddedUIFallbackHandler(api, EmbeddedUIOptions{BasePath: "/"})
	if err != nil {
		t.Fatalf("NewEmbeddedUIFallbackHandler() error=%v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/not-exist.js", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("friendly 404 status=%d, want 404", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "页面未找到") {
		t.Fatalf("friendly 404 body=%q", rr.Body.String())
	}
}

func TestEmbeddedUIFallbackBypassesReservedRootProbePaths(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	h, err := NewEmbeddedUIFallbackHandler(api, EmbeddedUIOptions{BasePath: "/"})
	if err != nil {
		t.Fatalf("handler init failed: %v", err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected json content-type, got %q body=%s", got, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `{"ok":true}`) {
		t.Fatalf("expected api response, got %s", rr.Body.String())
	}
}

func TestEmbeddedUIFallbackBypassesMetricsPath(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("demo_metric 1\n"))
	})
	h, err := NewEmbeddedUIFallbackHandler(api, EmbeddedUIOptions{BasePath: "/"})
	if err != nil {
		t.Fatalf("handler init failed: %v", err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("expected metrics content-type, got %q body=%s", got, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `demo_metric 1`) {
		t.Fatalf("expected metrics body, got %s", rr.Body.String())
	}
}

func TestReadEmbeddedUIDeliveryMetadataReportsAlignedBundle(t *testing.T) {
	meta := ReadEmbeddedUIDeliveryMetadata()
	if !meta.MetadataPresent {
		t.Fatal("expected embedded ui metadata to be present")
	}
	if meta.ConsistencyStatus != "aligned" {
		t.Fatalf("ConsistencyStatus=%q, want aligned", meta.ConsistencyStatus)
	}
	if meta.AssetBaseMode != "relative_assets+basepath_meta" {
		t.Fatalf("AssetBaseMode=%q, want relative_assets+basepath_meta", meta.AssetBaseMode)
	}
	if meta.BuildNonce == "" || meta.EmbeddedHashSHA256 == "" {
		t.Fatalf("expected non-empty build nonce/hash, got nonce=%q hash=%q", meta.BuildNonce, meta.EmbeddedHashSHA256)
	}
	if meta.DeliveryGuardStatus != "aligned" {
		t.Fatalf("DeliveryGuardStatus=%q, want aligned", meta.DeliveryGuardStatus)
	}
	if meta.DeliveryGuardRemaining != 0 || meta.DeliveryGuardActiveHits != 0 {
		t.Fatalf("expected no delivery guard drift, got remaining=%d hits=%d", meta.DeliveryGuardRemaining, meta.DeliveryGuardActiveHits)
	}
}
