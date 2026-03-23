package server

import (
	"container/list"
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type adaptiveDeliveryProfile string

const (
	adaptiveDeliveryStable   adaptiveDeliveryProfile = "stable"
	adaptiveDeliveryDegraded adaptiveDeliveryProfile = "degraded"
	adaptiveDeliveryHotspot  adaptiveDeliveryProfile = "hotspot"
)

type adaptiveDeliveryDecision struct {
	Profile                adaptiveDeliveryProfile
	PreferSegmentedPrimary bool
	SegmentConcurrency     int
	ConcurrencyReason      string
	WindowBytes            int64
	PrefetchSegments       int
	CacheEnabled           bool
	CacheTTL               time.Duration
	StabilityTier          string
}

type adaptivePathState struct {
	ConsecutiveStreamFailures int
	ConsecutiveSegmentErrors  int
	HotspotHits               int
	LastUpdated               time.Time
}

type adaptiveDeliveryController struct {
	mu          sync.Mutex
	paths       map[string]*adaptivePathState
	probeAborts map[string]time.Time
	cache       *adaptiveSegmentCache
}

var globalAdaptiveDelivery = newAdaptiveDeliveryController()

func newAdaptiveDeliveryController() *adaptiveDeliveryController {
	return &adaptiveDeliveryController{
		paths:       make(map[string]*adaptivePathState),
		probeAborts: make(map[string]time.Time),
		cache:       newAdaptiveSegmentCache(),
	}
}

func adaptivePathKey(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) string {
	if prepared == nil || prepared.TargetURL == nil {
		return "-"
	}
	builder := strings.Builder{}
	builder.WriteString(strings.ToUpper(strings.TrimSpace(firstNonEmpty(prepared.Method, http.MethodGet))))
	builder.WriteString("|")
	builder.WriteString(prepared.TargetURL.String())
	if resp != nil {
		if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
			builder.WriteString("|etag=")
			builder.WriteString(etag)
		} else if lastModified := strings.TrimSpace(resp.Header.Get("Last-Modified")); lastModified != "" {
			builder.WriteString("|lm=")
			builder.WriteString(lastModified)
		}
	}
	if req != nil {
		if r := strings.TrimSpace(req.Header.Get("Range")); r != "" {
			builder.WriteString("|client_range")
		}
	}
	return builder.String()
}

func adaptiveResourceKey(prepared *mappingForwardRequest, resp *http.Response) string {
	if prepared == nil || prepared.TargetURL == nil {
		return "-"
	}
	builder := strings.Builder{}
	builder.WriteString(strings.ToUpper(strings.TrimSpace(firstNonEmpty(prepared.Method, http.MethodGet))))
	builder.WriteString("|")
	builder.WriteString(prepared.TargetURL.String())
	if resp != nil {
		if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
			builder.WriteString("|etag=")
			builder.WriteString(etag)
		} else if lastModified := strings.TrimSpace(resp.Header.Get("Last-Modified")); lastModified != "" {
			builder.WriteString("|lm=")
			builder.WriteString(lastModified)
		}
	}
	return builder.String()
}

func adaptiveResourceKeyWithoutResp(prepared *mappingForwardRequest) string {
	return adaptiveResourceKey(prepared, nil)
}

func isPlaybackHotspotCandidate(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) bool {
	if prepared == nil || resp == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(prepared.Method), http.MethodGet) {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.Contains(ct, "video/") || strings.Contains(ct, "audio/") {
		return true
	}
	return isRangePlaybackRequest(prepared, req, resp)
}

func genericDownloadConservativeWindowBytes() int64 {
	window := genericDownloadOpenEndedWindowBytes()
	if window < 2<<20 {
		window = 2 << 20
	}
	return window
}

func isGenericDownloadOpenEndedRange(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) bool {
	if !isGenericLargeDownloadCandidate(prepared, req, resp) {
		return false
	}
	return isOpenEndedSingleByteRange(preparedOrRequestRangeHeader(prepared, req))
}

