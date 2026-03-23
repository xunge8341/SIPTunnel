package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"siptunnel/internal/tunnelmapping"
)

const (
	mappingStateDisabled    = "disabled"
	mappingStateListening   = "listening"
	mappingStateForwarding  = "forwarding"
	mappingStateDegraded    = "degraded"
	mappingStateStartFailed = "start_failed"
	mappingStateInterrupted = "interrupted"

	internalRangeFetchHeader = "X-SIPTunnel-Internal-Range-Fetch"
	downloadProfileHeader    = "X-SIPTunnel-Download-Profile"
	playbackIntentHeader     = "X-SIPTunnel-Playback-Intent"
	downloadTransferIDHeader = "X-SIPTunnel-Download-Transfer-ID"
)

type windowRecoveryFailureClass string

type windowRecoveryStrategy string

const (
	windowRecoveryFailureUnknown           windowRecoveryFailureClass = "unknown"
	windowRecoveryFailureTimeout           windowRecoveryFailureClass = "timeout"
	windowRecoveryFailureOutOfWindow       windowRecoveryFailureClass = "out_of_window"
	windowRecoveryFailureOutOfOrder        windowRecoveryFailureClass = "out_of_order"
	windowRecoveryFailurePeerError         windowRecoveryFailureClass = "peer_error"
	windowRecoveryFailureRangeMismatch     windowRecoveryFailureClass = "range_mismatch"
	windowRecoveryFailureShortCopy         windowRecoveryFailureClass = "short_copy"
	windowRecoveryFailureSequenceGap       windowRecoveryFailureClass = "sequence_gap"
	windowRecoveryFailureThresholdExceeded windowRecoveryFailureClass = "threshold_exceeded"
)

const (
	windowRecoveryStrategyResumeWithinWindow windowRecoveryStrategy = "resume_within_window"
	windowRecoveryStrategyRestartWindow      windowRecoveryStrategy = "restart_window"
	windowRecoveryStrategyAbortTransaction   windowRecoveryStrategy = "abort_transaction"
)

// windowRecoveryError 把 fixed-window / resume 失败转成结构化语义：
// 1. class 标识失败根因，便于现场直接判断是越界、乱序、超时还是对端异常；
// 2. strategy 标识下一步动作，避免继续“无上限硬重试”；
// 3. 原始 err 仍通过 Unwrap 暴露，保证排障时可以看到底层细节。
//
// 工业要求下，错误不仅要失败，还必须“可归因、可追踪、可验证后续动作”。
// 这里的结构化错误就是为任务 8 的验收而加的最小稳定抽象。
// 它会贯穿 resume_plan / resume_failure / segment_strategy_switch 三类日志。
type windowRecoveryError struct {
	Stage          string
	Class          windowRecoveryFailureClass
	Strategy       windowRecoveryStrategy
	Range          string
	ResumeAttempts int
	SegmentAttempt int
	Err            error
}

func (e *windowRecoveryError) Error() string {
	if e == nil {
		return "window recovery error"
	}
	parts := []string{"window recovery"}
	if stage := strings.TrimSpace(e.Stage); stage != "" {
		parts = append(parts, "stage="+stage)
	}
	if class := strings.TrimSpace(string(e.Class)); class != "" {
		parts = append(parts, "class="+class)
	}
	if strategy := strings.TrimSpace(string(e.Strategy)); strategy != "" {
		parts = append(parts, "strategy="+strategy)
	}
	if r := strings.TrimSpace(e.Range); r != "" {
		parts = append(parts, "range="+r)
	}
	if e.ResumeAttempts > 0 {
		parts = append(parts, fmt.Sprintf("resume_attempts=%d", e.ResumeAttempts))
	}
	if e.SegmentAttempt > 0 {
		parts = append(parts, fmt.Sprintf("segment_attempt=%d", e.SegmentAttempt))
	}
	if e.Err != nil {
		parts = append(parts, "err="+strings.TrimSpace(e.Err.Error()))
	}
	return strings.Join(parts, " ")
}

