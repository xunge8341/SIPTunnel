package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	mappingRTPResumeHardLimitBytes   int64 = 1 << 30
	adaptiveRangeRewriteReasonHeader       = "X-SIPTunnel-Range-Rewrite-Reason"
)

type byteRangeSpec struct {
	start    int64
	end      int64
	hasEnd   bool
	total    int64
	hasTotal bool
}

type segmentedDownloadProfile struct {
	name           string
	windowBytes    int64
	threshold      int64
	concurrency    int
	segmentRetries int
}

func formatOptionalResumeEnd(resumeEnd *int64) string {
	if resumeEnd == nil || *resumeEnd < 0 {
		return "-"
	}
	return strconv.FormatInt(*resumeEnd, 10)
}

func effectivePreparedMaxResponseBodyBytes(prepared *mappingForwardRequest) int64 {
	if prepared == nil {
		return 0
	}
	limit := prepared.MaxResponseBodyBytes
	if strings.EqualFold(strings.TrimSpace(prepared.Method), http.MethodGet) && strings.EqualFold(strings.TrimSpace(prepared.Mapping.ResponseMode), "RTP") {
		if limit <= 0 || limit < mappingRTPResumeHardLimitBytes {
			return mappingRTPResumeHardLimitBytes
		}
	}
	return limit
}

func copyForwardResponseWithResume(ctx context.Context, forward mappingForwarder, prepared *mappingForwardRequest, req *http.Request, initialResp *http.Response, w io.Writer, copyBuf []byte, requestID, traceID, mappingID, mappingName string) (int64, error) {
	return copyForwardResponseWithResumeBounds(ctx, forward, prepared, req, initialResp, w, copyBuf, requestID, traceID, mappingID, mappingName, -1, nil, boundaryResumeMaxAttempts())
}

// copyForwardResponseWithResumeBounds 在 RTP/大响应复制中承担“受限恢复”的兜底职责。
//
// 关键安全点：
// 1. 所有 resume 都从 baseResp 派生，保证起点以已写入字节数为准；
// 2. 若传入 resumeEnd，则每次 resume 只能落在当前闭区间窗口内；
// 3. 只有被识别为可恢复的读错误才会触发 resume，避免把业务错误误当作网络抖动重试。
func copyForwardResponseWithResumeBounds(ctx context.Context, forward mappingForwarder, prepared *mappingForwardRequest, req *http.Request, initialResp *http.Response, w io.Writer, copyBuf []byte, requestID, traceID, mappingID, mappingName string, copyLimit int64, resumeEnd *int64, resumeAttemptLimit int) (int64, error) {
	if initialResp == nil {
		return 0, fmt.Errorf("nil response")
	}
	currentResp := initialResp
	baseResp := initialResp
	var totalWritten int64
	resumeCount := 0
	maxResumeAttempts := boundaryResumeMaxAttempts()
	if resumeAttemptLimit > 0 && (maxResumeAttempts <= 0 || resumeAttemptLimit < maxResumeAttempts) {
		maxResumeAttempts = resumeAttemptLimit
	}
	if maxResumeAttempts <= 0 {
		maxResumeAttempts = 1
	}
	for {
		remaining := int64(-1)
		if copyLimit >= 0 {
			remaining = copyLimit - totalWritten
			if remaining <= 0 {
				return totalWritten, nil
			}
		}
		reader := io.Reader(currentResp.Body)
		if remaining >= 0 {
			reader = io.LimitReader(currentResp.Body, remaining)
		}
		n, err := io.CopyBuffer(w, reader, copyBuf)
		totalWritten += n
		if err == nil {
			if copyLimit >= 0 && totalWritten >= copyLimit {
				return totalWritten, nil
			}
			return totalWritten, nil
		}
		if !shouldResumeRTPResponseCopy(req, prepared, baseResp, err, totalWritten, resumeCount) {
			return totalWritten, err
		}
		failureClass := classifyWindowRecoveryFailure(err)
		if failureClass == windowRecoveryFailureUnknown {
			failureClass = windowRecoveryFailurePeerError
		}
		if resumeCount >= maxResumeAttempts {
			recoveryErr := &windowRecoveryError{Stage: "resume_limit", Class: windowRecoveryFailureThresholdExceeded, Strategy: windowRecoveryStrategyRestartWindow, ResumeAttempts: resumeCount, Err: err}
			logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, "resume_failure", "-", recoveryErr.Class, recoveryErr.Strategy, resumeCount, 0, recoveryErr)
			return totalWritten, recoveryErr
		}
		resumeReason := classifyRecoverableRTPReadError(err)
		resumePrepared, nextStart, rangeHeader, buildErr := buildPreparedResumeRequestWithLimit(prepared, baseResp, totalWritten, resumeEnd)
		if buildErr != nil {
			class := classifyWindowRecoveryFailure(buildErr)
			strategy := windowRecoveryStrategyForClass(class, true)
			recoveryErr := &windowRecoveryError{Stage: "resume_build", Class: class, Strategy: strategy, Range: rangeHeader, ResumeAttempts: resumeCount + 1, Err: buildErr}
			logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, "resume_failure", rangeHeader, class, strategy, resumeCount+1, 0, recoveryErr)
			return totalWritten, recoveryErr
		}
		_ = currentResp.Body.Close()
		log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=resume_plan forwarder=%s target_url=%s method=%s next_start=%d resume_end=%s range=%s bytes=%d attempt=%d resume_reason=%s range_playback=%t max_attempts=%d per_range_retries=%d err=%v", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, nextStart, formatOptionalResumeEnd(resumeEnd), rangeHeader, totalWritten, resumeCount+1, firstNonEmpty(resumeReason, "unknown"), isRangePlaybackRequest(prepared, req, baseResp), maxResumeAttempts, boundaryResumePerRangeRetries(), err)
		var resumeResp *http.Response
		var resumeErr error
		for retry := 0; retry <= boundaryResumePerRangeRetries(); retry++ {
			resumeResp, resumeErr = forward.ExecuteForward(ctx, resumePrepared)
			if resumeErr == nil {
				break
			}
			if retry >= boundaryResumePerRangeRetries() {
				class := classifyWindowRecoveryFailure(resumeErr)
				strategy := windowRecoveryStrategyForClass(class, resumeCount+1 >= maxResumeAttempts)
				recoveryErr := &windowRecoveryError{Stage: "resume_execute", Class: class, Strategy: strategy, Range: rangeHeader, ResumeAttempts: resumeCount + 1, Err: resumeErr}
				logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, "resume_failure", rangeHeader, class, strategy, resumeCount+1, 0, recoveryErr)
				return totalWritten, recoveryErr
			}
			backoff := time.Duration(1<<retry) * time.Second
			log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=resume_retry forwarder=%s target_url=%s method=%s next_start=%d range=%s attempt=%d retry=%d backoff_ms=%d range_playback=%t err=%v", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, nextStart, rangeHeader, resumeCount+1, retry+1, backoff.Milliseconds(), isRangePlaybackRequest(prepared, req, baseResp), resumeErr)
			select {
			case <-ctx.Done():
				return totalWritten, fmt.Errorf("rtp resume execute from byte %d canceled: %w", nextStart, ctx.Err())
			case <-time.After(backoff):
			}
		}
		if validateErr := validatePreparedResumeResponseWithLimit(baseResp, resumeResp, nextStart, resumeEnd); validateErr != nil {
			_ = resumeResp.Body.Close()
			class := classifyWindowRecoveryFailure(validateErr)
			strategy := windowRecoveryStrategyForClass(class, true)
			recoveryErr := &windowRecoveryError{Stage: "resume_validate", Class: class, Strategy: strategy, Range: rangeHeader, ResumeAttempts: resumeCount + 1, Err: validateErr}
			logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, "resume_failure", rangeHeader, class, strategy, resumeCount+1, 0, recoveryErr)
			return totalWritten, recoveryErr
		}
		resumeCount++
		if tracker := relayTransactionFromContext(ctx); tracker != nil {
			tracker.AddResume()
		}
		log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=resume_continue forwarder=%s target_url=%s method=%s next_start=%d attempt=%d status=%d range_playback=%t content_range=%q", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, nextStart, resumeCount, resumeResp.StatusCode, isRangePlaybackRequest(prepared, req, resumeResp), strings.TrimSpace(resumeResp.Header.Get("Content-Range")))
		currentResp = resumeResp
	}
}

