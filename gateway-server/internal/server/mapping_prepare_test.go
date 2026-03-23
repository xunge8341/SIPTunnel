package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"siptunnel/internal/tunnelmapping"
)

func TestPrepareMappingForwardRequestDirectStreamsKnownLengthBody(t *testing.T) {
	mapping := tunnelmapping.TunnelMapping{MappingID: "map-1", RemoteTargetIP: "127.0.0.1", RemoteTargetPort: 8080, MaxRequestBodyBytes: 1024, ConnectTimeoutMS: 100, RequestTimeoutMS: 1000, ResponseTimeoutMS: 1000}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/orders", strings.NewReader(`{"hello":"world"}`))
	prepared, err := prepareMappingForwardRequest(mapping, req, false)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if prepared.BodyStream == nil {
		t.Fatalf("expected streaming body")
	}
	if len(prepared.Body) != 0 {
		t.Fatalf("expected no buffered body, got %d", len(prepared.Body))
	}
	got, err := io.ReadAll(prepared.BodyStream)
	if err != nil {
		t.Fatalf("read streamed body: %v", err)
	}
	if string(got) != `{"hello":"world"}` {
		t.Fatalf("unexpected body: %s", string(got))
	}
}

func TestPrepareMappingForwardRequestTunnelBuffersBody(t *testing.T) {
	mapping := tunnelmapping.TunnelMapping{MappingID: "map-1", RemoteTargetIP: "127.0.0.1", RemoteTargetPort: 8080, MaxRequestBodyBytes: 1024, ConnectTimeoutMS: 100, RequestTimeoutMS: 1000, ResponseTimeoutMS: 1000}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/orders", strings.NewReader(`{"hello":"world"}`))
	prepared, err := prepareMappingForwardRequest(mapping, req, true)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if len(prepared.Body) == 0 {
		t.Fatalf("expected buffered body")
	}
	if string(prepared.Body) != `{"hello":"world"}` {
		t.Fatalf("unexpected body: %s", string(prepared.Body))
	}
}
