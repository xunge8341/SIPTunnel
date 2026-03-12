package observability

import (
	"context"
	"log/slog"
)

type Telemetry struct {
	Logger *slog.Logger
}

func NewTelemetry() *Telemetry {
	return &Telemetry{Logger: slog.Default()}
}

func (t *Telemetry) Audit(ctx context.Context, action string, attrs ...any) {
	_ = ctx
	t.Logger.Info("audit", append([]any{"action", action}, attrs...)...)
}
