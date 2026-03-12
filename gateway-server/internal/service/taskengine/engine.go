package taskengine

import (
	"context"
	"fmt"
	"time"

	"siptunnel/internal/repository"
	"siptunnel/internal/service"
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

type Engine struct {
	repo        repository.TaskRepository
	commandSM   *StateMachine
	fileSM      *StateMachine
	retryPolicy service.RetryPolicy
	clock       Clock
}

type CreateTaskInput struct {
	ID           string
	TaskType     repository.TaskType
	RequestID    string
	TraceID      string
	SessionID    string
	TransferID   string
	APICode      string
	SourceSystem string
	MaxAttempts  int
}

func NewEngine(repo repository.TaskRepository, retryPolicy service.RetryPolicy) *Engine {
	if retryPolicy.MaxAttempts <= 0 {
		retryPolicy.MaxAttempts = 3
	}
	if retryPolicy.BaseBackoff <= 0 {
		retryPolicy.BaseBackoff = 2 * time.Second
	}
	return &Engine{
		repo:        repo,
		commandSM:   NewCommandStateMachine(),
		fileSM:      NewFileStateMachine(),
		retryPolicy: retryPolicy,
		clock:       realClock{},
	}
}

func (e *Engine) CreateTask(ctx context.Context, input CreateTaskInput) (repository.Task, error) {
	now := e.clock.Now()
	maxAttempts := input.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = e.retryPolicy.MaxAttempts
	}
	task := repository.Task{
		ID:           input.ID,
		TaskType:     input.TaskType,
		RequestID:    input.RequestID,
		TraceID:      input.TraceID,
		SessionID:    input.SessionID,
		TransferID:   input.TransferID,
		APICode:      input.APICode,
		SourceSystem: input.SourceSystem,
		Status:       repository.TaskStatusPending,
		MaxAttempts:  maxAttempts,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	created, err := e.repo.CreateTask(ctx, task)
	if err != nil {
		return repository.Task{}, err
	}
	if err := e.repo.SaveTaskEvent(ctx, repository.TaskEvent{
		ID:         eventID(input.ID, now),
		TaskID:     input.ID,
		FromStatus: "",
		ToStatus:   repository.TaskStatusPending,
		Message:    "task created",
		CreatedAt:  now,
	}); err != nil {
		return repository.Task{}, err
	}
	return created, nil
}

func (e *Engine) TransitTask(ctx context.Context, taskID string, to repository.TaskStatus, resultCode string, message string) (repository.Task, error) {
	task, err := e.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return repository.Task{}, err
	}
	if err := e.stateMachine(task.TaskType).Validate(Transition{From: task.Status, To: to}); err != nil {
		return repository.Task{}, err
	}
	now := e.clock.Now()
	var completedAt *time.Time
	if isTerminalStatus(to) {
		completedAt = &now
	}
	updated, err := e.repo.UpdateTaskStatus(ctx, task.ID, to, repository.UpdateTaskStatusOptions{
		ResultCode:  resultCode,
		CompletedAt: completedAt,
	})
	if err != nil {
		return repository.Task{}, err
	}
	if err := e.repo.SaveTaskEvent(ctx, repository.TaskEvent{
		ID:         eventID(task.ID, now),
		TaskID:     task.ID,
		FromStatus: task.Status,
		ToStatus:   to,
		ResultCode: resultCode,
		Message:    message,
		CreatedAt:  now,
	}); err != nil {
		return repository.Task{}, err
	}
	return updated, nil
}