func (e *windowRecoveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type mappingForwarder interface {
	PrepareForward(context.Context, tunnelmapping.TunnelMapping, *http.Request) (*mappingForwardRequest, error)
	ExecuteForward(context.Context, *mappingForwardRequest) (*http.Response, error)
}

type directHTTPMappingForwarder struct{}

// mappingForwardRequest 是监听层与转发策略之间的稳定请求抽象。
// direct 模式会把 HTTP 请求组装为上游请求；未来 SIP/RTP 隧道模式
// 可复用该结构补充 SIP 元信息、RTP 大载荷和流式响应控制参数。
type mappingForwardRequest struct {
	MappingID         string
	Mapping           tunnelmapping.TunnelMapping
	Method            string
	TargetURL         *url.URL
	Headers           http.Header
	Body              []byte
	BodyStream        io.ReadCloser
	BodyContentLength int64

	ConnectTimeout         time.Duration
	RequestTimeout         time.Duration
	ResponseHeaderTimeout  time.Duration
	MaxResponseBodyBytes   int64
	RetryEnabled           bool
	RetryAttempts          int
	AllowStreamingResponse bool

	// 预留字段：后续隧道策略可在 Prepare 阶段写入 SIP/RTP 编排所需的上下文。
	TunnelHint *mappingTunnelHint
}

// mappingTunnelHint 预留给未来 SIP/RTP 承载策略，不在 direct 模式中使用。
type mappingTunnelHint struct {
	SIPCallID string
}

func newDirectHTTPMappingForwarder() mappingForwarder {
	return directHTTPMappingForwarder{}
}

func (directHTTPMappingForwarder) PrepareForward(_ context.Context, mapping tunnelmapping.TunnelMapping, req *http.Request) (*mappingForwardRequest, error) {
	return prepareMappingForwardRequest(mapping, req, false)
}

func prepareMappingForwardRequest(mapping tunnelmapping.TunnelMapping, req *http.Request, bufferBody bool) (*mappingForwardRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	if !methodAllowed(mapping.AllowedMethods, req.Method) {
		return nil, fmt.Errorf("method %s is not allowed", req.Method)
	}
	targetURL, err := buildTargetURL(mapping, req.URL)
	if err != nil {
		return nil, err
	}
	forwardHeaders := make(http.Header)
	copyHeaders(forwardHeaders, req.Header)
	stripHopByHopHeaders(forwardHeaders)
	trimmedHost := strings.TrimSpace(req.Host)
	if trimmedHost != "" {
		forwardHeaders.Set("X-Forwarded-Host", trimmedHost)
	}
	if remoteIP := clientIP(req); strings.TrimSpace(remoteIP) != "" {
		appendXForwardedFor(forwardHeaders, remoteIP)
	}
	forwardHeaders.Set("X-Forwarded-Proto", normalizeScheme(req))
	forwardHeaders.Set("X-SIPTunnel-Mapping-ID", mapping.MappingID)
	forwardHeaders.Del("Host")

	prepared := &mappingForwardRequest{
		MappingID:             mapping.MappingID,
		Mapping:               mapping,
		Method:                req.Method,
		TargetURL:             targetURL,
		Headers:               forwardHeaders,
		BodyContentLength:     req.ContentLength,
		ConnectTimeout:        time.Duration(mapping.ConnectTimeoutMS) * time.Millisecond,
		RequestTimeout:        time.Duration(mapping.RequestTimeoutMS) * time.Millisecond,
		ResponseHeaderTimeout: time.Duration(mapping.ResponseTimeoutMS) * time.Millisecond,
		MaxResponseBodyBytes:  mapping.MaxResponseBodyBytes,
		RetryEnabled:          shouldEnablePreparedRetry(req, req.ContentLength),
		RetryAttempts:         1,
	}
	prepared.MaxResponseBodyBytes = effectivePreparedMaxResponseBodyBytes(prepared)

	maxBody := mapping.MaxRequestBodyBytes
	if maxBody <= 0 || req.Body == nil || req.Body == http.NoBody {
		prepared.BodyStream = http.NoBody
		prepared.BodyContentLength = 0
		if prepared.RetryEnabled {
			prepared.RetryAttempts = 2
		}
		return prepared, nil
	}
	if req.ContentLength > maxBody {
		_ = req.Body.Close()
		req.Body = http.NoBody
		return nil, fmt.Errorf("request body exceeds max_request_body_bytes=%d", mapping.MaxRequestBodyBytes)
	}
	bufferBody = bufferBody || shouldBufferRequestBodyForRetry(prepared.RetryEnabled, req.ContentLength, maxBody)
	if bufferBody {
		defer func() {
			_ = req.Body.Close()
			req.Body = http.NoBody
		}()
		body, err := io.ReadAll(io.LimitReader(req.Body, maxBody+1))
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		if int64(len(body)) > maxBody {
			return nil, fmt.Errorf("request body exceeds max_request_body_bytes=%d", mapping.MaxRequestBodyBytes)
		}
		prepared.Body = body
		prepared.BodyContentLength = int64(len(body))
		if prepared.RetryEnabled {
			prepared.RetryAttempts = 2
		}
		return prepared, nil
	}
	prepared.BodyStream = newLimitedBodyReadCloser(req.Body, maxBody)
	prepared.RetryEnabled = false
	prepared.RetryAttempts = 1
	return prepared, nil
}

func executePreparedForward(ctx context.Context, prepared *mappingForwardRequest) (*http.Response, error) {
	if prepared == nil {
		return nil, fmt.Errorf("nil prepared forward request")
	}
	if prepared.TargetURL == nil {
		return nil, fmt.Errorf("nil prepared target url")
	}

	guardKey := upstreamGuardKey(prepared)
	if err := defaultUpstreamCircuitGuard.Before(guardKey, prepared.TargetURL.String(), time.Now().UTC()); err != nil {
		return nil, formatPreparedForwardError(prepared, classifyUpstreamError(err, prepared.TargetURL), err)
	}

	dialTimeout := prepared.ConnectTimeout
	if dialTimeout <= 0 {
		dialTimeout = 5 * time.Second
	}
	requestTimeout := prepared.RequestTimeout
	if requestTimeout <= 0 {
		requestTimeout = 15 * time.Second
	}
	responseHeaderTimeout := prepared.ResponseHeaderTimeout
	if responseHeaderTimeout <= 0 {
		responseHeaderTimeout = requestTimeout
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if !prepared.AllowStreamingResponse {
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > requestTimeout {
			requestCtx, cancel = context.WithTimeout(ctx, requestTimeout)
			defer cancel()
		}
	}
	outReq, err := http.NewRequestWithContext(requestCtx, prepared.Method, prepared.TargetURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	if prepared.BodyStream != nil && prepared.BodyStream != http.NoBody {
		outReq.Body = prepared.BodyStream
		outReq.ContentLength = prepared.BodyContentLength
	} else if len(prepared.Body) > 0 {
		outReq.Body = io.NopCloser(bytes.NewReader(prepared.Body))
		outReq.ContentLength = int64(len(prepared.Body))
	} else {
		outReq.Body = http.NoBody
		outReq.ContentLength = 0
	}
	copyHeaders(outReq.Header, prepared.Headers)
	stripHopByHopHeaders(outReq.Header)
	outReq.Header.Del(internalRangeFetchHeader)
	outReq.Header.Del(downloadProfileHeader)
	outReq.Header.Del(playbackIntentHeader)
	// X-SIPTunnel-Download-Transfer-ID 必须沿着上级->下级的内部 HTTP 请求继续透传。
	// 下载公平分享已经改成按“外层下载事务”统计；如果这里把 header 删掉，下级就只能退回 target URL 兜底，
	// 多个外层下载同时命中同一资源时会被错误并桶，现场看到的 active_segments_transfer=4/5 就会再次出现。
	outReq.Host = prepared.TargetURL.Host

	client := cachedMappingHTTPClient(dialTimeout, responseHeaderTimeout)

	attempts := maxIntVal(1, prepared.RetryAttempts)
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := client.Do(outReq)
		if err == nil {
			defaultUpstreamCircuitGuard.RecordSuccess(guardKey)
			if prepared.MaxResponseBodyBytes > 0 {
				resp.Body = &limitedReadCloser{ReadCloser: resp.Body, limit: prepared.MaxResponseBodyBytes}
			}
			return resp, nil
		}
		info := classifyUpstreamError(err, prepared.TargetURL)
		defaultUpstreamCircuitGuard.RecordFailure(guardKey, info, time.Now().UTC())
		if attempt >= attempts || !shouldRetryPreparedForward(prepared, info, requestCtx) {
			return nil, formatPreparedForwardError(prepared, info, err)
		}
		select {
		case <-requestCtx.Done():
			return nil, formatPreparedForwardError(prepared, classifyUpstreamError(requestCtx.Err(), prepared.TargetURL), requestCtx.Err())
		case <-time.After(mappingRetryBackoff(attempt)):
		}
		outReq, err = http.NewRequestWithContext(requestCtx, prepared.Method, prepared.TargetURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("build upstream request: %w", err)
		}
		if len(prepared.Body) > 0 {
			outReq.Body = io.NopCloser(bytes.NewReader(prepared.Body))
			outReq.ContentLength = int64(len(prepared.Body))
		} else {
			outReq.Body = http.NoBody
			outReq.ContentLength = 0
		}
		copyHeaders(outReq.Header, prepared.Headers)
		stripHopByHopHeaders(outReq.Header)
		outReq.Host = prepared.TargetURL.Host
	}
	return nil, fmt.Errorf("mapping forward exhausted retries")
}

func (directHTTPMappingForwarder) ExecuteForward(ctx context.Context, prepared *mappingForwardRequest) (*http.Response, error) {
	return executePreparedForward(ctx, prepared)
}

type limitedReadCloser struct {
	io.ReadCloser
	read  int64
	limit int64
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	n, err := l.ReadCloser.Read(p)
	l.read += int64(n)
	if l.read > l.limit {
		return n, fmt.Errorf("response body exceeds max_response_body_bytes=%d", l.limit)
	}
	return n, err
}

type mappingRuntimeStatus struct {
	State  string
	Reason string
}

type mappingRuntimeRunner struct {
	id       string
	endpoint string
	mapping  tunnelmapping.TunnelMapping
	listener net.Listener
	server   *http.Server
	active   int
	stopping bool
}

type mappingRuntimeManager struct {
	mu                  sync.RWMutex
	listenFn            func(network, address string) (net.Listener, error)
	forward             mappingForwarder
	protector           requestProtector
	accessLogRecorder   func(AccessLogEntry)
	runners             map[string]*mappingRuntimeRunner
	status              map[string]mappingRuntimeStatus
	requestSeq          uint64
	recoveryFailedTotal atomic.Uint64
}

func newMappingRuntimeManager(forward mappingForwarder) *mappingRuntimeManager {
	if forward == nil {
		forward = newDirectHTTPMappingForwarder()
	}
	return &mappingRuntimeManager{
		listenFn: net.Listen,
		forward:  forward,
		runners:  map[string]*mappingRuntimeRunner{},
		status:   map[string]mappingRuntimeStatus{},
	}
}

func (m *mappingRuntimeManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, runner := range m.runners {
		runner.stopping = true
		_ = runner.server.Close()
		_ = runner.listener.Close()
	}
	m.runners = map[string]*mappingRuntimeRunner{}
	return nil
}

func (m *mappingRuntimeManager) SyncMappings(items []TunnelMapping) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		seen[item.MappingID] = struct{}{}
		m.syncOneLocked(item)
	}
	for id, runner := range m.runners {
		if _, ok := seen[id]; ok {
			continue
		}
		runner.stopping = true
		_ = runner.server.Close()
		_ = runner.listener.Close()
		delete(m.runners, id)
		delete(m.status, id)
	}
}

