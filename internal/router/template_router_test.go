package router

import "testing"

func TestTemplateRouterControlledRoutes(t *testing.T) {
	r := NewTemplateRouter("http://localhost:8080", map[string]RouteTemplate{
		"A": {Method: "POST", Path: "/a"},
	})
	_, err := r.BuildRequest("UNKNOWN", map[string]any{})
	if err == nil {
		t.Fatalf("expected error on unknown api code")
	}
	req, err := r.BuildRequest("A", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("BuildRequest() err=%v", err)
	}
	if req.URL.String() != "http://localhost:8080/a" {
		t.Fatalf("unexpected url %s", req.URL)
	}
}