func isGenericLargeDownloadCandidate(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) bool {
	if prepared == nil || resp == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(prepared.Method), http.MethodGet) {
		return false
	}
	if isRangePlaybackRequest(prepared, req, resp) {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.Contains(ct, "video/") || strings.Contains(ct, "audio/") {
		return false
	}
	if strings.Contains(ct, "application/octet-stream") || strings.Contains(ct, "application/pcap") || strings.Contains(ct, "application/pcapng") {
		return true
	}
	if cd := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Disposition"))); strings.Contains(cd, "attachment") {
		return true
	}
	return resp.ContentLength >= genericSegmentedPrimaryThresholdBytes()
}

func isLoopbackPlaybackOrigin(prepared *mappingForwardRequest) bool {
	if prepared == nil || prepared.TargetURL == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(prepared.TargetURL.Hostname()))
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func isGenericDownloadWholeObjectFetch(prepared *mappingForwardRequest, req *http.Request, resp *http.Response) bool {
	if prepared == nil || req == nil || resp == nil {
		return false
	}
	if !isGenericLargeDownloadCandidate(prepared, req, resp) {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(req.Method), http.MethodGet) {
		return false
	}
	if resp.StatusCode != http.StatusOK {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(prepared.Headers.Get(internalRangeFetchHeader)), "1") {
		return false
	}
	if strings.TrimSpace(req.Header.Get("Range")) != "" {
		return false
	}
	if strings.TrimSpace(prepared.Headers.Get("Range")) != "" {
		return false
	}
	return resp.ContentLength >= genericSegmentedPrimaryThresholdBytes()
}

func (c *adaptiveDeliveryController) snapshot(prepared *mappingForwardRequest, req *http.Request, resp *http.Response, plan fixedWindowPlan) adaptiveDeliveryDecision {
	baseConcurrency := maxIntVal(1, minIntVal(plan.concurrency, 2))
	decision := adaptiveDeliveryDecision{
		Profile:            adaptiveDeliveryStable,
		SegmentConcurrency: baseConcurrency,
		ConcurrencyReason:  "base_plan",
		WindowBytes:        plan.window,
		PrefetchSegments:   0,
		CacheEnabled:       false,
		CacheTTL:           adaptivePlaybackSegmentCacheTTL(),
		StabilityTier:      "balanced",
	}
	if isGenericLargeDownloadCandidate(prepared, req, resp) {
		// 非侵入式共性优化：generic 大响应默认优先连续流，只有在 stream/resume 明确失败后，
		// 才退回 fixed-window 兜底；避免下载链路和预览链路都被过早切进 segmented primary。
		decision.Profile = adaptiveDeliveryStable
		decision.CacheEnabled = false
		decision.WindowBytes = maxInt64Val(plan.window, genericDownloadConservativeWindowBytes())
		decision.SegmentConcurrency = 1
		decision.ConcurrencyReason = "generic_stream_recovery_window"
		decision.PrefetchSegments = 0
		decision.PreferSegmentedPrimary = false
		bulkWholeObject := isGenericDownloadWholeObjectFetch(prepared, req, resp)
		if isGenericDownloadOpenEndedRange(prepared, req, resp) || bulkWholeObject {
			decision.StabilityTier = "balanced"
			decision.WindowBytes = maxInt64Val(decision.WindowBytes, genericDownloadConservativeWindowBytes())
			decision.ConcurrencyReason = "generic_stream_first_bulk_window"
			if bulkWholeObject {
				decision.ConcurrencyReason = "generic_stream_first_whole_object_window"
			}
		}
		if genericDownloadSourceConstrainedAutoSingleflightEnabled() && globalGenericDownloadController.sourceConstrainedForTarget(prepared.TargetURL.String()) {
			decision.ConcurrencyReason = "generic_stream_first_source_observed"
		}
		if globalGenericDownloadController.breakerOpenForTarget(prepared.TargetURL.String()) {
			decision.StabilityTier = "conservative"
			decision.WindowBytes = minInt64Val(decision.WindowBytes, genericDownloadConservativeWindowBytes())
			decision.SegmentConcurrency = 1
			decision.ConcurrencyReason = "generic_stream_recovery_breaker_open"
		}
		return decision
	}
	if !isPlaybackHotspotCandidate(prepared, req, resp) {
		return decision
	}
	decision.CacheEnabled = adaptivePlaybackSegmentCacheBytes() > 0
	decision.WindowBytes = maxInt64Val(plan.window, adaptivePlaybackHotWindowBytes())
	decision.SegmentConcurrency = minIntVal(maxIntVal(baseConcurrency, 2), maxIntVal(1, udpSegmentParallelismPerDevice()))
	decision.ConcurrencyReason = "adaptive_playback_hot_window"
	if isLoopbackPlaybackOrigin(prepared) {
		// 本地源经代理再回本地观看时仍需抑制 fan-out，但 stable=1 会把体验做得过钝；
		// 这里改成受配置约束的低并发（默认 2），在性能优先和回环放大之间取平衡。
		decision.SegmentConcurrency = minIntVal(decision.SegmentConcurrency, maxIntVal(1, adaptiveLoopbackPlaybackSegmentConcurrency()))
		decision.ConcurrencyReason = "adaptive_loopback_cap"
	}

	key := adaptivePathKey(prepared, req, resp)
	c.mu.Lock()
	state := c.paths[key]
	if state != nil && time.Since(state.LastUpdated) > 2*time.Minute {
		delete(c.paths, key)
		state = nil
	}
	for probeKey, ts := range c.probeAborts {
		if time.Since(ts) > 15*time.Second {
			delete(c.probeAborts, probeKey)
		}
	}
	if state != nil {
		if state.HotspotHits >= 2 {
			decision.Profile = adaptiveDeliveryHotspot
			decision.SegmentConcurrency = minIntVal(maxIntVal(4, decision.SegmentConcurrency), maxIntVal(1, udpSegmentParallelismPerDevice()))
			decision.ConcurrencyReason = "hotspot_scale_out"
			if isLoopbackPlaybackOrigin(prepared) {
				decision.SegmentConcurrency = minIntVal(decision.SegmentConcurrency, maxIntVal(2, adaptiveLoopbackPlaybackSegmentConcurrency()+1))
				decision.ConcurrencyReason = "hotspot_loopback_cap"
			}
			if decision.CacheEnabled {
				decision.PrefetchSegments = minIntVal(adaptivePlaybackPrefetchSegments(), 1)
			}
		}
		if state.ConsecutiveStreamFailures >= adaptivePrimarySegmentAfterFailures() {
			decision.Profile = adaptiveDeliveryDegraded
			decision.PreferSegmentedPrimary = true
			decision.SegmentConcurrency = minIntVal(maxIntVal(2, decision.SegmentConcurrency), maxIntVal(1, udpSegmentParallelismPerDevice()))
			decision.ConcurrencyReason = "adaptive_stream_failure_recovery"
			decision.PrefetchSegments = 0
		}
		if state.ConsecutiveSegmentErrors >= 2 {
			decision.Profile = adaptiveDeliveryDegraded
			decision.SegmentConcurrency = minIntVal(maxIntVal(1, decision.SegmentConcurrency), 2)
			decision.ConcurrencyReason = "adaptive_segment_error_guard"
			decision.PrefetchSegments = 0
		}
	}
	c.mu.Unlock()
	return decision
}

