package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"siptunnel/internal/nodeconfig"
)

type tunnelSessionRuntimeState struct {
	RegistrationStatus          string `json:"registration_status"`
	HeartbeatStatus             string `json:"heartbeat_status"`
	Phase                       string `json:"phase"`
	PhaseUpdatedAt              string `json:"phase_updated_at"`
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
	nodeStore       nodeConfigStore
	preferredPeerID func() string
}

func (r tcpTunnelRegistrar) targetAddr() (string, error) {
	if r.nodeStore == nil {
		return "", fmt.Errorf("node store not configured")
	}
	peers := r.nodeStore.ListPeers()
	if len(peers) == 0 {
		return "", fmt.Errorf("peer node not configured")
	}
	preferredID := ""
	if r.preferredPeerID != nil {
		preferredID = strings.TrimSpace(r.preferredPeerID())
	}
	var peer nodeconfig.PeerNodeConfig
	selected := false
	if preferredID != "" {
		for _, item := range peers {
			if item.Enabled && strings.EqualFold(strings.TrimSpace(item.PeerNodeID), preferredID) {
				peer = item
				selected = true
				break
			}
		}
	}
	if !selected {
		enabled := make([]nodeconfig.PeerNodeConfig, 0, len(peers))
		for _, item := range peers {
			if item.Enabled {
				enabled = append(enabled, item)
			}
		}
		if len(enabled) == 0 {
			return "", fmt.Errorf("no enabled peer node configured")
		}
		if len(enabled) > 1 {
			ids := make([]string, 0, len(enabled))
			for _, item := range enabled {
				ids = append(ids, item.PeerNodeID)
			}
			return "", fmt.Errorf("multiple enabled peer nodes configured (%s); current single-binding mode requires exactly one or an explicit peer binding", strings.Join(ids, ","))
		}
		peer = enabled[0]
	}
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
	connectionInitiator    string
	nextHeartbeatDue       time.Time
	phase                  string
	phaseUpdatedAt         time.Time
	sessionID              string
	closed                 chan struct{}
	wake                   chan struct{}
}

func newTunnelSessionManager(registrar tunnelRegistrar, cfg TunnelConfigPayload) *tunnelSessionManager {
	m := &tunnelSessionManager{
		registrar:             registrar,
		heartbeatInterval:     time.Duration(atLeastInt(1, cfg.HeartbeatIntervalSec)) * time.Second,
		registerRetryCount:    atLeastInt(0, cfg.RegisterRetryCount),
		registerRetryInterval: time.Duration(atLeastInt(1, cfg.RegisterRetryIntervalSec)) * time.Second,
		registrationStatus:    "unregistered",
		heartbeatStatus:       "unknown",
		connectionInitiator:   strings.ToUpper(strings.TrimSpace(cfg.ConnectionInitiator)),
		phase:                 "initializing",
		phaseUpdatedAt:        time.Now().UTC(),
		sessionID:             fmt.Sprintf("sess-%d", time.Now().UTC().UnixNano()),
		closed:                make(chan struct{}),
		wake:                  make(chan struct{}, 1),
	}
	return m
}

func atLeastInt(min, v int) int {
	if v < min {
		return min
	}
	return v
}

func (m *tunnelSessionManager) setPhaseLocked(phase string) {
	phase = strings.TrimSpace(phase)
	if phase == "" {
		return
	}
	m.phase = phase
	m.phaseUpdatedAt = time.Now().UTC()
}

