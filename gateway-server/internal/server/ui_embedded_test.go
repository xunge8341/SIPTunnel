package server

import (
	"net/http"
	"net/http/httptest"
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

	req = httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "pong" {
		t.Fatalf("/api/ping got status=%d body=%q", rr.Code, rr.Body.String())
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
