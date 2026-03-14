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
	"net/url"
	"strconv"
	"strings"
	"sync"
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
)

type mappingForwarder interface {
	PrepareForward(context.Context, tunnelmapping.TunnelMapping, *http.Request) (*mappingForwardRequest, error)
	ExecuteForward(context.Context, *mappingForwardRequest) (*http.Response, error)
}

type directHTTPMappingForwarder struct{}

// mappingForwardRequest 是监听层与转发策略之间的稳定请求抽象。
// direct 模式会把 HTTP 请求组装为上游请求；未来 SIP/RTP 隧道模式
// 可复用该结构补充 SIP 元信息、RTP 大载荷和流式响应控制参数。
type mappingForwardRequest struct {
	MappingID string
	Method    string
	TargetURL *url.URL
	Headers   http.Header
	Body      []byte

	ConnectTimeout        time.Duration
	RequestTimeout        time.Duration
	ResponseHeaderTimeout time.Duration
	MaxResponseBodyBytes  int64

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
	if !methodAllowed(mapping.AllowedMethods, req.Method) {
		return nil, fmt.Errorf("method %s is not allowed", req.Method)
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, mapping.MaxRequestBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if int64(len(body)) > mapping.MaxRequestBodyBytes {
		return nil, fmt.Errorf("request body exceeds max_request_body_bytes=%d", mapping.MaxRequestBodyBytes)
	}

	targetURL, err := buildTargetURL(mapping, req.URL)
	if err != nil {
		return nil, err
	}
	forwardHeaders := make(http.Header)
	copyHeaders(forwardHeaders, req.Header)
	trimmedHost := strings.TrimSpace(req.Host)
	if trimmedHost != "" {
		forwardHeaders.Set("X-Forwarded-Host", trimmedHost)
	}
	forwardHeaders.Set("X-Forwarded-Proto", normalizeScheme(req))
	forwardHeaders.Set("X-SIPTunnel-Mapping-ID", mapping.MappingID)

	return &mappingForwardRequest{
		MappingID:             mapping.MappingID,
		Method:                req.Method,
		TargetURL:             targetURL,
		Headers:               forwardHeaders,
		Body:                  body,
		ConnectTimeout:        time.Duration(mapping.ConnectTimeoutMS) * time.Millisecond,
		RequestTimeout:        time.Duration(mapping.RequestTimeoutMS) * time.Millisecond,
		ResponseHeaderTimeout: time.Duration(mapping.ResponseTimeoutMS) * time.Millisecond,
		MaxResponseBodyBytes:  mapping.MaxResponseBodyBytes,
	}, nil
}

func (directHTTPMappingForwarder) ExecuteForward(ctx context.Context, prepared *mappingForwardRequest) (*http.Response, error) {
	if prepared == nil {
		return nil, fmt.Errorf("nil prepared forward request")
	}

	outReq, err := http.NewRequestWithContext(ctx, prepared.Method, prepared.TargetURL.String(), bytes.NewReader(prepared.Body))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	copyHeaders(outReq.Header, prepared.Headers)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: prepared.ConnectTimeout}).DialContext,
			ResponseHeaderTimeout: prepared.ResponseHeaderTimeout,
		},
		Timeout: prepared.RequestTimeout,
	}

	resp, err := client.Do(outReq)
	if err != nil {
		return nil, fmt.Errorf("forward request to %s: %w", prepared.TargetURL.String(), err)
	}
	if prepared.MaxResponseBodyBytes > 0 {
		resp.Body = &limitedReadCloser{ReadCloser: resp.Body, limit: prepared.MaxResponseBodyBytes}
	}
	return resp, nil
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
	mu       sync.RWMutex
	listenFn func(network, address string) (net.Listener, error)
	forward  mappingForwarder
	runners  map[string]*mappingRuntimeRunner
	status   map[string]mappingRuntimeStatus
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
		if strings.Contains(strings.ToLower(err.Error()), "address already in use") {
			reason = fmt.Sprintf("端口冲突：本端入口 %s 已被占用，请调整 local_bind_port", endpoint)
		}
		m.status[item.MappingID] = mappingRuntimeStatus{State: mappingStateStartFailed, Reason: reason}
		return
	}

	r := &mappingRuntimeRunner{id: item.MappingID, endpoint: endpoint, mapping: item, listener: ln}
	r.server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		m.beginForward(r.id)
		prepared, err := m.forward.PrepareForward(req.Context(), r.mapping, req)
		if err != nil {
			m.endForward(r.id, false, "转发准备失败："+err.Error())
			log.Printf("mapping-runtime audit mapping=%s method=%s path=%s result=prepare_error err=%v", r.id, req.Method, req.URL.String(), err)
			http.Error(w, "请求转发准备失败："+err.Error(), http.StatusBadGateway)
			return
		}
		resp, err := m.forward.ExecuteForward(req.Context(), prepared)
		if err != nil {
			m.endForward(r.id, false, "转发失败："+err.Error())
			log.Printf("mapping-runtime audit mapping=%s method=%s path=%s result=error err=%v", r.id, req.Method, req.URL.String(), err)
			http.Error(w, "请求转发到对端失败："+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		if _, copyErr := io.Copy(w, resp.Body); copyErr != nil {
			m.endForward(r.id, false, "回传响应失败："+copyErr.Error())
			log.Printf("mapping-runtime audit mapping=%s method=%s path=%s status=%d result=copy_error err=%v", r.id, req.Method, req.URL.String(), resp.StatusCode, copyErr)
			return
		}
		elapsed := time.Since(start)
		m.endForward(r.id, true, fmt.Sprintf("最近转发成功：status=%d duration=%s", resp.StatusCode, elapsed.Round(time.Millisecond)))
		log.Printf("mapping-runtime audit mapping=%s method=%s path=%s status=%d duration_ms=%d", r.id, req.Method, req.URL.String(), resp.StatusCode, elapsed.Milliseconds())
	})}
	m.runners[item.MappingID] = r
	m.status[item.MappingID] = mappingRuntimeStatus{State: mappingStateListening, Reason: "监听中"}
	go func(id string, srv *http.Server, listener net.Listener) {
		err := srv.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			m.markInterrupted(id, fmt.Sprintf("监听异常中断：%v", err))
		}
	}(item.MappingID, r.server, r.listener)
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
	if base == "/" {
		return path, true
	}
	if path == base {
		return "", true
	}
	prefix := base + "/"
	if strings.HasPrefix(path, prefix) {
		return strings.TrimPrefix(path, base), true
	}
	return "", false
}

func normalizePath(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
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
}
