package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"siptunnel/internal/observability"
	sqliterepo "siptunnel/internal/repository/sqlite"
)

type RetentionPolicy struct {
	MaxTaskRecords       int
	MaxTaskAgeDays       int
	MaxAccessLogRecords  int
	MaxAccessLogAgeDays  int
	MaxAuditRecords      int
	MaxAuditAgeDays      int
	MaxDiagnosticRecords int
	MaxDiagnosticAgeDays int
}

type AccessLogRecord struct {
	ID            string
	OccurredAt    time.Time
	MappingName   string
	SourceIP      string
	Method        string
	Path          string
	StatusCode    int
	DurationMS    int64
	FailureReason string
	RequestID     string
	TraceID       string
}

type AccessLogQuery struct {
	Status     int
	Mapping    string
	SourceIP   string
	Method     string
	SlowOnly   bool
	FailedOnly bool
	StartTime  time.Time
	EndTime    time.Time
}

type SQLiteStore struct {
	db        *sql.DB
	tasks     *sqliterepo.TaskRepository
	retention RetentionPolicy
}

func (s *SQLiteStore) UpdateRetention(retention RetentionPolicy) {
	s.retention = retention
}

func OpenSQLiteStore(path string, retention RetentionPolicy) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA busy_timeout=5000; PRAGMA temp_store=MEMORY;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure sqlite pragmas: %w", err)
	}
	if err := applySchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLiteStore{db: db, tasks: sqliterepo.NewTaskRepository(db), retention: retention}, nil
}

func applySchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, datetime('now'));`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			task_type TEXT NOT NULL,
			request_id TEXT,
			trace_id TEXT,
			session_id TEXT,
			transfer_id TEXT,
			api_code TEXT,
			source_system TEXT,
			status TEXT NOT NULL,
			result_code TEXT,
			attempt INTEGER NOT NULL DEFAULT 0,
			max_attempts INTEGER NOT NULL DEFAULT 3,
			last_error TEXT,
			next_retry_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			completed_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS task_events (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			from_status TEXT,
			to_status TEXT,
			result_code TEXT,
			message TEXT,
			created_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			who TEXT,
			when_at DATETIME NOT NULL,
			request_type TEXT,
			validation_passed INTEGER,
			local_service_route TEXT,
			final_result TEXT,
			ops_action TEXT,
			core_json TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_audit_when ON audit_events(when_at DESC);`,
		`CREATE TABLE IF NOT EXISTS system_configs (
			config_key TEXT PRIMARY KEY,
			payload_json TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS diagnostic_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT,
			node_id TEXT,
			file_name TEXT,
			payload_json TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS access_logs (
			id TEXT PRIMARY KEY,
			occurred_at DATETIME NOT NULL,
			mapping_name TEXT NOT NULL,
			source_ip TEXT NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			status_code INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			failure_reason TEXT,
			request_id TEXT,
			trace_id TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_occurred_at ON access_logs(occurred_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_mapping ON access_logs(mapping_name, occurred_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_access_logs_source ON access_logs(source_ip, occurred_at DESC);`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("apply sqlite schema: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) Close() error                               { return s.db.Close() }
func (s *SQLiteStore) TaskRepository() *sqliterepo.TaskRepository { return s.tasks }

func (s *SQLiteStore) Record(ctx context.Context, event observability.AuditEvent) error {
	if event.When.IsZero() {
		event.When = time.Now().UTC()
	}
	coreJSON, err := json.Marshal(event.Core)
	if err != nil {
		return fmt.Errorf("marshal audit core: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO audit_events(who, when_at, request_type, validation_passed, local_service_route, final_result, ops_action, core_json) VALUES(?,?,?,?,?,?,?,?)`,
		event.Who, event.When.UTC(), event.RequestType, boolToInt(event.ValidationPassed), event.LocalServiceRoute, event.FinalResult, event.OpsAction, string(coreJSON))
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *SQLiteStore) List(ctx context.Context, query observability.AuditQuery) ([]observability.AuditEvent, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT who, when_at, request_type, validation_passed, local_service_route, final_result, ops_action, core_json FROM audit_events ORDER BY when_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query audit events: %w", err)
	}
	defer rows.Close()
	out := make([]observability.AuditEvent, 0, limit)
	for rows.Next() {
		var e observability.AuditEvent
		var validation int
		var coreJSON string
		if err := rows.Scan(&e.Who, &e.When, &e.RequestType, &validation, &e.LocalServiceRoute, &e.FinalResult, &e.OpsAction, &coreJSON); err != nil {
			return nil, fmt.Errorf("scan audit event: %w", err)
		}
		e.ValidationPassed = validation == 1
		_ = json.Unmarshal([]byte(coreJSON), &e.Core)
		if query.RequestID != "" && e.Core.RequestID != query.RequestID {
			continue
		}
		if query.TraceID != "" && e.Core.TraceID != query.TraceID {
			continue
		}
		if query.APICode != "" && e.Core.APICode != query.APICode {
			continue
		}
		if query.Who != "" && e.Who != query.Who {
			continue
		}
		if !query.StartTime.IsZero() && e.When.Before(query.StartTime) {
			continue
		}
		if !query.EndTime.IsZero() && e.When.After(query.EndTime) {
			continue
		}
		if query.ErrorOnly && e.FinalResult == "OK" {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (s *SQLiteStore) SaveSystemConfig(ctx context.Context, key string, payload any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal config payload: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO system_configs(config_key, payload_json, updated_at) VALUES(?,?,?) ON CONFLICT(config_key) DO UPDATE SET payload_json=excluded.payload_json, updated_at=excluded.updated_at`, key, string(buf), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save system config %s: %w", key, err)
	}
	return nil
}

func (s *SQLiteStore) RecordAccessLogBatch(ctx context.Context, entries []AccessLogRecord) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin access log batch tx: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO access_logs(id, occurred_at, mapping_name, source_ip, method, path, status_code, duration_ms, failure_reason, request_id, trace_id) VALUES(?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare access log batch insert: %w", err)
	}
	defer stmt.Close()
	for _, entry := range entries {
		if entry.OccurredAt.IsZero() {
			entry.OccurredAt = time.Now().UTC()
		}
		if _, err := stmt.ExecContext(ctx, entry.ID, entry.OccurredAt.UTC(), entry.MappingName, entry.SourceIP, entry.Method, entry.Path, entry.StatusCode, entry.DurationMS, entry.FailureReason, entry.RequestID, entry.TraceID); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("insert access log batch item: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit access log batch tx: %w", err)
	}
	return nil
}

func (s *SQLiteStore) RecordAccessLog(ctx context.Context, entry AccessLogRecord) error {
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO access_logs(id, occurred_at, mapping_name, source_ip, method, path, status_code, duration_ms, failure_reason, request_id, trace_id) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		entry.ID, entry.OccurredAt.UTC(), entry.MappingName, entry.SourceIP, entry.Method, entry.Path, entry.StatusCode, entry.DurationMS, entry.FailureReason, entry.RequestID, entry.TraceID)
	if err != nil {
		return fmt.Errorf("insert access log: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListAccessLogs(ctx context.Context, query AccessLogQuery) ([]AccessLogRecord, error) {
	where := make([]string, 0, 8)
	args := make([]any, 0, 8)
	if query.Status > 0 {
		where = append(where, "status_code = ?")
		args = append(args, query.Status)
	}
	if v := strings.TrimSpace(query.Mapping); v != "" {
		where = append(where, "LOWER(mapping_name) LIKE ?")
		args = append(args, "%"+strings.ToLower(v)+"%")
	}
	if v := strings.TrimSpace(query.SourceIP); v != "" {
		where = append(where, "LOWER(source_ip) LIKE ?")
		args = append(args, "%"+strings.ToLower(v)+"%")
	}
	if v := strings.TrimSpace(query.Method); v != "" {
		where = append(where, "UPPER(method) = ?")
		args = append(args, strings.ToUpper(v))
	}
	if query.SlowOnly {
		where = append(where, "duration_ms >= 500")
	}
	if query.FailedOnly {
		where = append(where, "(status_code >= 400 OR COALESCE(TRIM(failure_reason), '') <> '')")
	}
	if !query.StartTime.IsZero() {
		where = append(where, "occurred_at >= ?")
		args = append(args, query.StartTime.UTC())
	}
	if !query.EndTime.IsZero() {
		where = append(where, "occurred_at <= ?")
		args = append(args, query.EndTime.UTC())
	}
	stmt := `SELECT id, occurred_at, mapping_name, source_ip, method, path, status_code, duration_ms, failure_reason, request_id, trace_id FROM access_logs`
	if len(where) > 0 {
		stmt += " WHERE " + strings.Join(where, " AND ")
	}
	stmt += " ORDER BY occurred_at DESC"
	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("query access logs: %w", err)
	}
	defer rows.Close()
	out := make([]AccessLogRecord, 0, 128)
	for rows.Next() {
		var item AccessLogRecord
		if err := rows.Scan(&item.ID, &item.OccurredAt, &item.MappingName, &item.SourceIP, &item.Method, &item.Path, &item.StatusCode, &item.DurationMS, &item.FailureReason, &item.RequestID, &item.TraceID); err != nil {
			return nil, fmt.Errorf("scan access log: %w", err)
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *SQLiteStore) SaveDiagnosticRecord(ctx context.Context, jobID, nodeID, fileName string, payload any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal diagnostic payload: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO diagnostic_records(job_id, node_id, file_name, payload_json, created_at) VALUES(?,?,?,?,?)`, jobID, nodeID, fileName, string(buf), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("save diagnostic record: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Cleanup(ctx context.Context) error {
	_, err := s.CleanupWithStats(ctx)
	return err
}

func (s *SQLiteStore) CleanupWithStats(ctx context.Context) (int, error) {
	totalRemoved := 0
	if s.retention.MaxTaskAgeDays > 0 {
		removed, _ := execRowsAffected(ctx, s.db, `DELETE FROM tasks WHERE created_at < datetime('now', '-' || ? || ' day')`, s.retention.MaxTaskAgeDays)
		totalRemoved += removed
	}
	if s.retention.MaxTaskRecords > 0 {
		removed, _ := execRowsAffected(ctx, s.db, `DELETE FROM tasks WHERE id IN (SELECT id FROM tasks ORDER BY created_at DESC LIMIT -1 OFFSET ?)`, s.retention.MaxTaskRecords)
		totalRemoved += removed
	}
	if s.retention.MaxAccessLogAgeDays > 0 {
		removed, _ := execRowsAffected(ctx, s.db, `DELETE FROM access_logs WHERE occurred_at < datetime('now', '-' || ? || ' day')`, s.retention.MaxAccessLogAgeDays)
		totalRemoved += removed
	}
	if s.retention.MaxAccessLogRecords > 0 {
		removed, _ := execRowsAffected(ctx, s.db, `DELETE FROM access_logs WHERE id IN (SELECT id FROM access_logs ORDER BY occurred_at DESC LIMIT -1 OFFSET ?)`, s.retention.MaxAccessLogRecords)
		totalRemoved += removed
	}
	if s.retention.MaxAuditAgeDays > 0 {
		removed, _ := execRowsAffected(ctx, s.db, `DELETE FROM audit_events WHERE when_at < datetime('now', '-' || ? || ' day')`, s.retention.MaxAuditAgeDays)
		totalRemoved += removed
	}
	if s.retention.MaxAuditRecords > 0 {
		removed, _ := execRowsAffected(ctx, s.db, `DELETE FROM audit_events WHERE id IN (SELECT id FROM audit_events ORDER BY when_at DESC LIMIT -1 OFFSET ?)`, s.retention.MaxAuditRecords)
		totalRemoved += removed
	}
	if s.retention.MaxDiagnosticAgeDays > 0 {
		removed, _ := execRowsAffected(ctx, s.db, `DELETE FROM diagnostic_records WHERE created_at < datetime('now', '-' || ? || ' day')`, s.retention.MaxDiagnosticAgeDays)
		totalRemoved += removed
	}
	if s.retention.MaxDiagnosticRecords > 0 {
		removed, _ := execRowsAffected(ctx, s.db, `DELETE FROM diagnostic_records WHERE id IN (SELECT id FROM diagnostic_records ORDER BY created_at DESC LIMIT -1 OFFSET ?)`, s.retention.MaxDiagnosticRecords)
		totalRemoved += removed
	}
	_, _ = s.db.ExecContext(ctx, `PRAGMA optimize`)
	return totalRemoved, nil
}

func execRowsAffected(ctx context.Context, db *sql.DB, stmt string, args ...any) (int, error) {
	result, err := db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}
