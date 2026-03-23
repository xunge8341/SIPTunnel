package observability

import (
	"context"
)

type Telemetry struct {
	Logger *StructuredLogger
	Audit  *AuditLogger
}

func NewTelemetry() *Telemetry {
	logger := NewStructuredLogger(nil)
	store := NewInMemoryAuditStore()
	return &Telemetry{Logger: logger, Audit: NewAuditLogger(logger, store)}
}

func (t *Telemetry) AuditEvent(ctx context.Context, event AuditEvent) error {
	return t.Audit.Record(ctx, event)
}
