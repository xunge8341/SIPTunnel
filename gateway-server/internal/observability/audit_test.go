package observability

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryAuditStoreListFilter(t *testing.T) {
	store := NewInMemoryAuditStore()
	now := time.Now().UTC()

	_ = store.Record(context.Background(), AuditEvent{Who: "alice", When: now, Core: CoreFields{RequestID: "r1", APICode: "A"}})
	_ = store.Record(context.Background(), AuditEvent{Who: "bob", When: now.Add(time.Second), Core: CoreFields{RequestID: "r2", APICode: "B"}})

	events, err := store.List(context.Background(), AuditQuery{Who: "bob", APICode: "B", Limit: 5})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Who != "bob" {
		t.Fatalf("unexpected who: %s", events[0].Who)
	}
}

func TestInMemoryAuditStoreAdvancedFilters(t *testing.T) {
	store := NewInMemoryAuditStore()
	base := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)

	_ = store.Record(context.Background(), AuditEvent{
		Who:               "ops",
		When:              base,
		RequestType:       "ops",
		LocalServiceRoute: "gateway-server",
		FinalResult:       "OK",
		OpsAction:         "UPDATE_LIMITS",
		Core:              CoreFields{APICode: "asset.sync"},
	})
	_ = store.Record(context.Background(), AuditEvent{
		Who:               "ops",
		When:              base.Add(30 * time.Minute),
		RequestType:       "demo.process",
		LocalServiceRoute: "local.mock.service",
		FinalResult:       "UPSTREAM_TIMEOUT",
		OpsAction:         "NONE",
		Core:              CoreFields{APICode: "asset.upload"},
	})
	_ = store.Record(context.Background(), AuditEvent{
		Who:               "ops",
		When:              base.Add(90 * time.Minute),
		RequestType:       "ops",
		LocalServiceRoute: "gateway-server",
		FinalResult:       "VALIDATION_FAILED",
		OpsAction:         "UPDATE_ROUTE",
		Core:              CoreFields{APICode: "asset.route"},
	})

	events, err := store.List(context.Background(), AuditQuery{
		Rule:      "upload",
		ErrorOnly: true,
		StartTime: base.Add(15 * time.Minute),
		EndTime:   base.Add(45 * time.Minute),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event with advanced filters, got %d", len(events))
	}
	if events[0].Core.APICode != "asset.upload" || events[0].FinalResult != "UPSTREAM_TIMEOUT" {
		t.Fatalf("unexpected event returned: %+v", events[0])
	}
}
