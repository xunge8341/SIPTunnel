package httpinvoke

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type benchmarkRoundTripper struct{}

func (benchmarkRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"code":"OK"}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func benchmarkInvoker(b *testing.B) *Invoker {
	b.Helper()
	cfg := Config{Routes: []RouteConfig{{
		APICode:     "api.user.create",
		TargetHost:  "127.0.0.1",
		TargetPort:  19001,
		HTTPMethod:  http.MethodPost,
		HTTPPath:    "/internal/users/create",
		ContentType: "application/json",
		TimeoutMS:   200,
		RetryTimes:  0,
		HeaderMapping: map[string]string{
			"X-User-ID": "user.id",
		},
		BodyMapping: map[string]string{
			"user_id":   "user.id",
			"user_name": "user.name",
			"region":    "user.region",
		},
	}}}

	invoker, err := NewInvoker(cfg, &http.Client{Transport: benchmarkRoundTripper{}})
	if err != nil {
		b.Fatalf("new invoker: %v", err)
	}
	return invoker
}

func benchmarkInvokeInput() InvokeInput {
	return InvokeInput{
		APICode: "api.user.create",
		Context: RequestContext{
			RequestID:     "req-bench-1",
			TraceID:       "trace-bench-1",
			SessionID:     "session-bench-1",
			TransferID:    "transfer-bench-1",
			SourceSystem:  "system-b",
			IdempotentKey: "idem-bench-1",
		},
		Params: map[string]any{
			"user": map[string]any{
				"id":     "u-10001",
				"name":   "benchmark-user",
				"region": "cn-north-1",
			},
		},
	}
}

func BenchmarkHTTPMapByTemplate(b *testing.B) {
	mapping := map[string]string{
		"user_id":   "user.id",
		"user_name": "user.name",
		"region":    "user.region",
		"invalid":   "user.invalid",
	}
	params := benchmarkInvokeInput().Params

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mapped := mapByTemplate(mapping, params)
		if len(mapped) != 3 {
			b.Fatalf("mapped size mismatch: got=%d want=3", len(mapped))
		}
	}
}

func BenchmarkHTTPInvokeWrapper(b *testing.B) {
	invoker := benchmarkInvoker(b)
	input := benchmarkInvokeInput()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := invoker.Invoke(ctx, input)
		if err != nil {
			b.Fatalf("invoke failed: %v", err)
		}
		if result.StatusCode != http.StatusOK {
			b.Fatalf("unexpected status: %d", result.StatusCode)
		}
	}
}