func (c *adaptiveDeliveryController) observeStreamResult(prepared *mappingForwardRequest, req *http.Request, resp *http.Response, strategy largeResponseDeliveryStrategy, err error) {
	if prepared == nil || resp == nil || !isPlaybackHotspotCandidate(prepared, req, resp) {
		return
	}
	key := adaptivePathKey(prepared, req, resp)
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.paths[key]
	if state == nil {
		state = &adaptivePathState{}
		c.paths[key] = state
	}
	state.LastUpdated = time.Now()
	if strategy == deliveryStrategyStreamPrimary {
		if err != nil {
			state.ConsecutiveStreamFailures++
		} else if state.ConsecutiveStreamFailures > 0 {
			state.ConsecutiveStreamFailures--
		}
	}
	if strategy == deliveryStrategyAdaptiveSegmentedPrimary || strategy == deliveryStrategyFallbackSegmented {
		if err == nil && state.ConsecutiveStreamFailures > 0 {
			state.ConsecutiveStreamFailures--
		}
	}
}

func (c *adaptiveDeliveryController) observeSegmentResult(prepared *mappingForwardRequest, req *http.Request, resp *http.Response, cacheHit bool, err error) {
	if prepared == nil || resp == nil || !isPlaybackHotspotCandidate(prepared, req, resp) {
		return
	}
	key := adaptivePathKey(prepared, req, resp)
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.paths[key]
	if state == nil {
		state = &adaptivePathState{}
		c.paths[key] = state
	}
	state.LastUpdated = time.Now()
	if cacheHit {
		state.HotspotHits++
	}
	if err != nil {
		state.ConsecutiveSegmentErrors++
	} else if state.ConsecutiveSegmentErrors > 0 {
		state.ConsecutiveSegmentErrors--
	}
}