func (m *mappingRuntimeManager) Snapshot() map[string]mappingRuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]mappingRuntimeStatus, len(m.status))
	for k, v := range m.status {
		out[k] = v
	}
	return out
}

func (m *mappingRuntimeManager) syncOneLocked(item TunnelMapping) {
	runner, running := m.runners[item.MappingID]
	if !item.Enabled {
		if running {
			runner.stopping = true
			_ = runner.server.Close()
			_ = runner.listener.Close()
			delete(m.runners, item.MappingID)
		}
		m.status[item.MappingID] = mappingRuntimeStatus{State: mappingStateDisabled, Reason: "映射未启用"}
		return
	}

	endpoint := net.JoinHostPort(strings.TrimSpace(item.LocalBindIP), fmt.Sprintf("%d", item.LocalBindPort))
	for id, existing := range m.runners {
		if id == item.MappingID {
			continue
		}
		if existing.endpoint == endpoint {
			m.status[item.MappingID] = mappingRuntimeStatus{State: mappingStateStartFailed, Reason: fmt.Sprintf("端口冲突：本端入口 %s 已被映射 %s 占用", endpoint, id)}
			m.recoveryFailedTotal.Add(1)
			return
		}
	}

	if running {
		if runner.endpoint == endpoint {
			runner.mapping = item
			m.status[item.MappingID] = mappingRuntimeStatus{State: mappingStateListening, Reason: "监听中"}
			return
		}
		runner.stopping = true
		_ = runner.server.Close()
		_ = runner.listener.Close()
		delete(m.runners, item.MappingID)
	}

	ln, err := m.listenFn("tcp", endpoint)
	if err != nil {
		reason := fmt.Sprintf("启动监听失败：%v", err)
		if errors.Is(err, net.ErrClosed) {
			reason = "启动监听失败：监听器已关闭"
		}
		if strings.Contains(strings.ToLower(err.Error()), "address already in use") || strings.Contains(strings.ToLower(err.Error()), "only one usage of each socket address") {
			reason = fmt.Sprintf("端口冲突：本端入口 %s 已被占用，请调整 local_bind_port", endpoint)
		}
		m.status[item.MappingID] = mappingRuntimeStatus{State: mappingStateStartFailed, Reason: reason}
		m.recoveryFailedTotal.Add(1)
		return
	}
	ln = newTunedTCPListener(ln)

	r := &mappingRuntimeRunner{id: item.MappingID, endpoint: endpoint, mapping: item, listener: ln}
	r.server = &http.Server{
		Handler: wrapHTTPRecovery("mapping-runtime", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if shouldDisableHTTPKeepAlivesForRuntime("mapping-runtime") {
				w.Header().Set("Connection", "close")
			}
			start := time.Now()
			requestID := fmt.Sprintf("mreq-%d", atomic.AddUint64(&m.requestSeq, 1))
			traceID := strings.TrimSpace(req.Header.Get("X-Trace-ID"))
			reqContentLength := mappingRequestContentLength(req)
			mappingName := strings.TrimSpace(r.mapping.Name)
			if mappingName == "" {
				mappingName = r.id
			}
			remoteIP := clientIP(req)
			latencyTracker := newMappingNodeLatencyTracker(requestID, traceID, r.id, mappingName, mappingTargetEndpoint(r.mapping), req.Method)
			latencyTracker.MarkPrepared()
			if shouldLogMappingRequestPlan(req, r.mapping) {
				log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=request_start forwarder=%s local_endpoint=%s local_base_path=%s target=%s method=%s path=%s content_length=%d max_request_body_bytes=%d max_response_body_bytes=%d response_mode=%s remote_ip=%s", requestID, traceID, r.id, mappingName, mappingForwarderModeName(m.forward), mappingLocalEndpoint(r.mapping), strings.TrimSpace(r.mapping.LocalBasePath), mappingTargetEndpoint(r.mapping), req.Method, req.URL.String(), reqContentLength, r.mapping.MaxRequestBodyBytes, r.mapping.MaxResponseBodyBytes, strings.TrimSpace(r.mapping.ResponseMode), remoteIP)
			}
			if isBrowserAncillaryRequest(req) {
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusNoContent)
				elapsed := time.Since(start)
				m.recordAccessLog(AccessLogEntry{ID: requestID, OccurredAt: formatTimestamp(time.Now().UTC()), MappingName: mappingName, SourceIP: remoteIP, Method: req.Method, Path: req.URL.RequestURI(), StatusCode: http.StatusNoContent, DurationMS: elapsed.Milliseconds(), RequestID: requestID, TraceID: traceID})
				log.Printf("mapping-runtime request_id=%s mapping=%s direction=local method=%s path=%s status=%d reason=browser_probe_suppressed duration_ms=%d", requestID, r.id, req.Method, req.URL.String(), http.StatusNoContent, elapsed.Milliseconds())
				return
			}

			var release func()
			if m.protector != nil {
				permitRelease, protectErr := m.protector.Acquire(r.id, remoteIP)
				if protectErr != nil {
					m.endForward(r.id, false, "入口保护触发："+protectErr.Error())
					statusCode := http.StatusTooManyRequests
					if strings.Contains(strings.ToLower(protectErr.Error()), "concurrency") {
						statusCode = http.StatusServiceUnavailable
					}
					m.recordAccessLog(AccessLogEntry{ID: requestID, OccurredAt: formatTimestamp(time.Now().UTC()), MappingName: mappingName, SourceIP: remoteIP, Method: req.Method, Path: req.URL.RequestURI(), StatusCode: statusCode, DurationMS: time.Since(start).Milliseconds(), FailureReason: protectErr.Error(), RequestID: requestID, TraceID: traceID})
					log.Printf("mapping-runtime request_id=%s mapping=%s direction=local method=%s path=%s result=protection_reject err=%v", requestID, r.id, req.Method, req.URL.String(), protectErr)
					http.Error(w, "入口保护触发："+protectErr.Error(), statusCode)
					return
				}
				release = permitRelease
				defer release()
			}

			m.beginForward(r.id)
			req = req.WithContext(withMappingNodeLatencyTracker(req.Context(), latencyTracker))
			prepared, err := m.forward.PrepareForward(req.Context(), r.mapping, req)
			if err != nil {
				m.endForward(r.id, false, "转发准备失败(upstream)："+err.Error())
				m.recordAccessLog(AccessLogEntry{ID: requestID, OccurredAt: formatTimestamp(time.Now().UTC()), MappingName: mappingName, SourceIP: remoteIP, Method: req.Method, Path: req.URL.RequestURI(), StatusCode: http.StatusBadGateway, DurationMS: time.Since(start).Milliseconds(), FailureReason: err.Error(), RequestID: requestID, TraceID: traceID})
				if shouldLogMappingFailure(r.id+"|prepare|"+strings.ToLower(strings.TrimSpace(err.Error())), time.Now()) {
					log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=prepare_error forwarder=%s local_endpoint=%s target=%s method=%s path=%s content_length=%d max_request_body_bytes=%d max_response_body_bytes=%d response_mode=%s err=%v", requestID, traceID, r.id, mappingName, mappingForwarderModeName(m.forward), mappingLocalEndpoint(r.mapping), mappingTargetEndpoint(r.mapping), req.Method, req.URL.String(), reqContentLength, r.mapping.MaxRequestBodyBytes, r.mapping.MaxResponseBodyBytes, strings.TrimSpace(r.mapping.ResponseMode), err)
				}
				http.Error(w, "请求转发准备失败："+err.Error(), http.StatusBadGateway)
				return
			}
			if maybeRewriteOpenEndedRangeForAdaptivePlayback(prepared) {
				log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=range_rewrite target_url=%s method=%s effective_range=%s reason=%s", requestID, traceID, r.id, mappingName, prepared.TargetURL.String(), prepared.Method, strings.TrimSpace(prepared.Headers.Get("Range")), strings.TrimSpace(prepared.Headers.Get(adaptiveRangeRewriteReasonHeader)))
			}
			if shouldLogMappingRequestPlan(req, r.mapping) {
				log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=prepare_ok forwarder=%s target_url=%s body_mode=%s prepared_body_bytes=%d retry_enabled=%t retry_attempts=%d connect_timeout_ms=%d request_timeout_ms=%d response_timeout_ms=%d max_response_body_bytes=%d effective_range=%s", requestID, traceID, r.id, mappingName, mappingForwarderModeName(m.forward), prepared.TargetURL.String(), mappingPreparedBodyMode(prepared), mappingPreparedBodyBytes(prepared), prepared.RetryEnabled, prepared.RetryAttempts, prepared.ConnectTimeout.Milliseconds(), prepared.RequestTimeout.Milliseconds(), prepared.ResponseHeaderTimeout.Milliseconds(), prepared.MaxResponseBodyBytes, strings.TrimSpace(prepared.Headers.Get("Range")))
			}
			resp, err := m.forward.ExecuteForward(req.Context(), prepared)
			if err != nil {
				m.endForward(r.id, false, "转发失败(upstream)："+err.Error())
				m.recordAccessLog(AccessLogEntry{ID: requestID, OccurredAt: formatTimestamp(time.Now().UTC()), MappingName: mappingName, SourceIP: remoteIP, Method: req.Method, Path: req.URL.RequestURI(), StatusCode: http.StatusBadGateway, DurationMS: time.Since(start).Milliseconds(), FailureReason: err.Error(), RequestID: requestID, TraceID: traceID})
				if shouldLogMappingFailure(r.id+"|execute|"+strings.ToLower(strings.TrimSpace(err.Error())), time.Now()) {
					log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=execute_error forwarder=%s target_url=%s body_mode=%s prepared_body_bytes=%d method=%s path=%s err=%v", requestID, traceID, r.id, mappingName, mappingForwarderModeName(m.forward), prepared.TargetURL.String(), mappingPreparedBodyMode(prepared), mappingPreparedBodyBytes(prepared), req.Method, req.URL.String(), err)
				}
				http.Error(w, "请求转发到对端失败："+err.Error(), http.StatusBadGateway)
				return
			}
			latencyTracker.MarkResponseReady()
			resp.Body = &latencyTrackingReadCloser{ReadCloser: resp.Body, tracker: latencyTracker}
			defer resp.Body.Close()
			copyHeaders(w.Header(), resp.Header)
			stripHopByHopHeaders(w.Header())
			transportMode := strings.TrimSpace(resp.Header.Get("X-Siptunnel-Response-Mode"))
			entryPath := accessLogEntryPath(req)
			w.WriteHeader(resp.StatusCode)
			copyBuf := acquireCopyBuffer()
			defer releaseCopyBuffer(copyBuf)
			bytesWritten, copyErr := copyForwardResponseAdaptive(req.Context(), m.forward, prepared, req, resp, &latencyTrackingWriter{Writer: w, tracker: latencyTracker}, copyBuf, requestID, traceID, r.id, mappingName)
			latencyTracker.Finalize(req, prepared, resp, copyErr)
			if copyErr != nil {
				elapsed := time.Since(start)
				m.endForward(r.id, false, "回传响应失败(downstream)："+copyErr.Error())
				m.recordAccessLog(AccessLogEntry{ID: requestID, OccurredAt: formatTimestamp(time.Now().UTC()), MappingName: mappingName, SourceIP: remoteIP, Method: req.Method, Path: entryPath, StatusCode: resp.StatusCode, DurationMS: elapsed.Milliseconds(), FailureReason: copyErr.Error(), RequestID: requestID, TraceID: traceID})
				if shouldLogMappingFailure(r.id+"|copy|"+strings.ToLower(strings.TrimSpace(copyErr.Error())), time.Now()) {
					log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=copy_error forwarder=%s target_url=%s method=%s path=%s entry_path=%s transport=%s bytes=%d status=%d err=%v", requestID, traceID, r.id, mappingName, mappingForwarderModeName(m.forward), prepared.TargetURL.String(), req.Method, req.URL.String(), entryPath, firstNonEmpty(transportMode, "UNKNOWN"), bytesWritten, resp.StatusCode, copyErr)
				}
				return
			}
			elapsed := time.Since(start)
			m.endForward(r.id, true, fmt.Sprintf("最近转发成功：status=%d duration=%s", resp.StatusCode, elapsed.Round(time.Millisecond)))
			if !shouldSuppressAccessLog(req, resp.StatusCode, elapsed) {
				m.recordAccessLog(AccessLogEntry{ID: requestID, OccurredAt: formatTimestamp(time.Now().UTC()), MappingName: mappingName, SourceIP: remoteIP, Method: req.Method, Path: entryPath, StatusCode: resp.StatusCode, DurationMS: elapsed.Milliseconds(), FailureReason: "", RequestID: requestID, TraceID: traceID})
			}
			if shouldLogMappingSuccess(elapsed, resp.StatusCode) {
				log.Printf("mapping-runtime request_id=%s trace_id=%s mapping=%s mapping_name=%s stage=response_sent forwarder=%s target_url=%s method=%s path=%s entry_path=%s transport=%s bytes=%d status=%d duration_ms=%d", requestID, traceID, r.id, mappingName, mappingForwarderModeName(m.forward), prepared.TargetURL.String(), req.Method, req.URL.String(), entryPath, firstNonEmpty(transportMode, "UNKNOWN"), bytesWritten, resp.StatusCode, elapsed.Milliseconds())
			}
		})),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      0,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	ApplyRuntimeHTTPMitigations("mapping-runtime", r.server)
	m.runners[item.MappingID] = r
	m.status[item.MappingID] = mappingRuntimeStatus{State: mappingStateListening, Reason: "监听中"}
	go func(id string, srv *http.Server, listener net.Listener) {
		err := srv.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.markInterrupted(id, fmt.Sprintf("监听异常中断：%v", err))
		}
	}(item.MappingID, r.server, r.listener)
}