type largeResponseDeliveryStrategy string

const (
	deliveryStrategyStreamPrimary            largeResponseDeliveryStrategy = "stream_primary"
	deliveryStrategyRangePrimary             largeResponseDeliveryStrategy = "range_primary"
	deliveryStrategyAdaptiveSegmentedPrimary largeResponseDeliveryStrategy = "adaptive_segmented_primary"
	deliveryStrategyFallbackSegmented        largeResponseDeliveryStrategy = "fallback_segmented"
)

func preparedOrRequestRangeHeader(prepared *mappingForwardRequest, req *http.Request) string {
	if req != nil {
		if v := strings.TrimSpace(req.Header.Get("Range")); v != "" {
			return v
		}
	}
	if prepared != nil {
		return strings.TrimSpace(prepared.Headers.Get("Range"))
	}
	return ""
}

func isOpenEndedSingleByteRange(raw string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || !strings.HasPrefix(raw, "bytes=") {
		return false
	}
	parts := strings.Split(strings.TrimSpace(raw[6:]), ",")
	if len(parts) != 1 {
		return false
	}
	bounds := strings.SplitN(strings.TrimSpace(parts[0]), "-", 2)
	if len(bounds) != 2 {
		return false
	}
	return strings.TrimSpace(bounds[0]) != "" && strings.TrimSpace(bounds[1]) == ""
}

func maybeRewriteOpenEndedRangeForAdaptivePlayback(prepared *mappingForwardRequest) bool {
	if prepared == nil || prepared.Headers == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(prepared.Method), http.MethodGet) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(prepared.Headers.Get(internalRangeFetchHeader)), "1") {
		return false
	}
	rawRange := strings.TrimSpace(prepared.Headers.Get("Range"))
	if !isOpenEndedSingleByteRange(rawRange) {
		return false
	}
	if !globalAdaptiveDelivery.recentProbeAbortPrepared(prepared) {
		return false
	}
	parsed, ok := parseSingleByteRangeHeader(rawRange, adaptiveOpenEndedRangeInitialWindowBytes())
	if !ok {
		return false
	}
	window := adaptiveOpenEndedRangeInitialWindowBytes()
	if window <= 0 {
		return false
	}
	end := parsed.start + window - 1
	if end < parsed.start {
		return false
	}
	newRange := fmt.Sprintf("bytes=%d-%d", parsed.start, end)
	if newRange == rawRange {
		return false
	}
	prepared.Headers.Set("Range", newRange)
	prepared.Headers.Set(adaptiveRangeRewriteReasonHeader, "recent_probe_abort_open_ended_window")
	return true
}

func chooseLargeResponseDeliveryStrategy(req *http.Request, prepared *mappingForwardRequest, resp *http.Response) largeResponseDeliveryStrategy {
	if req == nil || prepared == nil || resp == nil {
		return deliveryStrategyStreamPrimary
	}
	if !strings.EqualFold(strings.TrimSpace(req.Method), http.MethodGet) {
		return deliveryStrategyStreamPrimary
	}
	if strings.EqualFold(strings.TrimSpace(prepared.Headers.Get(internalRangeFetchHeader)), "1") {
		// 内部分段子请求本身已经是 fallback 产物，禁止再递归切段。
		return deliveryStrategyStreamPrimary
	}
	rangeHeader := preparedOrRequestRangeHeader(prepared, req)
	if rangeHeader != "" {
		// 非侵入式共性策略：无论是预览还是下载，只要客户端还在顺序拉流（bytes=N-），
		// 默认都先走连续流 + resume；只有显式闭区间 Range 才直接走 range primary。
		if isOpenEndedSingleByteRange(rangeHeader) {
			if globalAdaptiveDelivery.recentProbeAbort(prepared, resp) && !isGenericDownloadOpenEndedRange(prepared, req, resp) {
				return deliveryStrategyRangePrimary
			}
			return deliveryStrategyStreamPrimary
		}
		return deliveryStrategyRangePrimary
	}
	decision := globalAdaptiveDelivery.snapshot(prepared, req, resp, fixedWindowPlan{concurrency: 1, window: adaptivePlaybackHotWindowBytes()})
	if decision.PreferSegmentedPrimary {
		return deliveryStrategyAdaptiveSegmentedPrimary
	}
	// 非显式 Range 的大 GET 仍然优先整流 + resume；只有观测到持续差网络后才允许自适应切分段。
	return deliveryStrategyStreamPrimary
}

func shouldFallbackToFixedWindow(err error) bool {
	if err == nil {
		return false
	}
	var recoveryErr *windowRecoveryError
	if errors.As(err, &recoveryErr) {
		return recoveryErr.Strategy == windowRecoveryStrategyRestartWindow || recoveryErr.Class == windowRecoveryFailureThresholdExceeded
	}
	return false
}

func strategyReasonForSelection(strategy largeResponseDeliveryStrategy, req *http.Request, prepared *mappingForwardRequest, resp *http.Response) string {
	rangeHeader := preparedOrRequestRangeHeader(prepared, req)
	switch strategy {
	case deliveryStrategyRangePrimary:
		if isOpenEndedSingleByteRange(rangeHeader) && globalAdaptiveDelivery.recentProbeAbort(prepared, resp) {
			return "recent_probe_abort_range_primary"
		}
		return "explicit_range"
	case deliveryStrategyAdaptiveSegmentedPrimary:
		return "adaptive_network_degraded"
	case deliveryStrategyFallbackSegmented:
		return "fallback_after_stream_error"
	default:
		if isOpenEndedSingleByteRange(rangeHeader) {
			return "open_ended_range_stream_primary"
		}
		return "stream_before_fallback"
	}
}

func buildRemainingFixedWindowPlan(req *http.Request, prepared *mappingForwardRequest, resp *http.Response, alreadyWritten int64) (fixedWindowPlan, bool) {
	plan, ok := buildFixedWindowPlan(req, prepared, resp)
	if !ok {
		return fixedWindowPlan{}, false
	}
	if alreadyWritten <= 0 {
		return plan, true
	}
	plan.responseStart += alreadyWritten
	if plan.responseStart > plan.responseEnd {
		return fixedWindowPlan{}, false
	}
	return plan, true
}

