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