func shouldLogMappingSuccess(elapsed time.Duration, statusCode int) bool {
	if statusCode >= http.StatusBadRequest {
		return true
	}
	return elapsed >= time.Second
}

func sameOriginRefererPath(req *http.Request) string {
	if req == nil {
		return ""
	}
	referer := strings.TrimSpace(req.Header.Get("Referer"))
	if referer == "" {
		return ""
	}
	refURL, err := url.Parse(referer)
	if err != nil || refURL.Path == "" {
		return ""
	}
	if !strings.EqualFold(refURL.Host, req.Host) {
		return ""
	}
	return normalizePath(refURL.Path)
}

func isBrowserSubrequest(req *http.Request) bool {
	if req == nil {
		return false
	}
	if sameOriginRefererPath(req) == "" {
		return false
	}
	secFetchDest := strings.ToLower(strings.TrimSpace(req.Header.Get("Sec-Fetch-Dest")))
	if secFetchDest != "" && secFetchDest != "document" {
		return true
	}
	accept := strings.ToLower(strings.TrimSpace(req.Header.Get("Accept")))
	for _, token := range []string{"text/css", "javascript", "image/", "font/", "text/html", "application/json", "text/plain"} {
		if strings.Contains(accept, token) {
			return true
		}
	}
	return false
}

