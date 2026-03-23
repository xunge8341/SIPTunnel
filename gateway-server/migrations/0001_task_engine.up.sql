-- 任务引擎主表，覆盖命令任务与文件任务
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  task_type TEXT NOT NULL,
  request_id TEXT NOT NULL,
  trace_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  transfer_id TEXT NOT NULL DEFAULT '',
  api_code TEXT NOT NULL,
  source_system TEXT NOT NULL,
  status TEXT NOT NULL,
  result_code TEXT NOT NULL DEFAULT '',
  attempt INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 3,
  last_error TEXT NOT NULL DEFAULT '',
  next_retry_at DATETIME NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  completed_at DATETIME NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_created_at ON tasks(status, created_at);
CREATE INDEX IF NOT EXISTS idx_tasks_request_id ON tasks(request_id);
CREATE INDEX IF NOT EXISTS idx_tasks_trace_id ON tasks(trace_id);
CREATE INDEX IF NOT EXISTS idx_tasks_session_id ON tasks(session_id);
CREATE INDEX IF NOT EXISTS idx_tasks_transfer_id ON tasks(transfer_id);

-- 任务事件流水，支持审计、运维查询和重放分析
CREATE TABLE IF NOT EXISTS task_events (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  from_status TEXT NOT NULL DEFAULT '',
  to_status TEXT NOT NULL,
  result_code TEXT NOT NULL DEFAULT '',
  message TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  FOREIGN KEY(task_id) REFERENCES tasks(id)
);

CREATE INDEX IF NOT EXISTS idx_task_events_task_id_created_at ON task_events(task_id, created_at);