func copyForwardResponseAdaptive(ctx context.Context, forward mappingForwarder, prepared *mappingForwardRequest, req *http.Request, initialResp *http.Response, w io.Writer, copyBuf []byte, requestID, traceID, mappingID, mappingName string) (int64, error) {
	strategy := chooseLargeResponseDeliveryStrategy(req, prepared, initialResp)
	selectionReason := strategyReasonForSelection(strategy, req, prepared, initialResp)
	rangePlayback := isRangePlaybackRequest(prepared, req, initialResp)
	downloadCandidate := isGenericLargeDownloadCandidate(prepared, req, initialResp)
	if tracker := nodeLatencyFromContext(ctx); tracker != nil {
		tracker.MarkStrategy(string(strategy), selectionReason)
	}
	if strategy == deliveryStrategyRangePrimary || strategy == deliveryStrategyAdaptiveSegmentedPrimary {
		if plan, ok := buildFixedWindowPlan(req, prepared, initialResp); ok {
			log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=delivery_strategy_selected target_url=%s method=%s strategy=%s reason=%s adaptive_profile=%s stability_tier=%s window=%d concurrency=%d concurrency_reason=%s prefetch_segments=%d cache_enabled=%t range_playback=%t download_candidate=%t", requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, strategy, selectionReason, plan.adaptiveProfile, firstNonEmpty(strings.TrimSpace(plan.stabilityTier), "balanced"), plan.window, plan.concurrency, firstNonEmpty(strings.TrimSpace(plan.concurrencyReason), "base_plan"), plan.prefetchSegments, plan.cacheEnabled, rangePlayback, downloadCandidate)
			written, err := copyForwardResponseWithFixedWindow(ctx, forward, prepared, req, initialResp, w, copyBuf, requestID, traceID, mappingID, mappingName, plan)
			globalAdaptiveDelivery.observeStreamResult(prepared, req, initialResp, strategy, err)
			if isGenericLargeDownloadCandidate(prepared, req, initialResp) {
				globalGenericDownloadController.observeTargetResult(prepared.TargetURL.String(), err)
			}
			return written, err
		}
	}
	log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=delivery_strategy_selected target_url=%s method=%s strategy=%s reason=%s range_playback=%t download_candidate=%t", requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, strategy, selectionReason, rangePlayback, downloadCandidate)
	written, err := copyForwardResponseWithResume(ctx, forward, prepared, req, initialResp, w, copyBuf, requestID, traceID, mappingID, mappingName)
	if err == nil || !shouldFallbackToFixedWindow(err) {
		globalAdaptiveDelivery.observeStreamResult(prepared, req, initialResp, strategy, err)
		if isGenericLargeDownloadCandidate(prepared, req, initialResp) {
			globalGenericDownloadController.observeTargetResult(prepared.TargetURL.String(), err)
		}
		return written, err
	}
	plan, ok := buildRemainingFixedWindowPlan(req, prepared, initialResp, written)
	if !ok {
		globalAdaptiveDelivery.observeStreamResult(prepared, req, initialResp, strategy, err)
		return written, err
	}
	log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=delivery_strategy_switch target_url=%s method=%s from=%s to=%s written=%d remaining_start=%d remaining_end=%d reason=%v adaptive_profile=%s stability_tier=%s window=%d concurrency=%d concurrency_reason=%s prefetch_segments=%d range_playback=%t download_candidate=%t", requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, deliveryStrategyStreamPrimary, deliveryStrategyFallbackSegmented, written, plan.responseStart, plan.responseEnd, err, plan.adaptiveProfile, firstNonEmpty(strings.TrimSpace(plan.stabilityTier), "balanced"), plan.window, plan.concurrency, firstNonEmpty(strings.TrimSpace(plan.concurrencyReason), "base_plan"), plan.prefetchSegments, rangePlayback, downloadCandidate)
	fallbackWritten, fallbackErr := copyForwardResponseWithFixedWindow(ctx, forward, prepared, req, initialResp, w, copyBuf, requestID, traceID, mappingID, mappingName, plan)
	globalAdaptiveDelivery.observeStreamResult(prepared, req, initialResp, deliveryStrategyFallbackSegmented, fallbackErr)
	if isGenericLargeDownloadCandidate(prepared, req, initialResp) {
		globalGenericDownloadController.observeTargetResult(prepared.TargetURL.String(), fallbackErr)
	}
	return written + fallbackWritten, fallbackErr
}

type fixedWindowPlan struct {
	responseStart     int64
	responseEnd       int64
	total             int64
	window            int64
	threshold         int64
	concurrency       int
	concurrencyReason string
	segmentRetries    int
	profileName       string
	rangePlayback     bool
	adaptiveProfile   string
	stabilityTier     string
	prefetchSegments  int
	cacheEnabled      bool
	cacheTTL          time.Duration
}

func applyAdaptiveFixedWindowPlan(prepared *mappingForwardRequest, req *http.Request, resp *http.Response, plan fixedWindowPlan) fixedWindowPlan {
	decision := globalAdaptiveDelivery.snapshot(prepared, req, resp, plan)
	plan.window = maxInt64Val(plan.window, decision.WindowBytes)
	plan.concurrency = maxIntVal(plan.concurrency, decision.SegmentConcurrency)
	plan.concurrencyReason = firstNonEmpty(strings.TrimSpace(decision.ConcurrencyReason), strings.TrimSpace(plan.concurrencyReason), "base_plan")
	plan.prefetchSegments = decision.PrefetchSegments
	plan.cacheEnabled = decision.CacheEnabled
	plan.cacheTTL = decision.CacheTTL
	plan.adaptiveProfile = string(decision.Profile)
	target := "-"
	method := "-"
	if prepared != nil {
		method = firstNonEmpty(strings.TrimSpace(prepared.Method), method)
		if prepared.TargetURL != nil {
			target = prepared.TargetURL.String()
		}
	}
	log.Printf("mapping-runtime stage=adaptive_delivery_plan target_url=%s method=%s decision=%s", target, method, adaptiveDecisionSummary(decision))
	return plan
}

func buildFixedWindowPlan(req *http.Request, prepared *mappingForwardRequest, resp *http.Response) (fixedWindowPlan, bool) {
	if req == nil || prepared == nil || resp == nil {
		return fixedWindowPlan{}, false
	}
	if !strings.EqualFold(strings.TrimSpace(req.Method), http.MethodGet) {
		return fixedWindowPlan{}, false
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fixedWindowPlan{}, false
	}
	if isRealtimeStreamingRequest(req, resp) {
		return fixedWindowPlan{}, false
	}
	profile := segmentedDownloadProfileForResponse(prepared, req, resp)
	start := int64(0)
	end := resp.ContentLength - 1
	total := resp.ContentLength
	if cr, ok := parseContentRangeHeader(resp.Header.Get("Content-Range")); ok {
		start = cr.start
		end = cr.end
		if cr.hasTotal {
			total = cr.total
		}
	} else {
		if resp.ContentLength <= 0 {
			return fixedWindowPlan{}, false
		}
		if rangeHeader := strings.TrimSpace(req.Header.Get("Range")); rangeHeader != "" {
			parsed, ok := parseSingleByteRangeHeader(rangeHeader, resp.ContentLength)
			if !ok {
				return fixedWindowPlan{}, false
			}
			start = parsed.start
			end = parsed.end
			total = resp.ContentLength
		}
	}
	if total <= 0 || end < start {
		return fixedWindowPlan{}, false
	}
	if isOpenEndedSingleByteRange(strings.TrimSpace(req.Header.Get("Range"))) && total > 0 {
		// 对 recent-probe-abort 后被改写成首个有界窗口的 bytes=N- 请求，仍应按“剩余全量”建计划，
		// 否则只会拿到首个 8MiB 响应然后停止，无法进入真正的并行分段主路径。
		end = total - 1
	}
	remaining := end - start + 1
	if remaining < profile.threshold {
		return fixedWindowPlan{}, false
	}
	if !supportsSegmentedRangeFetch(req, resp) {
		return fixedWindowPlan{}, false
	}
	window := profile.windowBytes
	if window <= 0 {
		window = boundarySegmentedDownloadProfile().windowBytes
	}
	concurrency := profile.concurrency
	concurrencyReason := "profile_default"
	if concurrency <= 0 {
		concurrency = 1
		concurrencyReason = "profile_default_min_guard"
	}
	if rangeHeader := strings.TrimSpace(req.Header.Get("Range")); rangeHeader != "" {
		concurrency = 1
		concurrencyReason = "explicit_range_initial_guard"
	}
	segmentRetries := profile.segmentRetries
	if segmentRetries <= 0 {
		segmentRetries = 1
	}
	plan := fixedWindowPlan{responseStart: start, responseEnd: end, total: total, window: window, threshold: profile.threshold, concurrency: concurrency, concurrencyReason: concurrencyReason, segmentRetries: segmentRetries, profileName: profile.name, rangePlayback: isRangePlaybackRequest(prepared, req, resp)}
	return applyAdaptiveFixedWindowPlan(prepared, req, resp, plan), true
}