func accessLogEntryPath(req *http.Request) string {
	if req == nil || req.URL == nil {
		return "/"
	}
	if refPath := sameOriginRefererPath(req); refPath != "" {
		return refPath
	}
	return req.URL.RequestURI()
}

func shouldSuppressAccessLog(req *http.Request, status int, elapsed time.Duration) bool {
	if req == nil {
		return false
	}
	if status >= 400 || elapsed >= 1500*time.Millisecond {
		return false
	}
	if isHealthProbeRequest(req) {
		return true
	}
	return isBrowserSubrequest(req)
}

func isHealthProbeRequest(req *http.Request) bool {
	if req == nil {
		return false
	}
	return isHealthProbePath(req.URL)
}

func isBrowserAncillaryRequest(req *http.Request) bool {
	if req == nil {
		return false
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(req.URL.Path))
	switch path {
	case "/favicon.ico", "/favicon.svg", "/robots.txt", "/apple-touch-icon.png", "/apple-touch-icon-precomposed.png", "/site.webmanifest", "/browserconfig.xml":
	default:
		return false
	}
	ua := strings.ToLower(strings.TrimSpace(req.Header.Get("User-Agent")))
	accept := strings.ToLower(strings.TrimSpace(req.Header.Get("Accept")))
	secFetchDest := strings.ToLower(strings.TrimSpace(req.Header.Get("Sec-Fetch-Dest")))
	if strings.Contains(ua, "mozilla") || strings.Contains(accept, "image/") || secFetchDest == "image" || secFetchDest == "empty" {
		return true
	}
	return false
}