func (c *adaptiveDeliveryController) observeProbeAbort(prepared *mappingForwardRequest, resp *http.Response) {
	if prepared == nil || resp == nil {
		return
	}
	keys := []string{adaptiveResourceKey(prepared, resp), adaptiveResourceKeyWithoutResp(prepared)}
	c.mu.Lock()
	for _, key := range keys {
		if key != "-" {
			c.probeAborts[key] = time.Now()
		}
	}
	c.mu.Unlock()
}

func (c *adaptiveDeliveryController) recentProbeAbort(prepared *mappingForwardRequest, resp *http.Response) bool {
	if prepared == nil || resp == nil {
		return false
	}
	key := adaptiveResourceKey(prepared, resp)
	if key == "-" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ts, ok := c.probeAborts[key]
	if !ok {
		return false
	}
	if time.Since(ts) > 15*time.Second {
		delete(c.probeAborts, key)
		return false
	}
	return true
}

func (c *adaptiveDeliveryController) recentProbeAbortPrepared(prepared *mappingForwardRequest) bool {
	if prepared == nil {
		return false
	}
	key := adaptiveResourceKeyWithoutResp(prepared)
	if key == "-" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ts, ok := c.probeAborts[key]
	if !ok {
		return false
	}
	if time.Since(ts) > 15*time.Second {
		delete(c.probeAborts, key)
		return false
	}
	return true
}

type adaptiveSegmentCacheEntry struct {
	key        string
	data       []byte
	expiresAt  time.Time
	lastAccess time.Time
	element    *list.Element
}

type adaptiveSegmentCacheFlight struct {
	ready chan struct{}
	data  []byte
	err   error
}

type adaptiveSegmentCache struct {
	mu         sync.Mutex
	maxBytes   int64
	totalBytes int64
	entries    map[string]*adaptiveSegmentCacheEntry
	order      *list.List
	flights    map[string]*adaptiveSegmentCacheFlight
}

func newAdaptiveSegmentCache() *adaptiveSegmentCache {
	return &adaptiveSegmentCache{
		entries: make(map[string]*adaptiveSegmentCacheEntry),
		order:   list.New(),
		flights: make(map[string]*adaptiveSegmentCacheFlight),
	}
}

func (c *adaptiveSegmentCache) setLimit(limit int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxBytes = limit
	c.pruneLocked(time.Now())
}

func (c *adaptiveSegmentCache) get(key string, now time.Time) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry == nil || now.After(entry.expiresAt) {
		if entry != nil {
			c.removeEntryLocked(entry)
		}
		return nil, false
	}
	entry.lastAccess = now
	if entry.element != nil {
		c.order.MoveToFront(entry.element)
	}
	return entry.data, true
}

func (c *adaptiveSegmentCache) getOrLoad(ctx context.Context, key string, ttl time.Duration, loader func() ([]byte, error)) ([]byte, bool, bool, error) {
	now := time.Now()
	if data, ok := c.get(key, now); ok {
		return data, true, false, nil
	}

	c.mu.Lock()
	if flight := c.flights[key]; flight != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, false, true, ctx.Err()
		case <-flight.ready:
			return flight.data, false, true, flight.err
		}
	}
	flight := &adaptiveSegmentCacheFlight{ready: make(chan struct{})}
	c.flights[key] = flight
	c.mu.Unlock()

	data, err := loader()
	if err == nil && len(data) > 0 {
		c.store(key, data, ttl)
	}

	c.mu.Lock()
	flight.data = data
	flight.err = err
	close(flight.ready)
	delete(c.flights, key)
	c.mu.Unlock()
	return data, false, false, err
}

