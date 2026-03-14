package observability

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type AuditEvent struct {
	Who               string     `json:"who"`
	When              time.Time  `json:"when"`
	RequestType       string     `json:"request_type"`
	ValidationPassed  bool       `json:"validation_passed"`
	LocalServiceRoute string     `json:"local_service_route"`
	FinalResult       string     `json:"final_result"`
	OpsAction         string     `json:"ops_action"`
	Core              CoreFields `json:"core"`
}

type AuditQuery struct {
	RequestID string
	TraceID   string
	APICode   string
	Who       string
	Rule      string
	ErrorOnly bool
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

type AuditQueryService interface {
	List(ctx context.Context, query AuditQuery) ([]AuditEvent, error)
}

type AuditRecorder interface {
	Record(ctx context.Context, event AuditEvent) error
}

type AuditStore interface {
	AuditRecorder
	AuditQueryService
}

type InMemoryAuditStore struct {
	mu     sync.RWMutex
	events []AuditEvent
}

func NewInMemoryAuditStore() *InMemoryAuditStore {
	return &InMemoryAuditStore{events: make([]AuditEvent, 0, 16)}
}

func (s *InMemoryAuditStore) Record(_ context.Context, event AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.When.IsZero() {
		event.When = time.Now().UTC()
	}
	s.events = append(s.events, event)
	return nil
}

func (s *InMemoryAuditStore) List(_ context.Context, query AuditQuery) ([]AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}

	out := make([]AuditEvent, 0, limit)
	for i := len(s.events) - 1; i >= 0 && len(out) < limit; i-- {
		e := s.events[i]
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
		if query.Rule != "" && !matchesAuditRule(e, query.Rule) {
			continue
		}
		if query.ErrorOnly && !isAuditError(e) {
			continue
		}
		out = append(out, e)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].When.After(out[j].When)
	})
	return out, nil
}

func matchesAuditRule(event AuditEvent, rule string) bool {
	rule = strings.TrimSpace(strings.ToLower(rule))
	if rule == "" {
		return true
	}
	for _, field := range []string{event.Core.APICode, event.OpsAction, event.RequestType, event.LocalServiceRoute} {
		if strings.Contains(strings.ToLower(field), rule) {
			return true
		}
	}
	return false
}

func isAuditError(event AuditEvent) bool {
	result := strings.ToUpper(strings.TrimSpace(event.FinalResult))
	return result != "" && result != "OK"
}

type AuditLogger struct {
	logger *StructuredLogger
	store  AuditStore
}

func NewAuditLogger(logger *StructuredLogger, store AuditStore) *AuditLogger {
	return &AuditLogger{logger: logger, store: store}
}

func (a *AuditLogger) Record(ctx context.Context, event AuditEvent) error {
	if err := a.store.Record(ctx, event); err != nil {
		return err
	}
	a.logger.Info(ctx, "audit_event", event.Core,
		"who", event.Who,
		"when", event.When.UTC().Format(time.RFC3339Nano),
		"request_type", event.RequestType,
		"validation_passed", event.ValidationPassed,
		"local_service_route", event.LocalServiceRoute,
		"final_result", event.FinalResult,
		"ops_action", event.OpsAction,
	)
	return nil
}
