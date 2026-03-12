package taskengine

import (
	"context"
	"testing"
	"time"

	"siptunnel/internal/repository"
	"siptunnel/internal/repository/memory"
	"siptunnel/internal/service"
)

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

func TestEngineHandleFailureRetryAndDeadLetter(t *testing.T) {
	repo := memory.NewTaskRepository()
	engine := NewEngine(repo, service.RetryPolicy{MaxAttempts: 2, BaseBackoff: time.Second})
	engine.clock = fixedClock{now: time.Unix(10, 0).UTC()}

	_, err := engine.CreateTask(context.Background(), CreateTaskInput{
		ID:           "cmd-1",
		TaskType:     repository.TaskTypeCommand,
		RequestID:    "req-1",
		TraceID:      "trace-1",
		SessionID:    "sess-1",
		APICode:      "api.demo",
		SourceSystem: "edge-a",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := engine.TransitTask(context.Background(), "cmd-1", repository.TaskStatusAccepted, "", "accepted"); err != nil {
		t.Fatalf("transit to accepted: %v", err)
	}
	if _, err := engine.TransitTask(context.Background(), "cmd-1", repository.TaskStatusRunning, "", "running"); err != nil {
		t.Fatalf("transit to running: %v", err)
	}

	firstFailure, err := engine.HandleFailure(context.Background(), "cmd-1", "E_TIMEOUT", "timeout")
	if err != nil {
		t.Fatalf("handle first failure: %v", err)
	}
	if firstFailure.Status != repository.TaskStatusRetryWait {
		t.Fatalf("expected retry_wait after first failure, got %s", firstFailure.Status)
	}
	if firstFailure.Attempt != 1 {
		t.Fatalf("expected attempt to be 1, got %d", firstFailure.Attempt)
	}

	if _, err := engine.TransitTask(context.Background(), "cmd-1", repository.TaskStatusRunning, "", "retry running"); err != nil {
		t.Fatalf("transit retry_wait->running: %v", err)
	}
	secondFailure, err := engine.HandleFailure(context.Background(), "cmd-1", "E_TIMEOUT", "timeout again")
	if err != nil {
		t.Fatalf("handle second failure: %v", err)
	}
	if secondFailure.Status != repository.TaskStatusDeadLettered {
		t.Fatalf("expected dead_lettered after second failure, got %s", secondFailure.Status)
	}
}

func TestEngineReplayDeadLetter(t *testing.T) {
	repo := memory.NewTaskRepository()
	engine := NewEngine(repo, service.RetryPolicy{MaxAttempts: 1, BaseBackoff: time.Second})
	engine.clock = fixedClock{now: time.Unix(100, 0).UTC()}

	_, err := engine.CreateTask(context.Background(), CreateTaskInput{
		ID:           "file-1",
		TaskType:     repository.TaskTypeFile,
		RequestID:    "req-file",
		TraceID:      "trace-file",
		SessionID:    "sess-file",
		TransferID:   "xfer-1",
		APICode:      "file.recv",
		SourceSystem: "edge-b",
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := engine.TransitTask(context.Background(), "file-1", repository.TaskStatusAccepted, "", "accepted"); err != nil {
		t.Fatalf("transit accepted: %v", err)
	}
	if _, err := engine.TransitTask(context.Background(), "file-1", repository.TaskStatusTransferring, "", "start transfer"); err != nil {
		t.Fatalf("transit transferring: %v", err)
	}
	if _, err := engine.HandleFailure(context.Background(), "file-1", "E_CORRUPT", "checksum mismatch"); err != nil {
		t.Fatalf("handle failure to dead letter: %v", err)
	}

	task, err := engine.ReplayDeadLetter(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("replay dead letter: %v", err)
	}
	if task.Status != repository.TaskStatusRetryWait {
		t.Fatalf("expected replayed task to be retry_wait, got %s", task.Status)
	}
	if task.Attempt != 0 {
		t.Fatalf("expected attempt to reset to 0, got %d", task.Attempt)
	}
}
