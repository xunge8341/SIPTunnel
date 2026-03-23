package httpinvoke

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMapByTemplate(t *testing.T) {
	params := map[string]any{
		"biz": map[string]any{
			"id":   "ORD-001",
			"name": "alice",
		},
	}

	mapped := mapByTemplate(map[string]string{
		"order_id":  "biz.id",
		"user_name": "biz.name",
		"missing":   "biz.none",
	}, params)

	if mapped["order_id"] != "ORD-001" {
		t.Fatalf("unexpected order_id: %v", mapped["order_id"])
	}
	if mapped["user_name"] != "alice" {
		t.Fatalf("unexpected user_name: %v", mapped["user_name"])
	}
	if _, ok := mapped["missing"]; ok {
		t.Fatalf("missing mapping key should not exist")
	}
}

func TestInvokeInjectHeadersAndIdempotentKey(t *testing.T) {
	var capturedHeader http.Header
	var capturedBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Clone()
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	addr := srv.Listener.Addr().(*net.TCPAddr)
	route := RouteConfig{
		APICode:     "api.order.create",
		TargetHost:  addr.IP.String(),
		TargetPort:  addr.Port,
		HTTPMethod:  http.MethodPost,
		HTTPPath:    "/",
		ContentType: "application/json",
		HeaderMapping: map[string]string{
			"X-Partner-ID": "header.partner_id",
		},
		BodyMapping: map[string]string{
			"order_id": "body.id",
		},
	}

	invoker, err := NewInvoker(Config{Routes: []RouteConfig{route}}, srv.Client())
	if err != nil {
		t.Fatalf("NewInvoker() err=%v", err)
	}

	_, err = invoker.Invoke(context.Background(), InvokeInput{
		APICode: "api.order.create",
		Context: RequestContext{
			RequestID:     "req-1",
			TraceID:       "trace-1",
			SessionID:     "session-1",
			TransferID:    "transfer-1",
			SourceSystem:  "B-SYS",
			IdempotentKey: "idem-1",
		},
		Params: map[string]any{
			"header": map[string]any{"partner_id": "P-01"},
			"body":   map[string]any{"id": "order-01"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke() err=%v", err)
	}

	if capturedHeader.Get("X-Request-ID") != "req-1" ||
		capturedHeader.Get("X-Trace-ID") != "trace-1" ||
		capturedHeader.Get("X-Session-ID") != "session-1" ||
		capturedHeader.Get("X-Transfer-ID") != "transfer-1" ||
		capturedHeader.Get("X-Api-Code") != "api.order.create" ||
		capturedHeader.Get("X-Source-System") != "B-SYS" ||
		capturedHeader.Get("X-Idempotent-Key") != "idem-1" {
		t.Fatalf("unified headers not injected correctly: %#v", capturedHeader)
	}
	if capturedHeader.Get("X-Partner-ID") != "P-01" {
		t.Fatalf("mapped header not injected: %#v", capturedHeader)
	}
	if capturedBody["order_id"] != "order-01" {
		t.Fatalf("unexpected mapped body: %#v", capturedBody)
	}
}

func TestMapHTTPStatusToResultCode(t *testing.T) {
	cases := []struct {
		status int
		code   string
	}{
		{status: http.StatusTooManyRequests, code: ResultCodeUpstreamRateLimit},
		{status: http.StatusInternalServerError, code: ResultCodeUpstreamServerErr},
		{status: http.StatusGatewayTimeout, code: ResultCodeUpstreamTimeout},
	}
	for _, tc := range cases {
		if got := MapHTTPStatusToResultCode(tc.status); got != tc.code {
			t.Fatalf("status=%d got=%s want=%s", tc.status, got, tc.code)
		}
	}
}

func TestInvokeWhitelistIntercept(t *testing.T) {
	invoker, err := NewInvoker(Config{Routes: []RouteConfig{{APICode: "allowed"}}}, nil)
	if err != nil {
		t.Fatalf("NewInvoker() err=%v", err)
	}

	_, err = invoker.Invoke(context.Background(), InvokeInput{APICode: "blocked"})
	if !errors.Is(err, ErrRouteNotAllowed) {
		t.Fatalf("expected ErrRouteNotAllowed, got %v", err)
	}
}
