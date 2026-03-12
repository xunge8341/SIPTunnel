package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
)

type StructuredLogger struct {
	base *slog.Logger
}

func NewStructuredLogger(out io.Writer) *StructuredLogger {
	if out == nil {
		out = os.Stdout
	}
	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &StructuredLogger{base: slog.New(handler)}
}

func (l *StructuredLogger) Info(ctx context.Context, msg string, fields CoreFields, attrs ...any) {
	args := append(fields.SlogAttrs(), attrs...)
	l.base.InfoContext(ctx, msg, args...)
}

func (l *StructuredLogger) Error(ctx context.Context, msg string, fields CoreFields, attrs ...any) {
	args := append(fields.SlogAttrs(), attrs...)
	l.base.ErrorContext(ctx, msg, args...)
}
