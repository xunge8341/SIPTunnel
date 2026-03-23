package memory

import (
	"context"
	"testing"
	"time"

	"siptunnel/internal/repository"
)

func TestTaskRepositoryCRUDAndFilter(t *testing.T) {
	repo := NewTaskRepository()
	now := time.Unix(1000, 0).UTC()

	t1 := repository.Task{
		ID:           "t1",
		TaskType:     repository.TaskTypeCommand,
		RequestID:    "req-1",
		TraceID:      "trace-1",
		SessionID:    "sess-1",
		APICode:      "api-1",
		SourceSystem: "sys-a",
		Status:       repository.TaskStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if _, err := repo.CreateTask(context.Background(), t1); err != nil {
		t.Fatalf("create task t1: %v", err)
	}
	t2 := t1
	t2.ID = "t2"
	t2.TaskType = repository.TaskTypeFile
	t2.TransferID = "xfer-1"
	t2.Status = repository.TaskStatusDeadLettered
	t2.CreatedAt = now.Add(time.Second)
	if _, err := repo.CreateTask(context.Background(), t2); err != nil {
		t.Fatalf("create task t2: %v", err)
	}

	attempt := 1
	if _, err := repo.UpdateTaskStatus(context.Background(), "t1", repository.TaskStatusRetryWait, repository.UpdateTaskStatusOptions{Attempt: &attempt}); err != nil {
		t.Fatalf("update task t1: %v", err)
	}
	if err := repo.SaveTaskEvent(context.Background(), repository.TaskEvent{ID: "evt-1", TaskID: "t1", ToStatus: repository.TaskStatusRetryWait, CreatedAt: now}); err != nil {
		t.Fatalf("save task event: %v", err)
	}

	tasks, err := repo.ListTasks(context.Background(), repository.TaskFilter{OnlyDeadLetter: true})
	if err != nil {
		t.Fatalf("list dead letters: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "t2" {
		t.Fatalf("expected only t2 in dead letter list, got %+v", tasks)
	}
}
