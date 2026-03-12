package service

import (
	"testing"
	"time"
)

func TestRateLimiterBurstAndRefill(t *testing.T) {
	limiter := NewRateLimiter(2, 2)

	if !limiter.Allow() {
		t.Fatalf("expected first request to pass")
	}
	if !limiter.Allow() {
		t.Fatalf("expected second request to pass")
	}
	if limiter.Allow() {
		t.Fatalf("expected third request to be throttled due to burst exhaustion")
	}

	time.Sleep(550 * time.Millisecond)
	if !limiter.Allow() {
		t.Fatalf("expected request to pass after token refill")
	}
}

func TestRateLimiterNeverExceedsCapacity(t *testing.T) {
	limiter := NewRateLimiter(10, 1)

	if !limiter.Allow() {
		t.Fatalf("expected initial request to pass")
	}
	time.Sleep(250 * time.Millisecond)

	if !limiter.Allow() {
		t.Fatalf("expected one token to refill")
	}
	if limiter.Allow() {
		t.Fatalf("expected limiter to cap at burst=1")
	}
}
