package server

import (
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	healthzResponseBytes     = []byte("{\"code\":\"OK\",\"message\":\"success\",\"data\":{\"status\":\"ok\",\"mode\":\"live\"}}\n")
	requestDiscardBufferPool = sync.Pool{New: func() any {
		buf := make([]byte, 8*1024)
		return &buf
	}}
	readyzLogThrottleNanos int64
)

func discardRequestBody(r *http.Request, limit int64) {
	if r == nil || r.Body == nil || r.Body == http.NoBody {
		return
	}
	defer func() {
		_ = r.Body.Close()
		r.Body = http.NoBody
	}()
	if limit <= 0 {
		limit = 64 * 1024
	}
	holder := requestDiscardBufferPool.Get().(*[]byte)
	defer requestDiscardBufferPool.Put(holder)
	_, _ = io.CopyBuffer(io.Discard, io.LimitReader(r.Body, limit), *holder)
}

func writeHealthzFastJSON(w http.ResponseWriter) {
	if w == nil {
		return
	}
	header := w.Header()
	header.Set("Content-Type", "application/json")
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Length", strconv.Itoa(len(healthzResponseBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(healthzResponseBytes)
}

func shouldLogReadyzSuccess(now time.Time) bool {
	if now.IsZero() {
		now = time.Now()
	}
	const window = int64((30 * time.Second) / time.Nanosecond)
	current := now.UnixNano()
	last := atomic.LoadInt64(&readyzLogThrottleNanos)
	if current-last < window {
		return false
	}
	return atomic.CompareAndSwapInt64(&readyzLogThrottleNanos, last, current)
}