func (m *mappingRuntimeManager) beginForward(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runner, ok := m.runners[id]
	if !ok {
		return
	}
	runner.active++
	m.status[id] = mappingRuntimeStatus{State: mappingStateForwarding, Reason: "转发中"}
}

func (m *mappingRuntimeManager) endForward(id string, ok bool, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runner, exists := m.runners[id]
	if !exists {
		return
	}
	if runner.active > 0 {
		runner.active--
	}
	if runner.active > 0 {
		m.status[id] = mappingRuntimeStatus{State: mappingStateForwarding, Reason: "转发中"}
		return
	}
	if ok {
		m.status[id] = mappingRuntimeStatus{State: mappingStateListening, Reason: reason}
		return
	}
	m.status[id] = mappingRuntimeStatus{State: mappingStateDegraded, Reason: reason}
}

func buildTargetURL(mapping tunnelmapping.TunnelMapping, in *url.URL) (*url.URL, error) {
	localBase := normalizePath(mapping.LocalBasePath)
	remoteBase := normalizePath(mapping.RemoteBasePath)
	inPath := normalizePath(in.Path)
	suffix, ok := pathSuffix(localBase, inPath)
	if !ok {
		return nil, fmt.Errorf("path %s does not match local_base_path %s", in.Path, localBase)
	}
	remotePath := strings.TrimSuffix(remoteBase, "/") + suffix
	return &url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort(strings.TrimSpace(mapping.RemoteTargetIP), strconv.Itoa(mapping.RemoteTargetPort)),
		Path:     remotePath,
		RawQuery: in.RawQuery,
	}, nil
}