func supportsSegmentedRangeFetch(req *http.Request, resp *http.Response) bool {
	if resp == nil {
		return false
	}
	if _, ok := parseContentRangeHeader(resp.Header.Get("Content-Range")); ok {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Accept-Ranges")), "bytes") {
		return true
	}
	if strings.TrimSpace(resp.Header.Get("Etag")) != "" || strings.TrimSpace(resp.Header.Get("Last-Modified")) != "" {
		return true
	}
	if isLikelyLargeDownloadResponse(req, resp) {
		return true
	}
	return false
}

func isLikelyLargeDownloadResponse(req *http.Request, resp *http.Response) bool {
	if resp == nil {
		return false
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	cd := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Disposition")))
	if strings.Contains(cd, "attachment") {
		return true
	}
	for _, prefix := range []string{"video/", "audio/", "image/"} {
		if strings.HasPrefix(ct, prefix) {
			return true
		}
	}
	for _, token := range []string{"application/octet-stream", "application/zip", "application/x-zip", "application/x-rar", "application/pdf", "application/vnd", "application/msword", "application/x-msdownload"} {
		if strings.Contains(ct, token) {
			return true
		}
	}
	path := ""
	if req != nil && req.URL != nil {
		path = strings.ToLower(strings.TrimSpace(req.URL.Path))
	}
	for _, suffix := range []string{".mp4", ".mkv", ".avi", ".mov", ".zip", ".rar", ".7z", ".tar", ".gz", ".iso", ".exe", ".apk", ".pdf", ".jpg", ".jpeg", ".png"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func isRealtimeStreamingRequest(req *http.Request, resp *http.Response) bool {
	if req != nil && req.URL != nil {
		path := strings.ToLower(strings.TrimSpace(req.URL.Path))
		if strings.Contains(path, "socket.io") || strings.Contains(path, "/events") || strings.Contains(path, "/stream") || strings.Contains(path, "/ws") {
			return true
		}
	}
	ct := ""
	if resp != nil {
		ct = strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	}
	return strings.Contains(ct, "text/event-stream")
}

// genericDownloadTransferID 用于把同一个外层下载请求派生出来的内部分段子请求串成一个“下载事务”。
// 带宽整形和公平分享必须按外层下载事务统计，而不是按每个 segment child 统计；
// 否则一个下载开多个分段时，会把 active_global 错算大，从而导致单个下载吃满带宽、其他下载起不来。
func genericDownloadTransferID(prepared *mappingForwardRequest, requestID string) string {
	if prepared != nil && prepared.Headers != nil {
		if transferID := strings.TrimSpace(prepared.Headers.Get(downloadTransferIDHeader)); transferID != "" {
			return transferID
		}
	}
	return strings.TrimSpace(requestID)
}

func parseSingleByteRangeHeader(raw string, total int64) (byteRangeSpec, bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || !strings.HasPrefix(raw, "bytes=") {
		return byteRangeSpec{}, false
	}
	parts := strings.Split(strings.TrimSpace(raw[6:]), ",")
	if len(parts) != 1 {
		return byteRangeSpec{}, false
	}
	bounds := strings.SplitN(strings.TrimSpace(parts[0]), "-", 2)
	if len(bounds) != 2 {
		return byteRangeSpec{}, false
	}
	if bounds[0] == "" {
		if total <= 0 {
			return byteRangeSpec{}, false
		}
		suffix, err := strconv.ParseInt(strings.TrimSpace(bounds[1]), 10, 64)
		if err != nil || suffix <= 0 {
			return byteRangeSpec{}, false
		}
		if suffix > total {
			suffix = total
		}
		return byteRangeSpec{start: total - suffix, end: total - 1, hasEnd: true, total: total, hasTotal: total > 0}, true
	}
	start, err := strconv.ParseInt(strings.TrimSpace(bounds[0]), 10, 64)
	if err != nil || start < 0 {
		return byteRangeSpec{}, false
	}
	end := total - 1
	if strings.TrimSpace(bounds[1]) != "" {
		end, err = strconv.ParseInt(strings.TrimSpace(bounds[1]), 10, 64)
		if err != nil || end < start {
			return byteRangeSpec{}, false
		}
	}
	if total > 0 && end >= total {
		end = total - 1
	}
	return byteRangeSpec{start: start, end: end, hasEnd: true, total: total, hasTotal: total > 0}, true
}

func copyForwardResponseWithFixedWindow(ctx context.Context, forward mappingForwarder, prepared *mappingForwardRequest, req *http.Request, initialResp *http.Response, w io.Writer, copyBuf []byte, requestID, traceID, mappingID, mappingName string, plan fixedWindowPlan) (int64, error) {
	if initialResp == nil {
		return 0, fmt.Errorf("nil initial response")
	}
	segments := buildFixedWindowSegments(plan)
	log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_plan forwarder=%s target_url=%s method=%s range=%d-%d total=%d window=%d threshold=%d segments=%d concurrency=%d concurrency_reason=%s segment_retries=%d profile=%s adaptive_profile=%s stability_tier=%s prefetch_segments=%d cache_enabled=%t range_playback=%t range_response_wait_ms=%d", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, plan.responseStart, plan.responseEnd, plan.total, plan.window, plan.threshold, len(segments), plan.concurrency, firstNonEmpty(strings.TrimSpace(plan.concurrencyReason), "base_plan"), plan.segmentRetries, plan.profileName, plan.adaptiveProfile, firstNonEmpty(strings.TrimSpace(plan.stabilityTier), "balanced"), plan.prefetchSegments, plan.cacheEnabled, plan.rangePlayback, boundaryRangeResponseStartWait().Milliseconds())
	if len(segments) == 0 {
		_ = initialResp.Body.Close()
		return 0, fmt.Errorf("empty fixed window plan")
	}
	var totalWritten int64
	if canReuseInitialFixedWindowSegment(req, initialResp, segments[0]) {
		log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=initial_segment_reuse target_url=%s method=%s range=%s", requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, segments[0].rangeHeader)
		written, err := copyInitialFixedWindowSegment(initialResp, w, copyBuf, requestID, traceID, mappingID, mappingName, prepared, plan, segments[0])
		totalWritten += written
		if err != nil {
			return totalWritten, err
		}
		segments = segments[1:]
	} else {
		// 只有确认无法复用首段时，才关闭已有流，避免“先起 RTP 再立刻 cancel”的无效往返。
		_ = initialResp.Body.Close()
	}
	if len(segments) == 0 {
		return totalWritten, nil
	}
	if len(segments) == 1 || plan.concurrency <= 1 {
		written, err := copyForwardResponseWithFixedWindowSequential(ctx, forward, prepared, req, initialResp, w, copyBuf, requestID, traceID, mappingID, mappingName, plan, segments)
		return totalWritten + written, err
	}
	written, err := copyForwardResponseWithFixedWindowParallel(ctx, forward, prepared, req, initialResp, w, requestID, traceID, mappingID, mappingName, plan, segments)
	return totalWritten + written, err
}

func canReuseInitialFixedWindowSegment(req *http.Request, initialResp *http.Response, segment fixedWindowSegment) bool {
	if req == nil || initialResp == nil {
		return false
	}
	switch initialResp.StatusCode {
	case http.StatusPartialContent:
		cr, ok := parseContentRangeHeader(initialResp.Header.Get("Content-Range"))
		if !ok {
			return false
		}
		return cr.start == segment.start && cr.end == segment.end
	case http.StatusOK:
		if strings.TrimSpace(req.Header.Get("Range")) != "" {
			return false
		}
		return segment.index == 0 && segment.start == 0
	default:
		return false
	}
}

func copyInitialFixedWindowSegment(initialResp *http.Response, w io.Writer, copyBuf []byte, requestID, traceID, mappingID, mappingName string, prepared *mappingForwardRequest, plan fixedWindowPlan, segment fixedWindowSegment) (int64, error) {
	defer initialResp.Body.Close()
	expected := segment.end - segment.start + 1
	reader := io.Reader(initialResp.Body)
	mode := "reuse_partial"
	if initialResp.StatusCode == http.StatusOK {
		reader = io.LimitReader(initialResp.Body, expected)
		mode = "reuse_fullbody_head"
	}
	written, err := io.CopyBuffer(w, reader, copyBuf)
	if err != nil {
		return written, err
	}
	targetURL := "-"
	method := "-"
	if prepared != nil {
		method = firstNonEmpty(strings.TrimSpace(prepared.Method), method)
		if prepared.TargetURL != nil {
			targetURL = firstNonEmpty(strings.TrimSpace(prepared.TargetURL.String()), targetURL)
		}
	}
	log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_complete forwarder=reuse_initial target_url=%s method=%s range=%s segment=%d written=%d progress=%d/%d mode=%s profile=%s adaptive_profile=%s", requestID, traceID, mappingID, mappingName, targetURL, method, segment.rangeHeader, segment.index+1, written, written, plan.responseEnd-plan.responseStart+1, mode, plan.profileName, plan.adaptiveProfile)
	if written != expected {
		return written, fmt.Errorf("fixed window initial segment short write range=%s wrote=%d expected=%d", segment.rangeHeader, written, expected)
	}
	return written, nil
}

type fixedWindowSegment struct {
	index       int
	start       int64
	end         int64
	rangeHeader string
}

type fixedWindowSegmentResult struct {
	segment fixedWindowSegment
	data    []byte
	err     error
}

func buildFixedWindowSegments(plan fixedWindowPlan) []fixedWindowSegment {
	if plan.window <= 0 || plan.responseEnd < plan.responseStart {
		return nil
	}
	segments := make([]fixedWindowSegment, 0, int((plan.responseEnd-plan.responseStart)/plan.window)+1)
	currentStart := plan.responseStart
	for currentStart <= plan.responseEnd {
		currentEnd := currentStart + plan.window - 1
		if currentEnd > plan.responseEnd {
			currentEnd = plan.responseEnd
		}
		segments = append(segments, fixedWindowSegment{index: len(segments), start: currentStart, end: currentEnd, rangeHeader: fmt.Sprintf("bytes=%d-%d", currentStart, currentEnd)})
		currentStart = currentEnd + 1
	}
	return segments
}

func copyForwardResponseWithFixedWindowSequential(ctx context.Context, forward mappingForwarder, prepared *mappingForwardRequest, req *http.Request, initialResp *http.Response, w io.Writer, copyBuf []byte, requestID, traceID, mappingID, mappingName string, plan fixedWindowPlan, segments []fixedWindowSegment) (int64, error) {
	var totalWritten int64
	for _, segment := range segments {
		written, err := fetchAndWriteFixedWindowSegment(ctx, forward, prepared, req, initialResp, w, copyBuf, requestID, traceID, mappingID, mappingName, plan, segment)
		totalWritten += written
		if err != nil {
			return totalWritten, err
		}
		maybePrefetchFixedWindowSegments(ctx, forward, prepared, req, initialResp, requestID, traceID, mappingID, mappingName, plan, segments, segment.index)
	}
	return totalWritten, nil
}

func copyForwardResponseWithFixedWindowParallel(ctx context.Context, forward mappingForwarder, prepared *mappingForwardRequest, req *http.Request, initialResp *http.Response, w io.Writer, requestID, traceID, mappingID, mappingName string, plan fixedWindowPlan, segments []fixedWindowSegment) (int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	workerCount := plan.concurrency
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > len(segments) {
		workerCount = len(segments)
	}
	jobs := make(chan fixedWindowSegment)
	results := make(chan fixedWindowSegmentResult, len(segments))
	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		copyBuf := acquireCopyBuffer()
		defer releaseCopyBuffer(copyBuf)
		for segment := range jobs {
			data, err := fetchFixedWindowSegmentToBuffer(ctx, forward, prepared, req, initialResp, copyBuf, requestID, traceID, mappingID, mappingName, plan, segment)
			select {
			case results <- fixedWindowSegmentResult{segment: segment, data: data, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go worker()
	}
	go func() {
		defer close(jobs)
		for _, segment := range segments {
			select {
			case jobs <- segment:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	pending := make(map[int]fixedWindowSegmentResult, len(segments))
	nextIndex := 0
	var totalWritten int64
	var firstErr error
	for res := range results {
		if res.err != nil && firstErr == nil {
			firstErr = res.err
			cancel()
		}
		pending[res.segment.index] = res
		for {
			ready, ok := pending[nextIndex]
			if !ok {
				break
			}
			delete(pending, nextIndex)
			if ready.err != nil {
				if firstErr == nil {
					firstErr = ready.err
				}
				return totalWritten, firstErr
			}
			written, err := w.Write(ready.data)
			totalWritten += int64(written)
			if err != nil {
				return totalWritten, err
			}
			if written != len(ready.data) {
				return totalWritten, io.ErrShortWrite
			}
			expected := ready.segment.end - ready.segment.start + 1
			log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_complete forwarder=%s target_url=%s method=%s range=%s segment=%d written=%d progress=%d/%d mode=parallel profile=%s adaptive_profile=%s", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, ready.segment.rangeHeader, ready.segment.index+1, written, totalWritten, plan.responseEnd-plan.responseStart+1, plan.profileName, plan.adaptiveProfile)
			if int64(written) != expected {
				return totalWritten, fmt.Errorf("fixed window short write range=%s wrote=%d expected=%d", ready.segment.rangeHeader, written, expected)
			}
			maybePrefetchFixedWindowSegments(ctx, forward, prepared, req, initialResp, requestID, traceID, mappingID, mappingName, plan, segments, ready.segment.index)
			nextIndex++
		}
	}
	if firstErr != nil {
		return totalWritten, firstErr
	}
	if nextIndex != len(segments) {
		return totalWritten, fmt.Errorf("fixed window incomplete: written_segments=%d total_segments=%d", nextIndex, len(segments))
	}
	return totalWritten, nil
}

func fetchAndWriteFixedWindowSegment(ctx context.Context, forward mappingForwarder, prepared *mappingForwardRequest, req *http.Request, initialResp *http.Response, w io.Writer, copyBuf []byte, requestID, traceID, mappingID, mappingName string, plan fixedWindowPlan, segment fixedWindowSegment) (int64, error) {
	data, err := fetchFixedWindowSegmentToBuffer(ctx, forward, prepared, req, initialResp, copyBuf, requestID, traceID, mappingID, mappingName, plan, segment)
	if err != nil {
		return 0, err
	}
	written, writeErr := w.Write(data)
	if writeErr != nil {
		return int64(written), writeErr
	}
	if written != len(data) {
		return int64(written), io.ErrShortWrite
	}
	expected := segment.end - segment.start + 1
	log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_complete forwarder=%s target_url=%s method=%s range=%s segment=%d written=%d progress=%d/%d mode=sequential profile=%s adaptive_profile=%s", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, segment.rangeHeader, segment.index+1, written, segment.end-plan.responseStart+1, plan.responseEnd-plan.responseStart+1, plan.profileName, plan.adaptiveProfile)
	if int64(written) != expected {
		return int64(written), fmt.Errorf("fixed window short write range=%s wrote=%d expected=%d", segment.rangeHeader, written, expected)
	}
	return int64(written), nil
}

func fetchFixedWindowSegmentToBuffer(ctx context.Context, forward mappingForwarder, prepared *mappingForwardRequest, req *http.Request, initialResp *http.Response, copyBuf []byte, requestID, traceID, mappingID, mappingName string, plan fixedWindowPlan, segment fixedWindowSegment) ([]byte, error) {
	loader := func() ([]byte, error) {
		segmentPrepared := clonePreparedForwardRequest(prepared)
		segmentPrepared.Headers.Set("Range", segment.rangeHeader)
		segmentPrepared.Headers.Set(internalRangeFetchHeader, "1")
		segmentPrepared.Headers.Set(downloadProfileHeader, plan.profileName)
		if plan.rangePlayback {
			segmentPrepared.Headers.Set(playbackIntentHeader, "1")
		} else {
			segmentPrepared.Headers.Del(playbackIntentHeader)
		}
		if transferID := genericDownloadTransferID(prepared, requestID); transferID != "" {
			segmentPrepared.Headers.Set(downloadTransferIDHeader, transferID)
		}
		if etag := strings.TrimSpace(initialResp.Header.Get("ETag")); etag != "" {
			segmentPrepared.Headers.Set("If-Range", etag)
		} else if lastModified := strings.TrimSpace(initialResp.Header.Get("Last-Modified")); lastModified != "" {
			segmentPrepared.Headers.Set("If-Range", lastModified)
		}
		if strings.TrimSpace(initialResp.Header.Get("Content-Encoding")) == "" {
			segmentPrepared.Headers.Set("Accept-Encoding", "identity")
		}
		segmentPrepared.MaxResponseBodyBytes = effectivePreparedMaxResponseBodyBytes(segmentPrepared)
		expected := segment.end - segment.start + 1
		attempts := plan.segmentRetries
		if attempts <= 0 {
			attempts = 1
		}
		resumeAttemptLimit := fixedWindowResumeAttemptLimit(plan)
		var lastErr error
		for attempt := 0; attempt < attempts; attempt++ {
			var segmentResp *http.Response
			var execErr error
			for retry := 0; retry <= boundaryResumePerRangeRetries(); retry++ {
				segmentResp, execErr = forward.ExecuteForward(ctx, segmentPrepared)
				if execErr == nil {
					break
				}
				if retry >= boundaryResumePerRangeRetries() {
					class := classifyWindowRecoveryFailure(execErr)
					strategy := windowRecoveryStrategyForClass(class, true)
					lastErr = &windowRecoveryError{Stage: "segment_execute", Class: class, Strategy: strategy, Range: segment.rangeHeader, SegmentAttempt: attempt + 1, Err: execErr}
					logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, "segment_failure", segment.rangeHeader, class, strategy, 0, attempt+1, lastErr)
					break
				}
				backoff := time.Duration(1<<retry) * time.Second
				log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_retry forwarder=%s target_url=%s method=%s range=%s segment=%d retry=%d backoff_ms=%d err=%v", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, segment.rangeHeader, segment.index+1, retry+1, backoff.Milliseconds(), execErr)
				select {
				case <-ctx.Done():
					return nil, fmt.Errorf("fixed window execute range=%s canceled: %w", segment.rangeHeader, ctx.Err())
				case <-time.After(backoff):
				}
			}
			if execErr != nil {
				if attempt >= attempts-1 {
					return nil, lastErr
				}
				backoff := time.Duration(attempt+1) * time.Second
				log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_restart forwarder=%s target_url=%s method=%s range=%s segment=%d attempt=%d backoff_ms=%d err=%v", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, segment.rangeHeader, segment.index+1, attempt+1, backoff.Milliseconds(), lastErr)
				select {
				case <-ctx.Done():
					return nil, fmt.Errorf("fixed window execute range=%s canceled: %w", segment.rangeHeader, ctx.Err())
				case <-time.After(backoff):
				}
				continue
			}
			if validateErr := validateFixedWindowResponse(initialResp, segmentResp, segment.start, segment.end); validateErr != nil {
				_ = segmentResp.Body.Close()
				class := classifyWindowRecoveryFailure(validateErr)
				strategy := windowRecoveryStrategyForClass(class, true)
				lastErr = &windowRecoveryError{Stage: "segment_validate", Class: class, Strategy: strategy, Range: segment.rangeHeader, SegmentAttempt: attempt + 1, Err: validateErr}
				logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, "segment_failure", segment.rangeHeader, class, strategy, 0, attempt+1, lastErr)
			} else {
				log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_start forwarder=%s target_url=%s method=%s range=%s segment=%d status=%d content_range=%q profile=%s adaptive_profile=%s attempt=%d resume_attempt_limit=%d", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, segment.rangeHeader, segment.index+1, segmentResp.StatusCode, strings.TrimSpace(segmentResp.Header.Get("Content-Range")), plan.profileName, plan.adaptiveProfile, attempt+1, resumeAttemptLimit)
				var buf bytes.Buffer
				if expected > 0 && expected < 32<<20 {
					buf.Grow(int(expected))
				}
				resumeEnd := segment.end
				written, copyErr := copyForwardResponseWithResumeBounds(ctx, forward, segmentPrepared, req, segmentResp, &buf, copyBuf, requestID, traceID, mappingID, mappingName, expected, &resumeEnd, resumeAttemptLimit)
				_ = segmentResp.Body.Close()
				if copyErr == nil && written == expected {
					return append([]byte(nil), buf.Bytes()...), nil
				}
				if copyErr != nil {
					class := classifyWindowRecoveryFailure(copyErr)
					strategy := windowRecoveryStrategyForClass(class, false)
					if recoveryErr := new(windowRecoveryError); errors.As(copyErr, &recoveryErr) && recoveryErr != nil && recoveryErr.Strategy != "" {
						strategy = recoveryErr.Strategy
					}
					lastErr = &windowRecoveryError{Stage: "segment_copy", Class: class, Strategy: strategy, Range: segment.rangeHeader, ResumeAttempts: resumeAttemptLimit, SegmentAttempt: attempt + 1, Err: copyErr}
					logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, "segment_failure", segment.rangeHeader, class, strategy, resumeAttemptLimit, attempt+1, lastErr)
					if strategy == windowRecoveryStrategyRestartWindow {
						logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, "segment_strategy_switch", segment.rangeHeader, class, windowRecoveryStrategyRestartWindow, resumeAttemptLimit, attempt+1, copyErr)
					}
				} else {
					lastErr = &windowRecoveryError{Stage: "segment_copy", Class: windowRecoveryFailureShortCopy, Strategy: windowRecoveryStrategyRestartWindow, Range: segment.rangeHeader, SegmentAttempt: attempt + 1, Err: fmt.Errorf("fixed window short copy range=%s wrote=%d expected=%d", segment.rangeHeader, written, expected)}
					logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), prepared.Method, "segment_failure", segment.rangeHeader, windowRecoveryFailureShortCopy, windowRecoveryStrategyRestartWindow, resumeAttemptLimit, attempt+1, lastErr)
				}
			}
			if attempt >= attempts-1 {
				return nil, lastErr
			}
			backoff := time.Duration(attempt+1) * time.Second
			log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_restart forwarder=%s target_url=%s method=%s range=%s segment=%d attempt=%d backoff_ms=%d err=%v", requestID, traceID, mappingID, mappingName, mappingForwarderModeName(forward), prepared.TargetURL.String(), prepared.Method, segment.rangeHeader, segment.index+1, attempt+1, backoff.Milliseconds(), lastErr)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("fixed window execute range=%s canceled: %w", segment.rangeHeader, ctx.Err())
			case <-time.After(backoff):
			}
		}
		return nil, lastErr
	}
	globalAdaptiveDelivery.cache.setLimit(adaptivePlaybackSegmentCacheBytes())
	cacheKey := ""
	if plan.cacheEnabled {
		cacheKey = buildAdaptiveSegmentCacheKey(prepared, initialResp, segment, plan.profileName)
	}
	if cacheKey != "" {
		data, hit, shared, err := globalAdaptiveDelivery.cache.getOrLoad(ctx, cacheKey, plan.cacheTTL, loader)
		if hit || shared {
			log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=segment_cache_hit target_url=%s range=%s profile=%s adaptive_profile=%s shared=%t bytes=%d ttl_ms=%d", requestID, traceID, mappingID, mappingName, prepared.TargetURL.String(), segment.rangeHeader, plan.profileName, plan.adaptiveProfile, shared, len(data), plan.cacheTTL.Milliseconds())
		}
		globalAdaptiveDelivery.observeSegmentResult(prepared, req, initialResp, hit || shared, err)
		return data, err
	}
	data, err := loader()
	globalAdaptiveDelivery.observeSegmentResult(prepared, req, initialResp, false, err)
	return data, err
}

func validateFixedWindowResponse(baseResp, resp *http.Response, expectedStart, expectedEnd int64) error {
	if resp == nil {
		return fmt.Errorf("nil response")
	}
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("expected 206 partial content, got %d", resp.StatusCode)
	}
	cr, ok := parseContentRangeHeader(resp.Header.Get("Content-Range"))
	if !ok {
		return fmt.Errorf("missing or invalid content-range")
	}
	if cr.start != expectedStart || cr.end != expectedEnd {
		return fmt.Errorf("content-range mismatch expected=%d-%d got=%d-%d", expectedStart, expectedEnd, cr.start, cr.end)
	}
	if baseResp != nil {
		if ct0, ct1 := strings.TrimSpace(baseResp.Header.Get("Content-Type")), strings.TrimSpace(resp.Header.Get("Content-Type")); ct0 != "" && ct1 != "" && !strings.EqualFold(ct0, ct1) {
			return fmt.Errorf("content-type changed from %q to %q", ct0, ct1)
		}
	}
	return nil
}

