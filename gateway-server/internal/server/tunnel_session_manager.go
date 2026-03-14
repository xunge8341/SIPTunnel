package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type tunnelSessionRuntimeState struct {
	RegistrationStatus          string `json:"registration_status"`
	HeartbeatStatus             string `json:"heartbeat_status"`
	LastRegisterTime            string `json:"last_register_time"`
	LastHeartbeatTime           string `json:"last_heartbeat_time"`
	LastFailureReason           string `json:"last_failure_reason"`
	NextRetryTime               string `json:"next_retry_time"`
	ConsecutiveHeartbeatTimeout int    `json:"consecutive_heartbeat_timeout"`
}

type tunnelSessionActionRequest struct {
	Action string `json:"action"`
}

type tunnelSessionActionResponse struct {
	Action string                    `json:"action"`
	State  tunnelSessionRuntimeState `json:"state"`
}

type tunnelRegistrar interface {
	Register(ctx context.Context, authenticated bool) (int, string, error)
	Heartbeat(ctx context.Context) error
}

type tcpTunnelRegistrar struct {
	nodeStore nodeConfigStore
}

func (r tcpTunnelRegistrar) targetAddr() (string, error) {
	if r.nodeStore == nil {
		return "", fmt.Errorf("node store not configured")
	}
	peers := r.nodeStore.ListPeers()
	if len(peers) == 0 {
		return "", fmt.Errorf("peer node not configured")
	}
	peer := peers[0]
	host := strings.TrimSpace(peer.PeerSignalingIP)
	if host == "" || peer.PeerSignalingPort <= 0 {
		return "", fmt.Errorf("peer signaling endpoint not configured")
	}
	return fmt.Sprintf("%s:%d", host, peer.PeerSignalingPort), nil
}

func (r tcpTunnelRegistrar) probe(ctx context.Context) error {
	addr, err := r.targetAddr()
	if err != nil {
		return err
	}
	d := net.Dialer{Timeout: 1200 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial peer signaling %s: %w", addr, err)
	}
	_ = conn.Close()
	return nil
}

func (r tcpTunnelRegistrar) Register(ctx context.Context, authenticated bool) (int, string, error) {
	if !authenticated {
		return 401, "digest challenge required", nil
	}
	if err := r.probe(ctx); err != nil {
		return 0, "", err
	}
	return 200, "registered", nil
}

func (r tcpTunnelRegistrar) Heartbeat(ctx context.Context) error {
	return r.probe(ctx)
}

type tunnelSessionManager struct {
	registrar tunnelRegistrar

	mu                     sync.RWMutex
	heartbeatInterval      time.Duration
	registerRetryInterval  time.Duration
	registerRetryCount     int
	registrationStatus     string
	heartbeatStatus        string
	lastRegisterTime       time.Time
	lastHeartbeatTime      time.Time
	lastFailureReason      string
	nextRetryAt            time.Time
	consecutiveHBTimeouts  int
	registerAttempts       int
	authenticatedChallenge bool
	nextHeartbeatDue       time.Time
	closed                 chan struct{}
	wake                   chan struct{}
}

func newTunnelSessionManager(registrar tunnelRegistrar, cfg TunnelConfigPayload) *tunnelSessionManager {
	m := &tunnelSessionManager{
		registrar:             registrar,
		heartbeatInterval:     time.Duration(maxInt(1, cfg.HeartbeatIntervalSec)) * time.Second,
		registerRetryCount:    maxInt(0, cfg.RegisterRetryCount),
		registerRetryInterval: time.Duration(maxInt(1, cfg.RegisterRetryIntervalSec)) * time.Second,
		registrationStatus:    "unregistered",
		heartbeatStatus:       "unknown",
		closed:                make(chan struct{}),
		wake:                  make(chan struct{}, 1),
	}
	return m
}

func maxInt(min, v int) int {
	if v < min {
		return min
	}
	return v
}

func (m *tunnelSessionManager) Start() {
	go m.loop()
	m.TriggerRegister()
}

func (m *tunnelSessionManager) Close() error {
	select {
	case <-m.closed:
	default:
		close(m.closed)
	}
	return nil
}

func (m *tunnelSessionManager) loop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.closed:
			return
		case <-m.wake:
			m.tick()
		case <-ticker.C:
			m.tick()
		}
	}
}

