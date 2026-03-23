package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
)

func TestShouldEnablePreparedRetryForHealthProbe(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://example.com/healthz", http.NoBody)
	if !shouldEnablePreparedRetry(req, 0) {
		t.Fatalf("expected health probe request to enable retry")
	}
}

func TestShouldBufferRequestBodyForRetry(t *testing.T) {
	if !shouldBufferRequestBodyForRetry(true, 4096, 1<<20) {
		t.Fatalf("expected small retryable body to be buffered")
	}
	if shouldBufferRequestBodyForRetry(true, mappingRetryableBodyBytes+1, 1<<20) {
		t.Fatalf("did not expect oversized body to be buffered for retry")
	}
}

func TestShouldRetryPreparedForward(t *testing.T) {
	prepared := &mappingForwardRequest{RetryEnabled: true, RetryAttempts: 2, Body: []byte("{}")}
	info := upstreamErrorInfo{Class: upstreamErrorClassConnectionReset, Temporary: true}
	if !shouldRetryPreparedForward(prepared, info, context.Background()) {
		t.Fatalf("expected temporary reset to be retried")
	}
	info = upstreamErrorInfo{Class: upstreamErrorClassUnknown, Temporary: false}
	if shouldRetryPreparedForward(prepared, info, context.Background()) {
		t.Fatalf("did not expect non-temporary error to be retried")
	}
}

func TestClassifyUpstreamErrorWindowsReset(t *testing.T) {
	target, _ := url.Parse("http://127.0.0.1:28080/healthz")
	info := classifyUpstreamError(errors.New(`Post "http://127.0.0.1:28080/healthz": wsarecv: An existing connection was forcibly closed by the remote host.`), target)
	if info.Class != upstreamErrorClassConnectionReset || !info.Temporary {
		t.Fatalf("unexpected info: %+v", info)
	}
}