func clonePreparedForwardRequest(prepared *mappingForwardRequest) *mappingForwardRequest {
	if prepared == nil {
		return nil
	}
	clone := *prepared
	if prepared.Headers != nil {
		clone.Headers = prepared.Headers.Clone()
	} else {
		clone.Headers = make(http.Header)
	}
	return &clone
}

func classifyRecoverableRTPReadError(err error) string {
	return classifyCommonTransferFailure(err)
}

func classifyWindowRecoveryFailure(err error) windowRecoveryFailureClass {
	if err == nil {
		return windowRecoveryFailureUnknown
	}
	if recoveryErr := new(windowRecoveryError); errors.As(err, &recoveryErr) && recoveryErr != nil && recoveryErr.Class != "" {
		return recoveryErr.Class
	}
	if reason := classifyCommonTransferFailure(err); reason != "" {
		switch reason {
		case failureReasonTimeout:
			return windowRecoveryFailureTimeout
		case failureReasonRTPGapTimeout, failureReasonRTPSequenceGap:
			return windowRecoveryFailureSequenceGap
		case failureReasonUnexpectedEOF, failureReasonConnectionReset, failureReasonBrokenPipe:
			return windowRecoveryFailurePeerError
		}
	}
	errText := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(errText, "resume start") && strings.Contains(errText, "beyond limit"):
		return windowRecoveryFailureOutOfWindow
	case strings.Contains(errText, "content-range end overflow"):
		return windowRecoveryFailureOutOfWindow
	case strings.Contains(errText, "content-range start mismatch"):
		return windowRecoveryFailureOutOfOrder
	case strings.Contains(errText, "content-range mismatch"), strings.Contains(errText, "missing or invalid content-range"):
		return windowRecoveryFailureRangeMismatch
	case strings.Contains(errText, "short copy"), strings.Contains(errText, "short write"):
		return windowRecoveryFailureShortCopy
	case strings.Contains(errText, "context deadline exceeded"), strings.Contains(errText, "timeout"), strings.Contains(errText, "i/o timeout"):
		return windowRecoveryFailureTimeout
	case strings.Contains(errText, "rtp sequence discontinuity"), strings.Contains(errText, "rtp pending gap"), strings.Contains(errText, "sequence gap"):
		return windowRecoveryFailureSequenceGap
	case strings.Contains(errText, "expected 206 partial content"), strings.Contains(errText, "content-type changed"):
		return windowRecoveryFailurePeerError
	case strings.Contains(errText, "connection reset"), strings.Contains(errText, "broken pipe"), strings.Contains(errText, "unexpected eof"):
		return windowRecoveryFailurePeerError
	default:
		return windowRecoveryFailureUnknown
	}
}