func (m *tunnelSessionManager) tick() {
	m.mu.RLock()
	nextRetry := m.nextRetryAt
	nextHB := m.nextHeartbeatDue
	regStatus := m.registrationStatus
	hbStatus := m.heartbeatStatus
	interval := m.heartbeatInterval
	lastHB := m.lastHeartbeatTime
	m.mu.RUnlock()

	now := time.Now().UTC()
	if !nextRetry.IsZero() && !now.Before(nextRetry) {
		m.registerOnce()
		return
	}
	if regStatus == "registered" && !nextHB.IsZero() && !now.Before(nextHB) {
		m.heartbeatOnce()
		return
	}
	if regStatus == "registered" && !lastHB.IsZero() && now.Sub(lastHB) > interval*2 && hbStatus != "timeout" {
		m.mu.Lock()
		m.heartbeatStatus = "timeout"
		m.consecutiveHBTimeouts++
		m.lastFailureReason = "heartbeat timeout"
		m.mu.Unlock()
		m.scheduleRetry()
	}
}

func (m *tunnelSessionManager) scheduleRetry() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.registrationStatus = "failed"
	m.nextRetryAt = time.Now().UTC().Add(m.registerRetryInterval)
}

func (m *tunnelSessionManager) TriggerRegister() {
	m.mu.Lock()
	m.nextRetryAt = time.Now().UTC()
	m.mu.Unlock()
	m.signalWake()
}

func (m *tunnelSessionManager) TriggerReregister() { m.TriggerRegister() }

func (m *tunnelSessionManager) TriggerHeartbeat() {
	m.mu.Lock()
	m.nextHeartbeatDue = time.Now().UTC()
	m.mu.Unlock()
	m.signalWake()
}

func (m *tunnelSessionManager) signalWake() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *tunnelSessionManager) ApplyConfig(cfg TunnelConfigPayload) {
	m.mu.Lock()
	m.heartbeatInterval = time.Duration(maxInt(1, cfg.HeartbeatIntervalSec)) * time.Second
	m.registerRetryCount = maxInt(0, cfg.RegisterRetryCount)
	m.registerRetryInterval = time.Duration(maxInt(1, cfg.RegisterRetryIntervalSec)) * time.Second
	m.mu.Unlock()
}

func (m *tunnelSessionManager) registerOnce() {
	m.mu.Lock()
	m.registrationStatus = "registering"
	m.nextRetryAt = time.Time{}
	auth := m.authenticatedChallenge
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	code, reason, err := m.registrar.Register(ctx, auth)
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.registrationStatus = "failed"
		m.registerAttempts++
		m.lastFailureReason = err.Error()
		if m.registerAttempts <= m.registerRetryCount {
			m.nextRetryAt = now.Add(m.registerRetryInterval)
		}
		return
	}
	if code == 401 {
		m.authenticatedChallenge = true
		m.lastFailureReason = reason
		m.nextRetryAt = now
		return
	}
	if code != 200 {
		m.registrationStatus = "failed"
		m.lastFailureReason = fmt.Sprintf("unexpected register response code=%d", code)
		m.nextRetryAt = now.Add(m.registerRetryInterval)
		return
	}
	m.registrationStatus = "registered"
	m.heartbeatStatus = "healthy"
	m.lastRegisterTime = now
	m.lastFailureReason = ""
	m.registerAttempts = 0
	m.consecutiveHBTimeouts = 0
	m.nextHeartbeatDue = now.Add(m.heartbeatInterval)
}

func (m *tunnelSessionManager) heartbeatOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	err := m.registrar.Heartbeat(ctx)
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		m.heartbeatStatus = "timeout"
		m.consecutiveHBTimeouts++
		m.lastFailureReason = err.Error()
		m.registrationStatus = "failed"
		if m.registerAttempts < m.registerRetryCount {
			m.registerAttempts++
			m.nextRetryAt = now.Add(m.registerRetryInterval)
		}
		return
	}
	m.heartbeatStatus = "healthy"
	m.lastHeartbeatTime = now
	m.nextHeartbeatDue = now.Add(m.heartbeatInterval)
}

func (m *tunnelSessionManager) Snapshot() tunnelSessionRuntimeState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return tunnelSessionRuntimeState{
		RegistrationStatus:          m.registrationStatus,
		HeartbeatStatus:             m.heartbeatStatus,
		LastRegisterTime:            formatRFC3339(m.lastRegisterTime),
		LastHeartbeatTime:           formatRFC3339(m.lastHeartbeatTime),
		LastFailureReason:           m.lastFailureReason,
		NextRetryTime:               formatRFC3339(m.nextRetryAt),
		ConsecutiveHeartbeatTimeout: m.consecutiveHBTimeouts,
	}
}

func formatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
