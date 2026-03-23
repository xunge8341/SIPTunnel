package server

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type mappingNodeLatencyKey struct{}

type mappingNodeLatencyTracker struct {
	startedAt time.Time

	requestID   string
	traceID     string
	mappingID   string
	mappingName string
	targetURL   string
	method      string

	preparedAt         time.Time
	responseReadyAt    time.Time
	strategySelectedAt time.Time
	strategy           atomic.Value
	strategyReason     atomic.Value
	bytesWritten       atomic.Int64
	firstByteAtUnix    atomic.Int64
	sourceReadNanos    atomic.Int64
	sourceReadCalls    atomic.Int64
	writerBlockNanos   atomic.Int64
	writeCalls         atomic.Int64
	maxWriteNanos      atomic.Int64
	finalizeOnce       sync.Once
}

func newMappingNodeLatencyTracker(requestID, traceID, mappingID, mappingName, targetURL, method string) *mappingNodeLatencyTracker {
	t := &mappingNodeLatencyTracker{
		startedAt:   time.Now().UTC(),
		requestID:   strings.TrimSpace(requestID),
		traceID:     strings.TrimSpace(traceID),
		mappingID:   strings.TrimSpace(mappingID),
		mappingName: strings.TrimSpace(mappingName),
		targetURL:   strings.TrimSpace(targetURL),
		method:      strings.TrimSpace(method),
	}
	t.strategy.Store("-")
	t.strategyReason.Store("-")
	return t
}

func withMappingNodeLatencyTracker(ctx context.Context, tracker *mappingNodeLatencyTracker) context.Context {
	if ctx == nil || tracker == nil {
		return ctx
	}
	return context.WithValue(ctx, mappingNodeLatencyKey{}, tracker)
}

func nodeLatencyFromContext(ctx context.Context) *mappingNodeLatencyTracker {
	if ctx == nil {
		return nil
	}
	tracker, _ := ctx.Value(mappingNodeLatencyKey{}).(*mappingNodeLatencyTracker)
	return tracker
}

func (t *mappingNodeLatencyTracker) MarkPrepared() {
	if t != nil && t.preparedAt.IsZero() {
		t.preparedAt = time.Now().UTC()
	}
}

func (t *mappingNodeLatencyTracker) MarkResponseReady() {
	if t != nil && t.responseReadyAt.IsZero() {
		t.responseReadyAt = time.Now().UTC()
	}
}

func (t *mappingNodeLatencyTracker) MarkStrategy(strategy, reason string) {
	if t == nil {
		return
	}
	if t.strategySelectedAt.IsZero() {
		t.strategySelectedAt = time.Now().UTC()
	}
	if v := strings.TrimSpace(strategy); v != "" {
		t.strategy.Store(v)
	}
	if v := strings.TrimSpace(reason); v != "" {
		t.strategyReason.Store(v)
	}
}

func (t *mappingNodeLatencyTracker) MarkFirstByte() {
	if t == nil {
		return
	}
	t.firstByteAtUnix.CompareAndSwap(0, time.Now().UTC().UnixNano())
}

func (t *mappingNodeLatencyTracker) AddBytes(n int) {
	if t == nil || n <= 0 {
		return
	}
	t.bytesWritten.Add(int64(n))
}

func (t *mappingNodeLatencyTracker) AddSourceRead(d time.Duration) {
	if t == nil || d <= 0 {
		return
	}
	t.sourceReadNanos.Add(d.Nanoseconds())
	t.sourceReadCalls.Add(1)
}

func (t *mappingNodeLatencyTracker) AddWriterBlock(d time.Duration) {
	if t == nil || d <= 0 {
		return
	}
	n := d.Nanoseconds()
	t.writerBlockNanos.Add(n)
	t.writeCalls.Add(1)
	for {
		cur := t.maxWriteNanos.Load()
		if n <= cur || t.maxWriteNanos.CompareAndSwap(cur, n) {
			break
		}
	}
}