func pathSuffix(base, path string) (string, bool) {
	base = normalizePath(base)
	path = normalizePath(path)
	if base == "/" {
		if isEntryDocumentPath(path) {
			return "", true
		}
		return path, true
	}
	if path == base || isBaseEntryDocumentPath(base, path) {
		return "", true
	}
	// 入口选错补救：当前运行态仍采用“单映射单入口端口”模型。
	// 若运维或播放器只打开了入口根路径（/、/index.html），而映射实际挂在非根 local_base_path，
	// 这里直接把根路径视为该映射的入口首页；同时把 /base/、/base/index.html、/base/default.html
	// 这类默认入口文档也统一折叠回 base 本身，避免“端口对了，只是入口文档名不同”继续打成路径不匹配。
	if isEntryDocumentPath(path) {
		return "", true
	}
	prefix := base + "/"
	if strings.HasPrefix(path, prefix) {
		suffix := strings.TrimPrefix(path, base)
		if isEntryDocumentPath(suffix) {
			return "", true
		}
		return normalizePath(suffix), true
	}
	return "", false
}

func isEntryDocumentPath(p string) bool {
	normalized := strings.ToLower(strings.TrimSpace(p))
	switch normalized {
	case "", "/", "/index.html", "/index.htm", "/default.html", "/default.htm", "/home.html", "/home.htm":
		return true
	default:
		return false
	}
}

