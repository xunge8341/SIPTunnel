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
	Forward(context.Context, tunnelmapping.TunnelMapping, *http.Request) (*http.Response, error)
}

type directHTTPMappingForwarder struct{}

func newDirectHTTPMappingForwarder() mappingForwarder {
	return directHTTPMappingForwarder{}
}

func (directHTTPMappingForwarder) Forward(ctx context.Context, mapping tunnelmapping.TunnelMapping, req *http.Request) (*http.Response, error) {
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

	outReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	copyHeaders(outReq.Header, req.Header)
	trimmedHost := strings.TrimSpace(req.Host)
	if trimmedHost != "" {
		outReq.Header.Set("X-Forwarded-Host", trimmedHost)
	}
	outReq.Header.Set("X-Forwarded-Proto", normalizeScheme(req))
	outReq.Header.Set("X-SIPTunnel-Mapping-ID", mapping.MappingID)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: time.Duration(mapping.ConnectTimeoutMS) * time.Millisecond,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(mapping.ResponseTimeoutMS) * time.Millisecond,
		},
		Timeout: time.Duration(mapping.RequestTimeoutMS) * time.Millisecond,
	}

	resp, err := client.Do(outReq)
	if err != nil {
		return nil, fmt.Errorf("forward request to %s: %w", targetURL.String(), err)
	}
	if mapping.MaxResponseBodyBytes > 0 {
		resp.Body = &limitedReadCloser{ReadCloser: resp.Body, limit: mapping.MaxResponseBodyBytes}
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
		resp, err := m.forward.Forward(req.Context(), r.mapping, req)
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
