package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"siptunnel/internal/observability"
	"siptunnel/internal/repository"
)

func TestSQLiteStore_TaskAndAuditPersistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gateway.db")
	store, err := OpenSQLiteStore(dbPath, RetentionPolicy{})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	repo := store.TaskRepository()
	now := time.Now().UTC()
	task := repository.Task{ID: "t-1", TaskType: "cmd", RequestID: "r-1", TraceID: "tr-1", Status: repository.TaskStatusPending, CreatedAt: now, UpdatedAt: now}
	if _, err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	got, err := repo.GetTaskByID(context.Background(), "t-1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.ID != task.ID {
		t.Fatalf("task id=%s, want %s", got.ID, task.ID)
	}

	err = store.Record(context.Background(), observability.AuditEvent{Who: "ops", When: now, FinalResult: "OK", Core: observability.CoreFields{RequestID: "r-1", TraceID: "tr-1", APICode: "map.create"}})
	if err != nil {
		t.Fatalf("record audit: %v", err)
	}
	audits, err := store.List(context.Background(), observability.AuditQuery{RequestID: "r-1", Limit: 10})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("audit len=%d, want 1", len(audits))
	}
}

func TestSQLiteStore_CleanupRetention(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gateway.db")
	store, err := OpenSQLiteStore(dbPath, RetentionPolicy{MaxTaskRecords: 2, MaxTaskAgeDays: 1, MaxAuditRecords: 2, MaxAuditAgeDays: 1})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer store.Close()

	repo := store.TaskRepository()
	old := time.Now().UTC().Add(-48 * time.Hour)
	for i := 0; i < 4; i++ {
		ts := old
		if i >= 2 {
			ts = time.Now().UTC().Add(time.Duration(i) * time.Minute)
		}
		if _, err := repo.CreateTask(context.Background(), repository.Task{ID: "task-" + string(rune('a'+i)), TaskType: "cmd", Status: repository.TaskStatusPending, CreatedAt: ts, UpdatedAt: ts}); err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
		if err := store.Record(context.Background(), observability.AuditEvent{Who: "ops", When: ts, FinalResult: "OK", Core: observability.CoreFields{RequestID: "r"}}); err != nil {
			t.Fatalf("record audit %d: %v", i, err)
		}
	}

	if err := store.Cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	tasks, err := repo.ListTasks(context.Background(), repository.TaskFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(tasks) > 2 {
		t.Fatalf("task len=%d, want <=2", len(tasks))
	}
	audits, err := store.List(context.Background(), observability.AuditQuery{Limit: 10})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) > 2 {
		t.Fatalf("audit len=%d, want <=2", len(audits))
	}
}