func (t *mappingNodeLatencyTracker) Finalize(req *http.Request, prepared *mappingForwardRequest, resp *http.Response, copyErr error) {
	if t == nil {
		return
	}
	if copyErr != nil {
		observeRuntimeCopyError(copyErr)
	}
	t.finalizeOnce.Do(func() {
		now := time.Now().UTC()
		prepareMS := durationSinceOrZero(t.startedAt, t.preparedAt)
		responseReadyMS := durationSinceOrZero(t.startedAt, t.responseReadyAt)
		strategySelectMS := durationSinceOrZero(t.startedAt, t.strategySelectedAt)
		firstByteMS := int64(-1)
		bodyActiveMS := int64(0)
		if first := t.firstByteAtUnix.Load(); first > 0 {
			firstAt := time.Unix(0, first).UTC()
			firstByteMS = firstAt.Sub(t.startedAt).Milliseconds()
			bodyActiveMS = now.Sub(firstAt).Milliseconds()
		}
		bytes := t.bytesWritten.Load()
		strategy := t.stringValue(t.strategy)
		reason := t.stringValue(t.strategyReason)
		sourceReadMS := time.Duration(t.sourceReadNanos.Load()).Milliseconds()
		sourceReadCalls := t.sourceReadCalls.Load()
		writerBlockMS := time.Duration(t.writerBlockNanos.Load()).Milliseconds()
		writeCalls := t.writeCalls.Load()
		maxWriteMS := time.Duration(t.maxWriteNanos.Load()).Milliseconds()
		probeLike := isProbeLikePlaybackAbort(req, prepared, resp, bytes, time.Since(t.startedAt), copyErr)
		if probeLike {
			globalAdaptiveDelivery.observeProbeAbort(prepared, resp)
			log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=probe_abort_detected target_url=%s method=%s bytes=%d total_ms=%d reason=%v", t.requestID, t.traceID, t.mappingID, t.mappingName, t.targetURL, t.method, bytes, time.Since(t.startedAt).Milliseconds(), copyErr)
		}
		log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=node_latency_summary target_url=%s method=%s prepare_ms=%d response_ready_ms=%d strategy_select_ms=%d first_byte_ms=%d body_active_ms=%d source_read_ms=%d source_read_calls=%d writer_block_ms=%d write_calls=%d max_write_ms=%d total_ms=%d bytes=%d strategy=%s reason=%s probe_like=%t range_playback=%t", t.requestID, t.traceID, t.mappingID, t.mappingName, t.targetURL, t.method, prepareMS, responseReadyMS, strategySelectMS, firstByteMS, bodyActiveMS, sourceReadMS, sourceReadCalls, writerBlockMS, writeCalls, maxWriteMS, time.Since(t.startedAt).Milliseconds(), bytes, firstNonEmpty(strategy, "-"), firstNonEmpty(reason, "-"), probeLike, isRangePlaybackRequest(prepared, req, resp))
	})
}

func (t *mappingNodeLatencyTracker) stringValue(v atomic.Value) string {
	if raw := v.Load(); raw != nil {
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func durationSinceOrZero(start, end time.Time) int64 {
	if start.IsZero() || end.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func isProbeLikePlaybackAbort(req *http.Request, prepared *mappingForwardRequest, resp *http.Response, bytes int64, elapsed time.Duration, copyErr error) bool {
	if req == nil || prepared == nil || resp == nil || copyErr == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(req.Method), http.MethodGet) {
		return false
	}
	if strings.TrimSpace(preparedOrRequestRangeHeader(prepared, req)) != "" {
		return false
	}
	if bytes <= 0 || bytes > 128*1024 {
		return false
	}
	if elapsed > 1500*time.Millisecond {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if !strings.Contains(ct, "video/") && !strings.Contains(ct, "audio/") && strings.TrimSpace(resp.Header.Get("Accept-Ranges")) == "" {
		return false
	}
	errText := strings.ToLower(strings.TrimSpace(copyErr.Error()))
	return strings.Contains(errText, "context canceled") || strings.Contains(errText, "cancel")
}

type latencyTrackingReadCloser struct {
	io.ReadCloser
	tracker *mappingNodeLatencyTracker
}

func (r *latencyTrackingReadCloser) Read(p []byte) (int, error) {
	if r == nil || r.ReadCloser == nil {
		return 0, io.EOF
	}
	started := time.Now()
	n, err := r.ReadCloser.Read(p)
	if r.tracker != nil {
		r.tracker.AddSourceRead(time.Since(started))
	}
	return n, err
}

type latencyTrackingWriter struct {
	io.Writer
	tracker *mappingNodeLatencyTracker
}

func (w *latencyTrackingWriter) Write(p []byte) (int, error) {
	if w == nil || w.Writer == nil {
		return 0, io.ErrClosedPipe
	}
	if len(p) > 0 && w.tracker != nil {
		w.tracker.MarkFirstByte()
	}
	started := time.Now()
	n, err := w.Writer.Write(p)
	if w.tracker != nil {
		blockDur := time.Since(started)
		w.tracker.AddWriterBlock(blockDur)
		observeRuntimeWriterBlock(blockDur)
		if n > 0 {
			w.tracker.AddBytes(n)
		}
	}
	return n, err
}