func windowRecoveryStrategyForClass(class windowRecoveryFailureClass, thresholdReached bool) windowRecoveryStrategy {
	if thresholdReached {
		return windowRecoveryStrategyRestartWindow
	}
	switch class {
	case windowRecoveryFailureOutOfWindow, windowRecoveryFailureRangeMismatch, windowRecoveryFailureOutOfOrder:
		return windowRecoveryStrategyRestartWindow
	case windowRecoveryFailurePeerError, windowRecoveryFailureTimeout, windowRecoveryFailureSequenceGap, windowRecoveryFailureShortCopy:
		return windowRecoveryStrategyResumeWithinWindow
	default:
		return windowRecoveryStrategyAbortTransaction
	}
}

func fixedWindowResumeAttemptLimit(plan fixedWindowPlan) int {
	// 下载类 generic-rtp 允许更积极的窗口内恢复，避免一次 gap 直接把整个下载拖回外层重下。
	var limit int
	var global int
	if plan.profileName == "generic-rtp" {
		limit = genericDownloadResumePerRangeRetries() + 1
		global = genericDownloadResumeMaxAttempts()
	} else {
		// 同一 window 内的 resume 不是越多越安全。
		limit = boundaryResumePerRangeRetries() + 1
		global = boundaryResumeMaxAttempts()
	}
	if limit <= 0 {
		limit = 1
	}
	maxWindowLocal := 3
	if plan.profileName == "generic-rtp" {
		maxWindowLocal = 6
	}
	if limit > maxWindowLocal {
		limit = maxWindowLocal
	}
	if plan.segmentRetries > 0 && limit > plan.segmentRetries+1 {
		limit = plan.segmentRetries + 1
	}
	if global > 0 && limit > global {
		limit = global
	}
	if limit <= 0 {
		return 1
	}
	return limit
}