func (e *Engine) HandleFailure(ctx context.Context, taskID string, resultCode string, failureReason string) (repository.Task, error) {
	task, err := e.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return repository.Task{}, err
	}
	now := e.clock.Now()
	nextAttempt := task.Attempt + 1
	maxAttempts := task.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = e.retryPolicy.MaxAttempts
	}

	if err := e.stateMachine(task.TaskType).Validate(Transition{From: task.Status, To: repository.TaskStatusFailed}); err != nil {
		return repository.Task{}, err
	}
	if _, err := e.repo.UpdateTaskStatus(ctx, task.ID, repository.TaskStatusFailed, repository.UpdateTaskStatusOptions{
		ResultCode: resultCode,
		LastError:  failureReason,
		Attempt:    &nextAttempt,
	}); err != nil {
		return repository.Task{}, err
	}
	if err := e.repo.SaveTaskEvent(ctx, repository.TaskEvent{
		ID:         eventID(task.ID, now),
		TaskID:     task.ID,
		FromStatus: task.Status,
		ToStatus:   repository.TaskStatusFailed,
		ResultCode: resultCode,
		Message:    failureReason,
		CreatedAt:  now,
	}); err != nil {
		return repository.Task{}, err
	}

	if nextAttempt >= maxAttempts {
		return e.moveToDeadLetter(ctx, task.ID, resultCode, "max retry attempts exceeded")
	}

	nextRetryAt := now.Add(e.retryPolicy.NextDelay(nextAttempt + 1))
	retryStatus, err := e.repo.UpdateTaskStatus(ctx, task.ID, repository.TaskStatusRetryWait, repository.UpdateTaskStatusOptions{
		ResultCode:  resultCode,
		LastError:   failureReason,
		Attempt:     &nextAttempt,
		NextRetryAt: &nextRetryAt,
	})
	if err != nil {
		return repository.Task{}, err
	}
	if err := e.repo.SaveTaskEvent(ctx, repository.TaskEvent{
		ID:         eventID(task.ID, nextRetryAt),
		TaskID:     task.ID,
		FromStatus: repository.TaskStatusFailed,
		ToStatus:   repository.TaskStatusRetryWait,
		ResultCode: resultCode,
		Message:    "scheduled for retry",
		CreatedAt:  now,
	}); err != nil {
		return repository.Task{}, err
	}
	return retryStatus, nil
}

func (e *Engine) moveToDeadLetter(ctx context.Context, taskID string, resultCode string, message string) (repository.Task, error) {
	task, err := e.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return repository.Task{}, err
	}
	now := e.clock.Now()
	if err := e.stateMachine(task.TaskType).Validate(Transition{From: task.Status, To: repository.TaskStatusDeadLettered}); err != nil {
		return repository.Task{}, err
	}
	updated, err := e.repo.UpdateTaskStatus(ctx, taskID, repository.TaskStatusDeadLettered, repository.UpdateTaskStatusOptions{
		ResultCode:  resultCode,
		LastError:   message,
		CompletedAt: &now,
	})
	if err != nil {
		return repository.Task{}, err
	}
	if err := e.repo.SaveTaskEvent(ctx, repository.TaskEvent{
		ID:         eventID(task.ID, now),
		TaskID:     task.ID,
		FromStatus: task.Status,
		ToStatus:   repository.TaskStatusDeadLettered,
		ResultCode: resultCode,
		Message:    message,
		CreatedAt:  now,
	}); err != nil {
		return repository.Task{}, err
	}
	return updated, nil
}

func (e *Engine) ListDeadLetters(ctx context.Context, limit int) ([]repository.Task, error) {
	return e.repo.ListTasks(ctx, repository.TaskFilter{
		OnlyDeadLetter: true,
		Limit:          limit,
	})
}

func (e *Engine) ReplayDeadLetter(ctx context.Context, taskID string) (repository.Task, error) {
	task, err := e.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return repository.Task{}, err
	}
	if task.Status != repository.TaskStatusDeadLettered {
		return repository.Task{}, fmt.Errorf("task %s is not in dead-letter queue", taskID)
	}
	attempt := 0
	now := e.clock.Now()
	updated, err := e.repo.UpdateTaskStatus(ctx, taskID, repository.TaskStatusRetryWait, repository.UpdateTaskStatusOptions{
		ResultCode:  "",
		LastError:   "",
		Attempt:     &attempt,
		NextRetryAt: &now,
	})
	if err != nil {
		return repository.Task{}, err
	}
	if err := e.repo.SaveTaskEvent(ctx, repository.TaskEvent{
		ID:         eventID(task.ID, now),
		TaskID:     task.ID,
		FromStatus: task.Status,
		ToStatus:   repository.TaskStatusRetryWait,
		Message:    "replayed from dead-letter queue",
		CreatedAt:  now,
	}); err != nil {
		return repository.Task{}, err
	}
	return updated, nil
}

func (e *Engine) stateMachine(taskType repository.TaskType) *StateMachine {
	if taskType == repository.TaskTypeFile {
		return e.fileSM
	}
	return e.commandSM
}

func isTerminalStatus(status repository.TaskStatus) bool {
	switch status {
	case repository.TaskStatusSucceeded, repository.TaskStatusCancelled, repository.TaskStatusDeadLettered:
		return true
	default:
		return false
	}
}

func eventID(taskID string, ts time.Time) string {
	return fmt.Sprintf("%s-%d", taskID, ts.UnixNano())
}
