package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"siptunnel/internal/observability"
	sqliterepo "siptunnel/internal/repository/sqlite"
)

type RetentionPolicy struct {
	MaxTaskRecords       int
	MaxTaskAgeDays       int
	MaxAuditRecords      int
	MaxAuditAgeDays      int
	MaxDiagnosticRecords int
	MaxDiagnosticAgeDays int
}

type SQLiteStore struct {
	db        *sql.DB
	tasks     *sqliterepo.TaskRepository
	retention RetentionPolicy
}

func OpenSQLiteStore(path string, retention RetentionPolicy) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`); err != nil {
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
	if s.retention.MaxTaskAgeDays > 0 {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM tasks WHERE created_at < datetime('now', '-' || ? || ' day')`, s.retention.MaxTaskAgeDays)
	}
	if s.retention.MaxTaskRecords > 0 {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id IN (SELECT id FROM tasks ORDER BY created_at DESC LIMIT -1 OFFSET ?)`, s.retention.MaxTaskRecords)
	}
	if s.retention.MaxAuditAgeDays > 0 {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM audit_events WHERE when_at < datetime('now', '-' || ? || ' day')`, s.retention.MaxAuditAgeDays)
	}
	if s.retention.MaxAuditRecords > 0 {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM audit_events WHERE id IN (SELECT id FROM audit_events ORDER BY when_at DESC LIMIT -1 OFFSET ?)`, s.retention.MaxAuditRecords)
	}
	if s.retention.MaxDiagnosticAgeDays > 0 {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM diagnostic_records WHERE created_at < datetime('now', '-' || ? || ' day')`, s.retention.MaxDiagnosticAgeDays)
	}
	if s.retention.MaxDiagnosticRecords > 0 {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM diagnostic_records WHERE id IN (SELECT id FROM diagnostic_records ORDER BY created_at DESC LIMIT -1 OFFSET ?)`, s.retention.MaxDiagnosticRecords)
	}
	_, _ = s.db.ExecContext(ctx, `VACUUM`)
	return nil
}
