package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
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

func NewStructuredLoggerWithFile(logDir string) (*StructuredLogger, io.Closer, error) {
	if err := os.MkdirAll(filepath.Clean(logDir), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}
	path := filepath.Join(logDir, "gateway.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	return NewStructuredLogger(io.MultiWriter(os.Stdout, f)), f, nil
}

func (l *StructuredLogger) Info(ctx context.Context, msg string, fields CoreFields, attrs ...any) {
	args := append(fields.SlogAttrs(), attrs...)
	l.base.InfoContext(ctx, msg, args...)
}

func (l *StructuredLogger) Error(ctx context.Context, msg string, fields CoreFields, attrs ...any) {
	args := append(fields.SlogAttrs(), attrs...)
	l.base.ErrorContext(ctx, msg, args...)
}
