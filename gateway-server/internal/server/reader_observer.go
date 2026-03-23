package server

import (
	"io"
	"sync"
	"time"
)

type observedReadMetrics struct {
	Bytes        int64
	Reads        int64
	BlockedTotal time.Duration
	BlockedMax   time.Duration
	Source       string
	StartedAt    time.Time
	CompletedAt  time.Time
}

type observedReadCloser struct {
	reader  io.Reader
	closer  io.Closer
	source  string
	started time.Time
	mu      sync.Mutex
	metrics observedReadMetrics
}

func newObservedReadCloser(reader io.Reader, closer io.Closer, source string) *observedReadCloser {
	now := time.Now()
	return &observedReadCloser{
		reader:  reader,
		closer:  closer,
		source:  source,
		started: now,
		metrics: observedReadMetrics{Source: source, StartedAt: now},
	}
}

func (o *observedReadCloser) Read(p []byte) (int, error) {
	if o == nil || o.reader == nil {
		return 0, io.EOF
	}
	started := time.Now()
	n, err := o.reader.Read(p)
	blocked := time.Since(started)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.metrics.Reads++
	o.metrics.Bytes += int64(n)
	o.metrics.BlockedTotal += blocked
	if blocked > o.metrics.BlockedMax {
		o.metrics.BlockedMax = blocked
	}
	if err == io.EOF || err != nil {
		o.metrics.CompletedAt = time.Now()
	}
	return n, err
}

func (o *observedReadCloser) Close() error {
	if o == nil || o.closer == nil {
		return nil
	}
	err := o.closer.Close()
	o.mu.Lock()
	if o.metrics.CompletedAt.IsZero() {
		o.metrics.CompletedAt = time.Now()
	}
	o.mu.Unlock()
	return err
}

func (o *observedReadCloser) Snapshot() observedReadMetrics {
	if o == nil {
		return observedReadMetrics{}
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	metrics := o.metrics
	if metrics.CompletedAt.IsZero() {
		metrics.CompletedAt = time.Now()
	}
	return metrics
}
