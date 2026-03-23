package server

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type gb28181PeerView struct {
	DeviceID              string `json:"device_id"`
	RemoteAddr            string `json:"remote_addr"`
	CallbackAddr          string `json:"callback_addr"`
	Transport             string `json:"transport"`
	LastRegisterAt        string `json:"last_register_at,omitempty"`
	RegisterExpiresAt     string `json:"register_expires_at,omitempty"`
	LastKeepaliveAt       string `json:"last_keepalive_at,omitempty"`
	SubscribedAt          string `json:"subscribed_at,omitempty"`
	SubscriptionExpiresAt string `json:"subscription_expires_at,omitempty"`
	LastCatalogNotifyAt   string `json:"last_catalog_notify_at,omitempty"`
	AuthRequired          bool   `json:"auth_required"`
	LastError             string `json:"last_error,omitempty"`
}

type gb28181PendingView struct {
	CallID       string `json:"call_id"`
	DeviceID     string `json:"device_id"`
	MappingID    string `json:"mapping_id"`
	ResponseMode string `json:"response_mode"`
	Stage        string `json:"stage,omitempty"`
	LastStageAt  string `json:"last_stage_at,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
}

type gb28181InboundView struct {
	CallID        string `json:"call_id"`
	DeviceID      string `json:"device_id"`
	MappingID     string `json:"mapping_id"`
	CallbackAddr  string `json:"callback_addr"`
	Transport     string `json:"transport"`
	RemoteRTPIP   string `json:"remote_rtp_ip"`
	RemoteRTPPort int    `json:"remote_rtp_port"`
	Stage         string `json:"stage,omitempty"`
	LastStageAt   string `json:"last_stage_at,omitempty"`
	LastError     string `json:"last_error,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	LastInvokeAt  string `json:"last_invoke_at,omitempty"`
}

type gb28181CatalogState struct {
	ResourceTotal int `json:"resource_total"`
	ExposedTotal  int `json:"exposed_total"`
}

type GB28181Snapshot struct {
	Peers     []gb28181PeerView    `json:"peers"`
	Pending   []gb28181PendingView `json:"pending_sessions"`
	Inbound   []gb28181InboundView `json:"inbound_sessions"`
	Catalog   gb28181CatalogState  `json:"catalog"`
	UpdatedAt string               `json:"updated_at"`
}

func latestNonZeroTime(values ...time.Time) time.Time {
	var latest time.Time
	for _, item := range values {
		if item.IsZero() {
			continue
		}
		if latest.IsZero() || item.After(latest) {
			latest = item
		}
	}
	return latest
}

func (s *GB28181TunnelService) pruneRuntimeLocked(now time.Time) {
	for deviceID, peer := range s.peers {
		if peer == nil {
			delete(s.peers, deviceID)
			continue
		}
		expiredRegister := !peer.registerExpiresAt.IsZero() && now.After(peer.registerExpiresAt.Add(2*time.Minute))
		expiredSubscribe := !peer.subscriptionExpiresAt.IsZero() && now.After(peer.subscriptionExpiresAt.Add(2*time.Minute))
		latest := latestNonZeroTime(peer.lastRegisterAt, peer.lastKeepaliveAt, peer.subscribedAt, peer.lastCatalogNotifyAt, peer.registerExpiresAt, peer.subscriptionExpiresAt)
		staleInactive := !latest.IsZero() && now.After(latest.Add(30*time.Minute))
		if (expiredRegister && expiredSubscribe) || staleInactive {
			delete(s.peers, deviceID)
		}
	}
	for key, state := range s.subscriptions {
		if strings.TrimSpace(state.remoteTarget) == "" || strings.TrimSpace(state.callID) == "" {
			delete(s.subscriptions, key)
		}
	}
	for callID, pending := range s.pending {
		if pending == nil {
			delete(s.pending, callID)
			continue
		}
		latest := latestNonZeroTime(pending.lastStageAt, pending.startedAt)
		if !latest.IsZero() && now.After(latest.Add(15*time.Minute)) {
			delete(s.pending, callID)
		}
	}
	for callID, inbound := range s.inbound {
		if inbound == nil {
			delete(s.inbound, callID)
			continue
		}
		latest := latestNonZeroTime(inbound.lastStageAt, inbound.startedAt, inbound.lastInvokeAt)
		if !latest.IsZero() && now.After(latest.Add(15*time.Minute)) {
			if inbound.rtpSender != nil {
				_ = inbound.rtpSender.Close()
			}
			delete(s.inbound, callID)
		}
	}
}

func (s *GB28181TunnelService) currentConfig() TunnelConfigPayload {
	if s == nil || s.config == nil {
		return defaultTunnelConfigPayload("")
	}
	return normalizeTunnelConfigPayload(s.config(), "")
}