func logWindowRecoveryEvent(requestID, traceID, mappingID, mappingName, targetURL, method, stage, rangeHeader string, class windowRecoveryFailureClass, strategy windowRecoveryStrategy, resumeAttempts, segmentAttempt int, err error) {
	log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=%s target_url=%s method=%s range=%s failure_class=%s recovery_strategy=%s resume_attempts=%d segment_attempt=%d err=%v", requestID, traceID, mappingID, mappingName, stage, targetURL, method, rangeHeader, firstNonEmpty(strings.TrimSpace(string(class)), string(windowRecoveryFailureUnknown)), firstNonEmpty(strings.TrimSpace(string(strategy)), string(windowRecoveryStrategyAbortTransaction)), resumeAttempts, segmentAttempt, err)
}

func shouldResumeRTPResponseCopy(req *http.Request, prepared *mappingForwardRequest, resp *http.Response, err error, bytesWritten int64, resumeCount int) bool {
	if err == nil || req == nil || prepared == nil || resp == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(req.Method), http.MethodGet) {
		return false
	}
	if resumeCount >= boundaryResumeMaxAttempts() || bytesWritten <= 0 {
		return false
	}
	if !supportsSegmentedRangeFetch(req, resp) {
		return false
	}
	if classifyRecoverableRTPReadError(err) == "" {
		return false
	}
	if _, ok := computeResumeRange(resp, bytesWritten); !ok {
		return false
	}
	return true
}