func (m *tunnelSessionManager) Start() {
	go m.loop()
	m.mu.Lock()
	initiator := strings.ToUpper(strings.TrimSpace(m.connectionInitiator))
	if initiator == "" {
		initiator = "LOCAL"
		m.connectionInitiator = initiator
	}
	if initiator == "PEER" {
		m.registrationStatus = "waiting_peer"
		m.setPhaseLocked("waiting_peer")
		m.nextRetryAt = time.Time{}
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
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
	initiator := strings.ToUpper(strings.TrimSpace(m.connectionInitiator))
	m.mu.RUnlock()

	now := time.Now().UTC()
	if initiator == "PEER" && nextRetry.IsZero() {
		return
	}
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
		m.setPhaseLocked("retry_wait")
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
	m.setPhaseLocked("retry_wait")
	m.nextRetryAt = time.Now().UTC().Add(m.registerRetryInterval)
}

func (m *tunnelSessionManager) TriggerRegister() {
	m.mu.Lock()
	m.registrationStatus = "registering"
	m.setPhaseLocked("registering")
	m.nextRetryAt = time.Now().UTC()
	m.mu.Unlock()
	m.signalWake()
}

func (m *tunnelSessionManager) TriggerReregister() { m.TriggerRegister() }

func (m *tunnelSessionManager) TriggerHeartbeat() {
	m.mu.Lock()
	m.setPhaseLocked("heartbeat_due")
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
	m.heartbeatInterval = time.Duration(atLeastInt(1, cfg.HeartbeatIntervalSec)) * time.Second
	m.registerRetryCount = atLeastInt(0, cfg.RegisterRetryCount)
	m.registerRetryInterval = time.Duration(atLeastInt(1, cfg.RegisterRetryIntervalSec)) * time.Second
	m.connectionInitiator = strings.ToUpper(strings.TrimSpace(cfg.ConnectionInitiator))
	if m.connectionInitiator == "" {
		m.connectionInitiator = "LOCAL"
	}
	if m.connectionInitiator == "PEER" {
		m.nextRetryAt = time.Time{}
		if m.registrationStatus == "unregistered" || m.registrationStatus == "failed" || m.registrationStatus == "registering" {
			m.registrationStatus = "waiting_peer"
			m.setPhaseLocked("waiting_peer")
		}
	}
	m.mu.Unlock()
	if strings.EqualFold(strings.TrimSpace(cfg.ConnectionInitiator), "LOCAL") {
		m.TriggerRegister()
	}
}

func (m *tunnelSessionManager) registerOnce() {
	m.mu.Lock()
	m.registrationStatus = "registering"
	m.setPhaseLocked("registering")
	m.nextRetryAt = time.Time{}
	auth := m.authenticatedChallenge
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	start := time.Now()
	code, reason, err := m.registrar.Register(ctx, auth)
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		log.Printf("sip event=REGISTER session_id=%s status=error duration_ms=%d reason=%v", m.sessionID, time.Since(start).Milliseconds(), err)
		m.registrationStatus = "failed"
		m.setPhaseLocked("retry_wait")
		m.registerAttempts++
		m.lastFailureReason = err.Error()
		m.nextRetryAt = now.Add(m.registerRetryInterval)
		return
	}
	if code == 401 {
		log.Printf("sip event=REGISTER session_id=%s status=401 duration_ms=%d reason=%s", m.sessionID, time.Since(start).Milliseconds(), reason)
		m.authenticatedChallenge = true
		m.setPhaseLocked("auth_challenge")
		m.lastFailureReason = reason
		m.nextRetryAt = now
		return
	}
	if code != 200 {
		log.Printf("sip event=REGISTER session_id=%s status=%d duration_ms=%d reason=%s", m.sessionID, code, time.Since(start).Milliseconds(), reason)
		m.registrationStatus = "failed"
		m.setPhaseLocked("retry_wait")
		m.lastFailureReason = fmt.Sprintf("unexpected register response code=%d", code)
		m.nextRetryAt = now.Add(m.registerRetryInterval)
		return
	}
	log.Printf("sip event=REGISTER session_id=%s status=200 duration_ms=%d", m.sessionID, time.Since(start).Milliseconds())
	m.registrationStatus = "registered"
	m.setPhaseLocked("registered")
	log.Printf("sip event=MESSAGE session_id=%s status=ok duration_ms=%d", m.sessionID, time.Since(start).Milliseconds())
	log.Printf("sip event=MESSAGE session_id=%s status=ok duration_ms=%d", m.sessionID, time.Since(start).Milliseconds())
	m.heartbeatStatus = "healthy"
	m.lastRegisterTime = now
	m.lastFailureReason = ""
	m.registerAttempts = 0
	m.consecutiveHBTimeouts = 0
	m.nextHeartbeatDue = now.Add(m.heartbeatInterval)
	m.setPhaseLocked("heartbeat_ready")
}

func (m *tunnelSessionManager) heartbeatOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := m.registrar.Heartbeat(ctx)
	now := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		log.Printf("sip event=MESSAGE session_id=%s status=timeout duration_ms=%d reason=%v", m.sessionID, time.Since(start).Milliseconds(), err)
		m.heartbeatStatus = "timeout"
		m.setPhaseLocked("retry_wait")
		m.consecutiveHBTimeouts++
		m.lastFailureReason = err.Error()
		m.registrationStatus = "failed"
		m.setPhaseLocked("retry_wait")
		m.registerAttempts++
		m.nextRetryAt = now.Add(m.registerRetryInterval)
		return
	}
	m.heartbeatStatus = "healthy"
	m.setPhaseLocked("heartbeat_healthy")
	m.lastHeartbeatTime = now
	m.nextHeartbeatDue = now.Add(m.heartbeatInterval)
}

func (m *tunnelSessionManager) Snapshot() tunnelSessionRuntimeState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return tunnelSessionRuntimeState{
		RegistrationStatus:          m.registrationStatus,
		HeartbeatStatus:             m.heartbeatStatus,
		Phase:                       m.phase,
		PhaseUpdatedAt:              formatRFC3339(m.phaseUpdatedAt),
		LastRegisterTime:            formatRFC3339(m.lastRegisterTime),
		LastHeartbeatTime:           formatRFC3339(m.lastHeartbeatTime),
		LastFailureReason:           m.lastFailureReason,
		NextRetryTime:               formatRFC3339(m.nextRetryAt),
		ConsecutiveHeartbeatTimeout: m.consecutiveHBTimeouts,
	}
}

func formatRFC3339(t time.Time) string {
	return formatTimestamp(t)
}