func (s *GB28181TunnelService) subscriptionLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.renewCatalogSubscriptions()
	}
}

func (s *GB28181TunnelService) renewCatalogSubscriptions() {
	if s == nil {
		return
	}
	now := time.Now().UTC()
	var renew []gb28181PeerView
	s.mu.Lock()
	for _, peer := range s.peers {
		if peer == nil || strings.TrimSpace(peer.callbackAddr) == "" {
			continue
		}
		if !peer.subscriptionExpiresAt.IsZero() && now.Add(90*time.Second).Before(peer.subscriptionExpiresAt) {
			continue
		}
		renew = append(renew, gb28181PeerView{DeviceID: peer.deviceID, CallbackAddr: peer.callbackAddr, Transport: peer.transport})
	}
	s.mu.Unlock()
	for _, item := range renew {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s.ensureCatalogSubscribe(ctx, item.CallbackAddr, item.Transport, item.DeviceID)
		cancel()
	}
}

func (s *GB28181TunnelService) Snapshot() GB28181Snapshot {
	now := time.Now().UTC()
	out := GB28181Snapshot{UpdatedAt: formatTimestamp(now)}
	if s == nil {
		return out
	}
	s.mu.Lock()
	s.pruneRuntimeLocked(now)
	defer s.mu.Unlock()
	for _, peer := range s.peers {
		if peer == nil {
			continue
		}
		out.Peers = append(out.Peers, gb28181PeerView{
			DeviceID:              peer.deviceID,
			RemoteAddr:            peer.remoteAddr,
			CallbackAddr:          peer.callbackAddr,
			Transport:             peer.transport,
			LastRegisterAt:        formatTimestamp(peer.lastRegisterAt),
			RegisterExpiresAt:     formatTimestamp(peer.registerExpiresAt),
			LastKeepaliveAt:       formatTimestamp(peer.lastKeepaliveAt),
			SubscribedAt:          formatTimestamp(peer.subscribedAt),
			SubscriptionExpiresAt: formatTimestamp(peer.subscriptionExpiresAt),
			LastCatalogNotifyAt:   formatTimestamp(peer.lastCatalogNotifyAt),
			AuthRequired:          peer.authRequired,
			LastError:             peer.lastError,
		})
	}
	for _, pending := range s.pending {
		if pending == nil {
			continue
		}
		out.Pending = append(out.Pending, gb28181PendingView{CallID: pending.callID, DeviceID: pending.device, MappingID: pending.mappingID, ResponseMode: pending.responseMode, Stage: pending.stage, LastStageAt: formatTimestamp(pending.lastStageAt), LastError: pending.lastError, StartedAt: formatTimestamp(pending.startedAt)})
	}
	for _, inbound := range s.inbound {
		if inbound == nil {
			continue
		}
		out.Inbound = append(out.Inbound, gb28181InboundView{CallID: inbound.callID, DeviceID: inbound.deviceID, MappingID: inbound.mappingID, CallbackAddr: inbound.callbackAddr, Transport: inbound.transport, RemoteRTPIP: inbound.remoteRTPIP, RemoteRTPPort: inbound.remoteRTPPort, Stage: inbound.stage, LastStageAt: formatTimestamp(inbound.lastStageAt), LastError: inbound.lastError, StartedAt: formatTimestamp(inbound.startedAt), LastInvokeAt: formatTimestamp(inbound.lastInvokeAt)})
	}
	sort.Slice(out.Peers, func(i, j int) bool { return out.Peers[i].DeviceID < out.Peers[j].DeviceID })
	sort.Slice(out.Pending, func(i, j int) bool { return out.Pending[i].CallID < out.Pending[j].CallID })
	sort.Slice(out.Inbound, func(i, j int) bool { return out.Inbound[i].CallID < out.Inbound[j].CallID })
	if s.catalog != nil {
		out.Catalog.ResourceTotal = len(s.catalog.Snapshot())
		exposure := s.catalog.ExposureSnapshot()
		for _, ports := range exposure {
			if len(ports) > 0 {
				out.Catalog.ExposedTotal++
			}
		}
	}
	return out
}

func parseHeaderInt(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func (d *handlerDeps) handleGB28181State(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	state := tunnelSessionRuntimeState{}
	if d.sessionMgr != nil {
		state = d.sessionMgr.Snapshot()
	}
	resp := map[string]any{
		"session": state,
		"config":  normalizeTunnelConfigPayload(d.tunnelConfig, ""),
	}
	if d.gbService != nil {
		resp["gb28181"] = d.gbService.Snapshot()
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
}
