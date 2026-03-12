package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type FileBackedAuditStore struct {
	base AuditStore
	mu   sync.Mutex
	file *os.File
}

func NewFileBackedAuditStore(auditDir string) (*FileBackedAuditStore, error) {
	if err := os.MkdirAll(filepath.Clean(auditDir), 0o755); err != nil {
		return nil, fmt.Errorf("create audit directory: %w", err)
	}
	path := filepath.Join(auditDir, "audit-events.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open audit file %q: %w", path, err)
	}
	return &FileBackedAuditStore{base: NewInMemoryAuditStore(), file: f}, nil
}

func (s *FileBackedAuditStore) Record(ctx context.Context, event AuditEvent) error {
	if err := s.base.Record(ctx, event); err != nil {
		return err
	}
	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func (s *FileBackedAuditStore) List(ctx context.Context, query AuditQuery) ([]AuditEvent, error) {
	return s.base.List(ctx, query)
}

func (s *FileBackedAuditStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}
