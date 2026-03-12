package repository

import (
	"context"
	"errors"
	"time"
)

var ErrTaskNotFound = errors.New("task not found")

type TaskType string

const (
	TaskTypeCommand TaskType = "command"
	TaskTypeFile    TaskType = "file"
)

type TaskStatus string

const (
	TaskStatusPending      TaskStatus = "pending"
	TaskStatusAccepted     TaskStatus = "accepted"
	TaskStatusRunning      TaskStatus = "running"
	TaskStatusTransferring TaskStatus = "transferring"
	TaskStatusVerifying    TaskStatus = "verifying"
	TaskStatusRetryWait    TaskStatus = "retry_wait"
	TaskStatusSucceeded    TaskStatus = "succeeded"
	TaskStatusFailed       TaskStatus = "failed"
	TaskStatusDeadLettered TaskStatus = "dead_lettered"
	TaskStatusCancelled    TaskStatus = "cancelled"
)

type Task struct {
	ID           string
	TaskType     TaskType
	RequestID    string
	TraceID      string
	SessionID    string
	TransferID   string
	APICode      string
	SourceSystem string
	Status       TaskStatus
	ResultCode   string
	Attempt      int
	MaxAttempts  int
	LastError    string
	NextRetryAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CompletedAt  *time.Time
}

type TaskEvent struct {
	ID         string
	TaskID     string
	FromStatus TaskStatus
	ToStatus   TaskStatus
	ResultCode string
	Message    string
	CreatedAt  time.Time
}

type TaskFilter struct {
	TaskType       TaskType
	Status         TaskStatus
	RequestID      string
	TraceID        string
	SessionID      string
	TransferID     string
	SourceSystem   string
	OnlyDeadLetter bool
	Limit          int
	Offset         int
}

type UpdateTaskStatusOptions struct {
	ResultCode  string
	LastError   string
	Attempt     *int
	NextRetryAt *time.Time
	CompletedAt *time.Time
}

type TaskRepository interface {
	CreateTask(ctx context.Context, task Task) (Task, error)
	UpdateTaskStatus(ctx context.Context, taskID string, status TaskStatus, opts UpdateTaskStatusOptions) (Task, error)
	GetTaskByID(ctx context.Context, taskID string) (Task, error)
	ListTasks(ctx context.Context, filter TaskFilter) ([]Task, error)
	SaveTaskEvent(ctx context.Context, event TaskEvent) error
}
