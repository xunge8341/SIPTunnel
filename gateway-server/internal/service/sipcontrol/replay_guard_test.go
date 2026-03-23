package sipcontrol

import (
	"testing"
	"time"
)

func TestInMemoryReplayGuardAccept(t *testing.T) {
	now := time.Now().UTC()
	guard := NewInMemoryReplayGuard(2 * time.Minute)

	if err := guard.Accept("req-1", "nonce-1", now.Add(time.Minute), now); err != nil {
		t.Fatalf("first accept failed: %v", err)
	}
	if err := guard.Accept("req-1", "nonce-2", now.Add(time.Minute), now); err == nil {
		t.Fatalf("expected duplicate request_id to be rejected")
	}
	if err := guard.Accept("req-2", "nonce-1", now.Add(time.Minute), now); err == nil {
		t.Fatalf("expected duplicate nonce to be rejected")
	}
}

func TestInMemoryReplayGuardGC(t *testing.T) {
	now := time.Now().UTC()
	guard := NewInMemoryReplayGuard(30 * time.Second)

	if err := guard.Accept("req-1", "nonce-1", now.Add(10*time.Second), now); err != nil {
		t.Fatalf("first accept failed: %v", err)
	}
	later := now.Add(31 * time.Second)
	if err := guard.Accept("req-1", "nonce-1", later.Add(time.Minute), later); err != nil {
		t.Fatalf("accept after expiration should pass: %v", err)
	}
}
