package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const mappingRetryableBodyBytes int64 = 64 * 1024

func shouldEnablePreparedRetry(req *http.Request, contentLength int64) bool {
	if req == nil {
		return false
	}
	if isHealthProbeRequest(req) {
		return true
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodDelete:
		return true
	case http.MethodPut:
		return contentLength >= 0 && contentLength <= mappingRetryableBodyBytes
	default:
		return false
	}
}

func shouldBufferRequestBodyForRetry(retryEnabled bool, contentLength, maxBody int64) bool {
	if !retryEnabled {
		return false
	}
	if contentLength < 0 || maxBody <= 0 {
		return false
	}
	limit := mappingRetryableBodyBytes
	if maxBody < limit {
		limit = maxBody
	}
	return contentLength <= limit
}

func shouldRetryPreparedForward(prepared *mappingForwardRequest, info upstreamErrorInfo, ctx context.Context) bool {
	if prepared == nil || !prepared.RetryEnabled || prepared.RetryAttempts <= 1 {
		return false
	}
	if !info.Temporary {
		return false
	}
	if prepared.BodyStream != nil && prepared.BodyStream != http.NoBody && len(prepared.Body) == 0 {
		return false
	}
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	switch info.Class {
	case upstreamErrorClassTimeout, upstreamErrorClassConnectionRefused, upstreamErrorClassConnectionReset, upstreamErrorClassNetworkUnreachable, upstreamErrorClassCircuitOpen:
		return true
	default:
		return false
	}
}

func mappingRetryBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return 15 * time.Millisecond
	}
	if attempt == 2 {
		return 35 * time.Millisecond
	}
	return 75 * time.Millisecond
}

func isHealthProbePath(target *url.URL) bool {
	if target == nil {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(target.Path))
	switch path {
	case "/healthz", "/readyz", "/livez", "/health", "/ready", "/live":
		return true
	default:
		return false
	}
}
