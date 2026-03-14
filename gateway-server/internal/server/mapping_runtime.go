package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"siptunnel/internal/tunnelmapping"
)

const (
	mappingStateDisabled    = "disabled"
	mappingStateListening   = "listening"
	mappingStateStartFailed = "start_failed"
	mappingStateInterrupted = "interrupted"
)

type mappingForwarder interface {
	PrepareForward(context.Context, tunnelmapping.TunnelMapping, *http.Request) error
}

type noopMappingForwarder struct{}

func (noopMappingForwarder) PrepareForward(context.Context, tunnelmapping.TunnelMapping, *http.Request) error {
	return nil
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
		forward = noopMappingForwarder{}
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
		if err := m.forward.PrepareForward(req.Context(), r.mapping, req); err != nil {
			http.Error(w, "请求已接收，但准备转发到对端失败："+err.Error(), http.StatusBadGateway)
			return
		}
		http.Error(w, "请求已接收，转发到对端链路待接入", http.StatusNotImplemented)
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