func (c *adaptiveSegmentCache) store(key string, data []byte, ttl time.Duration) {
	if len(data) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxBytes <= 0 || int64(len(data)) > c.maxBytes/2 {
		return
	}
	now := time.Now()
	if existing := c.entries[key]; existing != nil {
		c.totalBytes -= int64(len(existing.data))
		existing.data = data
		existing.expiresAt = now.Add(ttl)
		existing.lastAccess = now
		c.totalBytes += int64(len(data))
		if existing.element != nil {
			c.order.MoveToFront(existing.element)
		}
		c.pruneLocked(now)
		return
	}
	entry := &adaptiveSegmentCacheEntry{key: key, data: data, expiresAt: now.Add(ttl), lastAccess: now}
	entry.element = c.order.PushFront(entry)
	c.entries[key] = entry
	c.totalBytes += int64(len(data))
	c.pruneLocked(now)
}

func (c *adaptiveSegmentCache) removeEntryLocked(entry *adaptiveSegmentCacheEntry) {
	delete(c.entries, entry.key)
	c.totalBytes -= int64(len(entry.data))
	if entry.element != nil {
		c.order.Remove(entry.element)
		entry.element = nil
	}
}

func (c *adaptiveSegmentCache) pruneLocked(now time.Time) {
	for _, entry := range c.entries {
		if now.After(entry.expiresAt) {
			c.removeEntryLocked(entry)
		}
	}
	for c.maxBytes > 0 && c.totalBytes > c.maxBytes && c.order.Len() > 0 {
		back := c.order.Back()
		if back == nil {
			break
		}
		entry, _ := back.Value.(*adaptiveSegmentCacheEntry)
		if entry == nil {
			c.order.Remove(back)
			continue
		}
		c.removeEntryLocked(entry)
	}
}

func buildAdaptiveSegmentCacheKey(prepared *mappingForwardRequest, resp *http.Response, segment fixedWindowSegment, profile string) string {
	if prepared == nil || prepared.TargetURL == nil || resp == nil {
		return ""
	}
	builder := strings.Builder{}
	builder.WriteString(strings.ToUpper(strings.TrimSpace(firstNonEmpty(prepared.Method, http.MethodGet))))
	builder.WriteString("|")
	builder.WriteString(prepared.TargetURL.String())
	builder.WriteString("|")
	builder.WriteString(segment.rangeHeader)
	builder.WriteString("|")
	builder.WriteString(profile)
	if etag := strings.TrimSpace(resp.Header.Get("ETag")); etag != "" {
		builder.WriteString("|etag=")
		builder.WriteString(etag)
	} else if lastModified := strings.TrimSpace(resp.Header.Get("Last-Modified")); lastModified != "" {
		builder.WriteString("|lm=")
		builder.WriteString(lastModified)
	}
	return builder.String()
}

func maybePrefetchFixedWindowSegments(ctx context.Context, forward mappingForwarder, prepared *mappingForwardRequest, req *http.Request, initialResp *http.Response, requestID, traceID, mappingID, mappingName string, plan fixedWindowPlan, segments []fixedWindowSegment, afterIndex int) {
	if plan.prefetchSegments <= 0 || !plan.cacheEnabled || afterIndex < 0 || afterIndex >= len(segments) {
		return
	}
	copyLimit := plan.prefetchSegments
	for i := 1; i <= copyLimit; i++ {
		next := afterIndex + i
		if next >= len(segments) {
			break
		}
		segment := segments[next]
		go func(seg fixedWindowSegment) {
			prefetchCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			buf := acquireCopyBuffer()
			defer releaseCopyBuffer(buf)
			_, _ = fetchFixedWindowSegmentToBuffer(prefetchCtx, forward, prepared, req, initialResp, buf, requestID, traceID, mappingID, mappingName, plan, seg)
			log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_prefetch target_url=%s range=%s profile=%s", requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), seg.rangeHeader, plan.adaptiveProfile)
		}(segment)
	}
}

func minIntVal(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func minInt64Val(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64Val(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func adaptiveDecisionSummary(decision adaptiveDeliveryDecision) string {
	return fmt.Sprintf("profile=%s prefer_segmented_primary=%t segment_concurrency=%d concurrency_reason=%s window=%d prefetch=%d cache_enabled=%t stability_tier=%s", decision.Profile, decision.PreferSegmentedPrimary, decision.SegmentConcurrency, firstNonEmpty(strings.TrimSpace(decision.ConcurrencyReason), "base_plan"), decision.WindowBytes, decision.PrefetchSegments, decision.CacheEnabled, firstNonEmpty(strings.TrimSpace(decision.StabilityTier), "balanced"))
}
