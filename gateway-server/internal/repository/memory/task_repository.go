package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"siptunnel/internal/repository"
)

type TaskRepository struct {
	mu     sync.RWMutex
	tasks  map[string]repository.Task
	events map[string][]repository.TaskEvent
}

func NewTaskRepository() *TaskRepository {
	return &TaskRepository{
		tasks:  make(map[string]repository.Task),
		events: make(map[string][]repository.TaskEvent),
	}
}

func (r *TaskRepository) CreateTask(_ context.Context, task repository.Task) (repository.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tasks[task.ID]; exists {
		return repository.Task{}, fmt.Errorf("task %s already exists", task.ID)
	}
	r.tasks[task.ID] = task
	return task, nil
}

func (r *TaskRepository) UpdateTaskStatus(_ context.Context, taskID string, status repository.TaskStatus, opts repository.UpdateTaskStatusOptions) (repository.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return repository.Task{}, repository.ErrTaskNotFound
	}
	task.Status = status
	task.ResultCode = opts.ResultCode
	task.LastError = opts.LastError
	if opts.Attempt != nil {
		task.Attempt = *opts.Attempt
	}
	task.NextRetryAt = opts.NextRetryAt
	if opts.CompletedAt != nil {
		task.CompletedAt = opts.CompletedAt
	}
	task.UpdatedAt = time.Now().UTC()
	r.tasks[task.ID] = task
	return task, nil
}

func (r *TaskRepository) GetTaskByID(_ context.Context, taskID string) (repository.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.tasks[taskID]
	if !ok {
		return repository.Task{}, repository.ErrTaskNotFound
	}
	return task, nil
}

func (r *TaskRepository) ListTasks(_ context.Context, filter repository.TaskFilter) ([]repository.Task, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tasks []repository.Task
	for _, task := range r.tasks {
		if !matchesFilter(task, filter) {
			continue
		}
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})

	start := filter.Offset
	if start > len(tasks) {
		return []repository.Task{}, nil
	}
	end := len(tasks)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return tasks[start:end], nil
}

func (r *TaskRepository) SaveTaskEvent(_ context.Context, event repository.TaskEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tasks[event.TaskID]; !ok {
		return repository.ErrTaskNotFound
	}
	r.events[event.TaskID] = append(r.events[event.TaskID], event)
	return nil
}

func matchesFilter(task repository.Task, filter repository.TaskFilter) bool {
	if filter.TaskType != "" && task.TaskType != filter.TaskType {
		return false
	}
	if filter.Status != "" && task.Status != filter.Status {
		return false
	}
	if filter.RequestID != "" && task.RequestID != filter.RequestID {
		return false
	}
	if filter.TraceID != "" && task.TraceID != filter.TraceID {
		return false
	}
	if filter.SessionID != "" && task.SessionID != filter.SessionID {
		return false
	}
	if filter.TransferID != "" && task.TransferID != filter.TransferID {
		return false
	}
	if filter.SourceSystem != "" && task.SourceSystem != filter.SourceSystem {
		return false
	}
	if filter.OnlyDeadLetter && task.Status != repository.TaskStatusDeadLettered {
		return false
	}
	return true
}
