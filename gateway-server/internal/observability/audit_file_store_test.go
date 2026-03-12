package observability

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileBackedAuditStoreRecord(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileBackedAuditStore(dir)
	if err != nil {
		t.Fatalf("NewFileBackedAuditStore failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	event := AuditEvent{Who: "ops", When: time.Now().UTC(), RequestType: "demo", FinalResult: "OK"}
	if err := store.Record(context.Background(), event); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	events, err := store.List(context.Background(), AuditQuery{Who: "ops", Limit: 10})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	b, err := os.ReadFile(filepath.Join(dir, "audit-events.jsonl"))
	if err != nil {
		t.Fatalf("read audit file failed: %v", err)
	}
	if !strings.Contains(string(b), `"who":"ops"`) {
		t.Fatalf("unexpected file content: %s", string(b))
	}
}