func buildPreparedResumeRequest(prepared *mappingForwardRequest, resp *http.Response, bytesWritten int64) (*mappingForwardRequest, int64, string, error) {
	return buildPreparedResumeRequestWithLimit(prepared, resp, bytesWritten, nil)
}

// buildPreparedResumeRequestWithLimit 把“下一次恢复读取”严格限制在当前允许区间内。
// 当 fixed-window 正在搬运某个 segment 时，resumeEnd 就是该 segment 的闭区间 end；
// 一旦 nextStart 超出该 end，立即拒绝恢复，从根上阻断历史上出现过的超窗 copy。
func buildPreparedResumeRequestWithLimit(prepared *mappingForwardRequest, resp *http.Response, bytesWritten int64, resumeEnd *int64) (*mappingForwardRequest, int64, string, error) {
	nextStart, ok := computeResumeRange(resp, bytesWritten)
	if !ok {
		return nil, 0, "", fmt.Errorf("response is not resumable")
	}
	if resumeEnd != nil && *resumeEnd >= 0 && nextStart > *resumeEnd {
		return nil, 0, "", fmt.Errorf("resume start %d beyond limit %d", nextStart, *resumeEnd)
	}
	clone := *prepared
	clone.Headers = prepared.Headers.Clone()
	rangeHeader := fmt.Sprintf("bytes=%d-", nextStart)
	if resumeEnd != nil && *resumeEnd >= nextStart {
		rangeHeader = fmt.Sprintf("bytes=%d-%d", nextStart, *resumeEnd)
	}
	clone.Headers.Set("Range", rangeHeader)
	if strings.TrimSpace(resp.Header.Get("Content-Encoding")) == "" {
		clone.Headers.Set("Accept-Encoding", "identity")
	}
	if etag := strings.TrimSpace(resp.Header.Get("Etag")); etag != "" {
		clone.Headers.Set("If-Range", etag)
	} else if lastModified := strings.TrimSpace(resp.Header.Get("Last-Modified")); lastModified != "" {
		clone.Headers.Set("If-Range", lastModified)
	}
	clone.MaxResponseBodyBytes = effectivePreparedMaxResponseBodyBytes(&clone)
	return &clone, nextStart, rangeHeader, nil
}

// validatePreparedResumeResponseWithLimit 对 resume 回包做窗口约束校验：
// - start 必须与期望断点精确相等；
// - 若当前处于 fixed-window segment 内，则 end 不允许越过 segment end；
// - Content-Type 不能在恢复后悄悄变化。
func validatePreparedResumeResponseWithLimit(baseResp, resp *http.Response, expectedStart int64, resumeEnd *int64) error {
	if resp == nil {
		return fmt.Errorf("nil response")
	}
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("expected 206 partial content, got %d", resp.StatusCode)
	}
	cr, ok := parseContentRangeHeader(resp.Header.Get("Content-Range"))
	if !ok {
		return fmt.Errorf("missing or invalid content-range")
	}
	if cr.start != expectedStart {
		return fmt.Errorf("content-range start mismatch expected=%d got=%d", expectedStart, cr.start)
	}
	if resumeEnd != nil && *resumeEnd >= 0 && cr.end > *resumeEnd {
		return fmt.Errorf("content-range end overflow expected<=%d got=%d", *resumeEnd, cr.end)
	}
	if baseResp != nil {
		if ct0, ct1 := strings.TrimSpace(baseResp.Header.Get("Content-Type")), strings.TrimSpace(resp.Header.Get("Content-Type")); ct0 != "" && ct1 != "" && !strings.EqualFold(ct0, ct1) {
			return fmt.Errorf("content-type changed from %q to %q", ct0, ct1)
		}
	}
	return nil
}

func computeResumeRange(resp *http.Response, bytesWritten int64) (int64, bool) {
	if resp == nil || bytesWritten < 0 {
		return 0, false
	}
	if cr, ok := parseContentRangeHeader(resp.Header.Get("Content-Range")); ok {
		next := cr.start + bytesWritten
		if cr.hasEnd && next > cr.end {
			return 0, false
		}
		return next, true
	}
	if resp.StatusCode == http.StatusOK {
		if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Accept-Ranges")), "bytes") || resp.ContentLength > 0 {
			return bytesWritten, true
		}
	}
	return 0, false
}

func parseContentRangeHeader(raw string) (byteRangeSpec, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return byteRangeSpec{}, false
	}
	if !strings.HasPrefix(strings.ToLower(raw), "bytes ") {
		return byteRangeSpec{}, false
	}
	parts := strings.SplitN(strings.TrimSpace(raw[6:]), "/", 2)
	if len(parts) != 2 {
		return byteRangeSpec{}, false
	}
	rangePart := strings.TrimSpace(parts[0])
	totalPart := strings.TrimSpace(parts[1])
	bounds := strings.SplitN(rangePart, "-", 2)
	if len(bounds) != 2 {
		return byteRangeSpec{}, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(bounds[0]), 10, 64)
	if err != nil || start < 0 {
		return byteRangeSpec{}, false
	}
	end, err := strconv.ParseInt(strings.TrimSpace(bounds[1]), 10, 64)
	if err != nil || end < start {
		return byteRangeSpec{}, false
	}
	spec := byteRangeSpec{start: start, end: end, hasEnd: true}
	if totalPart != "*" {
		total, err := strconv.ParseInt(totalPart, 10, 64)
		if err == nil && total > 0 {
			spec.total = total
			spec.hasTotal = true
		}
	}
	return spec, true
}
