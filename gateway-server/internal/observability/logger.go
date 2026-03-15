package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type StructuredLogger struct {
	base *slog.Logger
}

type LogRetentionPolicy struct {
	MaxSizeMB  int
	MaxFiles   int
	MaxAgeDays int
}

func NewStructuredLogger(out io.Writer) *StructuredLogger {
	if out == nil {
		out = os.Stdout
	}
	handler := slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &StructuredLogger{base: slog.New(handler)}
}

func NewStructuredLoggerWithFile(logDir string, policy LogRetentionPolicy) (*StructuredLogger, io.Closer, error) {
	if err := os.MkdirAll(filepath.Clean(logDir), 0o755); err != nil {
		return nil, nil, fmt.Errorf("create log directory: %w", err)
	}
	if policy.MaxSizeMB <= 0 {
		policy.MaxSizeMB = 20
	}
	if policy.MaxFiles <= 0 {
		policy.MaxFiles = 5
	}
	if policy.MaxAgeDays <= 0 {
		policy.MaxAgeDays = 7
	}
	rotator, err := newLogRotator(logDir, policy)
	if err != nil {
		return nil, nil, err
	}
	return NewStructuredLogger(io.MultiWriter(os.Stdout, rotator)), rotator, nil
}

func (l *StructuredLogger) Info(ctx context.Context, msg string, fields CoreFields, attrs ...any) {
	args := append(fields.SlogAttrs(), attrs...)
	l.base.InfoContext(ctx, msg, args...)
}

func (l *StructuredLogger) Error(ctx context.Context, msg string, fields CoreFields, attrs ...any) {
	args := append(fields.SlogAttrs(), attrs...)
	l.base.ErrorContext(ctx, msg, args...)
}

type logRotator struct {
	dir      string
	path     string
	file     *os.File
	maxBytes int64
	maxFiles int
	maxAge   time.Duration
}

func newLogRotator(logDir string, policy LogRetentionPolicy) (*logRotator, error) {
	r := &logRotator{dir: filepath.Clean(logDir), path: filepath.Join(logDir, "gateway.log"), maxBytes: int64(policy.MaxSizeMB) * 1024 * 1024, maxFiles: policy.MaxFiles, maxAge: time.Duration(policy.MaxAgeDays) * 24 * time.Hour}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", r.path, err)
	}
	r.file = f
	return r, nil
}

func (r *logRotator) Write(p []byte) (int, error) {
	if err := r.rotateIfNeeded(len(p)); err != nil {
		return 0, err
	}
	return r.file.Write(p)
}

func (r *logRotator) rotateIfNeeded(incoming int) error {
	st, err := r.file.Stat()
	if err != nil {
		return err
	}
	if st.Size()+int64(incoming) < r.maxBytes {
		return nil
	}
	_ = r.file.Close()
	archived := filepath.Join(r.dir, fmt.Sprintf("gateway-%s.log", time.Now().UTC().Format("20060102T150405Z")))
	_ = os.Rename(r.path, archived)
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	r.file = f
	r.cleanupArchives()
	return nil
}

func (r *logRotator) cleanupArchives() {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	type fi struct {
		name string
		mod  time.Time
	}
	logs := make([]fi, 0)
	now := time.Now()
	for _, e := range entries {
		if e.IsDir() || len(e.Name()) < 11 || e.Name()[:7] != "gateway" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > r.maxAge {
			_ = os.Remove(filepath.Join(r.dir, e.Name()))
			continue
		}
		if e.Name() != "gateway.log" {
			logs = append(logs, fi{name: e.Name(), mod: info.ModTime()})
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].mod.After(logs[j].mod) })
	for i := r.maxFiles; i < len(logs); i++ {
		_ = os.Remove(filepath.Join(r.dir, logs[i].name))
	}
}

func (r *logRotator) Close() error {
	if r.file == nil {
		return nil
	}
	return r.file.Close()
}
