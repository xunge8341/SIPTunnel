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