func isBaseEntryDocumentPath(base, p string) bool {
	if base == "/" {
		return isEntryDocumentPath(p)
	}
	normalized := strings.ToLower(strings.TrimSpace(p))
	base = strings.ToLower(strings.TrimSpace(base))
	for _, suffix := range []string{"/", "/index.html", "/index.htm", "/default.html", "/default.htm", "/home.html", "/home.htm"} {
		if normalized == base+suffix {
			return true
		}
	}
	return false
}

func (m *mappingRuntimeManager) SetProtector(protector requestProtector) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.protector = protector
}

func (m *mappingRuntimeManager) SetAccessLogRecorder(recorder func(AccessLogEntry)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accessLogRecorder = recorder
}

func (m *mappingRuntimeManager) recordAccessLog(entry AccessLogEntry) {
	m.mu.RLock()
	recorder := m.accessLogRecorder
	m.mu.RUnlock()
	if recorder != nil {
		recorder(entry)
	}
}

func clientIP(req *http.Request) string {
	if ip := strings.TrimSpace(requestClientIP(req)); ip != "" {
		return ip
	}
	return "unknown"
}

func normalizePath(v string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(v, "\\", "/"))
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	trimmed = path.Clean(trimmed)
	if trimmed == "." || trimmed == "" {
		return "/"
	}
	if len(trimmed) > 1 {
		trimmed = strings.TrimSuffix(trimmed, "/")
	}
	return trimmed
}

func methodAllowed(allowed []string, method string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, m := range allowed {
		v := strings.ToUpper(strings.TrimSpace(m))
		if v == "*" || v == strings.ToUpper(method) {
			return true
		}
	}
	return false
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func stripHopByHopHeaders(h http.Header) {
	if h == nil {
		return
	}
	connectionValue := h.Get("Connection")
	for _, token := range strings.Split(connectionValue, ",") {
		if trimmed := textproto.TrimString(token); trimmed != "" {
			h.Del(trimmed)
		}
	}
	for _, key := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Transfer-Encoding", "Upgrade", "Trailer", "TE", "Proxy-Authenticate", "Proxy-Authorization"} {
		h.Del(key)
	}
}

func appendXForwardedFor(h http.Header, ip string) {
	ip = strings.TrimSpace(ip)
	if h == nil || ip == "" {
		return
	}
	existing := strings.TrimSpace(h.Get("X-Forwarded-For"))
	if existing == "" {
		h.Set("X-Forwarded-For", ip)
		return
	}
	h.Set("X-Forwarded-For", existing+", "+ip)
}

func normalizeScheme(req *http.Request) string {
	if req.TLS != nil {
		return "https"
	}
	if req.URL != nil && strings.TrimSpace(req.URL.Scheme) != "" {
		return req.URL.Scheme
	}
	return "http"
}

func (m *mappingRuntimeManager) markInterrupted(id, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	runner, ok := m.runners[id]
	if !ok {
		return
	}
	if runner.stopping {
		return
	}
	delete(m.runners, id)
	m.status[id] = mappingRuntimeStatus{State: mappingStateInterrupted, Reason: reason}
	m.recoveryFailedTotal.Add(1)
}
