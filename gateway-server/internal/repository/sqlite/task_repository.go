package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"siptunnel/internal/repository"
)

type TaskRepository struct {
	db *sql.DB
}

func NewTaskRepository(db *sql.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

func (r *TaskRepository) CreateTask(ctx context.Context, task repository.Task) (repository.Task, error) {
	const query = `
INSERT INTO tasks (
  id, task_type, request_id, trace_id, session_id, transfer_id,
  api_code, source_system, status, result_code, attempt, max_attempts,
  last_error, next_retry_at, created_at, updated_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query,
		task.ID, task.TaskType, task.RequestID, task.TraceID, task.SessionID, task.TransferID,
		task.APICode, task.SourceSystem, task.Status, task.ResultCode, task.Attempt, task.MaxAttempts,
		task.LastError, task.NextRetryAt, task.CreatedAt, task.UpdatedAt, task.CompletedAt,
	)
	if err != nil {
		return repository.Task{}, fmt.Errorf("create task: %w", err)
	}
	return task, nil
}

func (r *TaskRepository) UpdateTaskStatus(ctx context.Context, taskID string, status repository.TaskStatus, opts repository.UpdateTaskStatusOptions) (repository.Task, error) {
	const query = `
UPDATE tasks
SET status = ?, result_code = ?, last_error = ?, attempt = ?, next_retry_at = ?, completed_at = ?, updated_at = ?
WHERE id = ?`
	attempt := 0
	if opts.Attempt != nil {
		attempt = *opts.Attempt
	}
	updatedAt := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, query,
		status, opts.ResultCode, opts.LastError, attempt, opts.NextRetryAt, opts.CompletedAt, updatedAt, taskID,
	)
	if err != nil {
		return repository.Task{}, fmt.Errorf("update task status: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return repository.Task{}, fmt.Errorf("read rows affected: %w", err)
	}
	if rows == 0 {
		return repository.Task{}, repository.ErrTaskNotFound
	}
	return r.GetTaskByID(ctx, taskID)
}

func (r *TaskRepository) GetTaskByID(ctx context.Context, taskID string) (repository.Task, error) {
	const query = `
SELECT id, task_type, request_id, trace_id, session_id, transfer_id,
       api_code, source_system, status, result_code, attempt, max_attempts,
       last_error, next_retry_at, created_at, updated_at, completed_at
FROM tasks WHERE id = ?`

	var task repository.Task
	var nextRetryAt sql.NullTime
	var completedAt sql.NullTime
	row := r.db.QueryRowContext(ctx, query, taskID)
	err := row.Scan(
		&task.ID, &task.TaskType, &task.RequestID, &task.TraceID, &task.SessionID, &task.TransferID,
		&task.APICode, &task.SourceSystem, &task.Status, &task.ResultCode, &task.Attempt, &task.MaxAttempts,
		&task.LastError, &nextRetryAt, &task.CreatedAt, &task.UpdatedAt, &completedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return repository.Task{}, repository.ErrTaskNotFound
		}
		return repository.Task{}, fmt.Errorf("get task by id: %w", err)
	}
	if nextRetryAt.Valid {
		t := nextRetryAt.Time
		task.NextRetryAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time
		task.CompletedAt = &t
	}
	return task, nil
}

func (r *TaskRepository) ListTasks(ctx context.Context, filter repository.TaskFilter) ([]repository.Task, error) {
	query := `
SELECT id, task_type, request_id, trace_id, session_id, transfer_id,
       api_code, source_system, status, result_code, attempt, max_attempts,
       last_error, next_retry_at, created_at, updated_at, completed_at
FROM tasks`
	args := make([]any, 0, 8)
	predicates := make([]string, 0, 8)

	if filter.TaskType != "" {
		predicates = append(predicates, "task_type = ?")
		args = append(args, filter.TaskType)
	}
	if filter.Status != "" {
		predicates = append(predicates, "status = ?")
		args = append(args, filter.Status)
	}
	if filter.RequestID != "" {
		predicates = append(predicates, "request_id = ?")
		args = append(args, filter.RequestID)
	}
	if filter.TraceID != "" {
		predicates = append(predicates, "trace_id = ?")
		args = append(args, filter.TraceID)
	}
	if filter.SessionID != "" {
		predicates = append(predicates, "session_id = ?")
		args = append(args, filter.SessionID)
	}
	if filter.TransferID != "" {
		predicates = append(predicates, "transfer_id = ?")
		args = append(args, filter.TransferID)
	}
	if filter.SourceSystem != "" {
		predicates = append(predicates, "source_system = ?")
		args = append(args, filter.SourceSystem)
	}
	if filter.OnlyDeadLetter {
		predicates = append(predicates, "status = ?")
		args = append(args, repository.TaskStatusDeadLettered)
	}
	if len(predicates) > 0 {
		query += " WHERE " + strings.Join(predicates, " AND ")
	}
	query += " ORDER BY created_at ASC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	result := make([]repository.Task, 0)
	for rows.Next() {
		var task repository.Task
		var nextRetryAt sql.NullTime
		var completedAt sql.NullTime
		if err := rows.Scan(
			&task.ID, &task.TaskType, &task.RequestID, &task.TraceID, &task.SessionID, &task.TransferID,
			&task.APICode, &task.SourceSystem, &task.Status, &task.ResultCode, &task.Attempt, &task.MaxAttempts,
			&task.LastError, &nextRetryAt, &task.CreatedAt, &task.UpdatedAt, &completedAt,
		); err != nil {
			return nil, fmt.Errorf("scan task row: %w", err)
		}
		if nextRetryAt.Valid {
			t := nextRetryAt.Time
			task.NextRetryAt = &t
		}
		if completedAt.Valid {
			t := completedAt.Time
			task.CompletedAt = &t
		}
		result = append(result, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task rows: %w", err)
	}
	return result, nil
}

func (r *TaskRepository) SaveTaskEvent(ctx context.Context, event repository.TaskEvent) error {
	const query = `
INSERT INTO task_events (
  id, task_id, from_status, to_status, result_code, message, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query,
		event.ID, event.TaskID, event.FromStatus, event.ToStatus, event.ResultCode, event.Message, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save task event: %w", err)
	}
	return nil
}
