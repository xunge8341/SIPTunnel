package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/observability"
)

type securityEventRecord struct {
	When        string `json:"when"`
	Category    string `json:"category"`
	Transport   string `json:"transport"`
	RequestID   string `json:"request_id"`
	TraceID     string `json:"trace_id"`
	SessionID   string `json:"session_id"`
	Reason      string `json:"reason"`
	AuditLinked bool   `json:"audit_linked"`
}

type securityEventStore struct {
	mu       sync.RWMutex
	items    []securityEventRecord
	maxItems int
	path     string
	audit    observability.AuditStore
}

func newSecurityEventStore(maxItems int, path string, audit observability.AuditStore) *securityEventStore {
	if maxItems <= 0 {
		maxItems = 512
	}
	return &securityEventStore{items: make([]securityEventRecord, 0, maxItems), maxItems: maxItems, path: path, audit: audit}
}

func (s *securityEventStore) Add(event securityEventRecord) {
	if strings.TrimSpace(event.When) == "" {
		event.When = formatTimestamp(time.Now().UTC())
	}
	event.AuditLinked = s.audit != nil
	s.mu.Lock()
	s.items = append([]securityEventRecord{event}, s.items...)
	if s.maxItems > 0 && len(s.items) > s.maxItems {
		s.items = s.items[:s.maxItems]
	}
	s.mu.Unlock()
	if s.path != "" {
		line, _ := json.Marshal(event)
		_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
		f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = f.Write(append(line, '\n'))
			_ = f.Close()
		}
	}
	if s.audit != nil {
		when := time.Now().UTC()
		if ts, err := parseTimestamp(event.When); err == nil {
			when = ts
		}
		_ = s.audit.Record(context.Background(), observability.AuditEvent{Who: "security", When: when, RequestType: "security.event", LocalServiceRoute: event.Transport, FinalResult: strings.ToUpper(strings.TrimSpace(event.Category)), OpsAction: "SECURITY_EVENT", ValidationPassed: false, Core: observability.CoreFields{RequestID: event.RequestID, TraceID: event.TraceID, SessionID: event.SessionID, APICode: event.Category, ResultCode: event.Reason}})
	}
}

func (s *securityEventStore) RecentSince(since time.Time) []securityEventRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]securityEventRecord, 0, len(s.items))
	for _, item := range s.items {
		if ts, err := parseTimestamp(item.When); err == nil && !ts.Before(since) {
			out = append(out, item)
		}
	}
	return out
}

func (s *securityEventStore) List(limit int) []securityEventRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.items) {
		limit = len(s.items)
	}
	out := make([]securityEventRecord, limit)
	copy(out, s.items[:limit])
	return out
}
