package server

import (
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestClassifyUpstreamErrorConnectionRefused(t *testing.T) {
	target, _ := url.Parse("http://10.196.57.191:80/")
	info := classifyUpstreamError(errors.New(`Get "http://10.196.57.191:80/": dial tcp 10.196.57.191:80: connectex: No connection could be made because the target machine actively refused it.`), target)
	if info.Class != upstreamErrorClassConnectionRefused {
		t.Fatalf("expected connection_refused, got %s", info.Class)
	}
	if !info.Temporary {
		t.Fatalf("expected temporary=true")
	}
	if !strings.Contains(info.UserReason, "被拒绝连接") {
		t.Fatalf("unexpected user reason: %s", info.UserReason)
	}
}

func TestUpstreamCircuitGuardBackoffAndRecovery(t *testing.T) {
	guard := newUpstreamCircuitGuard()
	key := "map-1|http://10.0.0.1:80"
	now := time.Date(2026, 3, 17, 1, 0, 0, 0, time.UTC)
	info := upstreamErrorInfo{Class: upstreamErrorClassConnectionRefused, Temporary: true, UserReason: "访问目标被拒绝连接"}
	guard.RecordFailure(key, info, now)
	if err := guard.Before(key, "http://10.0.0.1:80", now.Add(500*time.Millisecond)); err == nil {
		t.Fatalf("expected circuit open error")
	}
	if err := guard.Before(key, "http://10.0.0.1:80", now.Add(2*time.Second)); err != nil {
		t.Fatalf("expected circuit closed after backoff, got %v", err)
	}
	guard.RecordFailure(key, info, now.Add(2*time.Second))
	guard.RecordSuccess(key)
	if err := guard.Before(key, "http://10.0.0.1:80", now.Add(2100*time.Millisecond)); err != nil {
		t.Fatalf("expected success after reset, got %v", err)
	}
}
