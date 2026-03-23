package server

import "testing"

func TestProtectionRuntimeAutoRestrictionAfterRepeatedRateLimit(t *testing.T) {
	runtime := newProtectionRuntime(OpsLimits{RPS: 1, Burst: 1, MaxConcurrent: 4})
	release, err := runtime.Acquire("map-auto", "10.0.0.9")
	if err != nil {
		t.Fatalf("expected initial acquire to succeed: %v", err)
	}
	defer release()

	for i := 0; i < autoRestrictionRateLimitThreshold; i++ {
		if rel, err := runtime.Acquire("map-auto", "10.0.0.9"); err == nil && rel != nil {
			rel()
		}
	}

	snapshot := runtime.Snapshot()
	found := false
	for _, item := range snapshot.Restrictions {
		if item.Scope == "source" && item.Target == "10.0.0.9" {
			if !item.Auto || !item.AutoRelease {
				t.Fatalf("expected auto restriction with auto release, got %+v", item)
			}
			if item.Trigger == "" {
				t.Fatalf("expected trigger to be populated, got %+v", item)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("expected auto restriction for source target, snapshot=%+v", snapshot.Restrictions)
	}

	if _, err := runtime.Acquire("map-auto", "10.0.0.9"); err == nil {
		t.Fatalf("expected acquire to be blocked after auto restriction")
	} else if pre := classifyProtectionReject(err); pre == nil || pre.Kind != "temporary_block" {
		t.Fatalf("expected temporary_block after auto restriction, got %v", err)
	}
}
