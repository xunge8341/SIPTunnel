package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	filerepo "siptunnel/internal/repository/file"
)

func resolveSinglePeerBinding(peers []nodeconfig.PeerNodeConfig) (*PeerBinding, error) {
	enabled := make([]nodeconfig.PeerNodeConfig, 0, len(peers))
	for _, peer := range peers {
		if peer.Enabled {
			enabled = append(enabled, peer)
		}
	}
	if len(enabled) == 0 {
		return nil, fmt.Errorf("no enabled peer node configured; configure exactly one peer node in /api/peers")
	}
	if len(enabled) > 1 {
		ids := make([]string, 0, len(enabled))
		for _, peer := range enabled {
			ids = append(ids, peer.PeerNodeID)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("multiple enabled peer nodes configured (%s); current single-binding mode requires exactly one", strings.Join(ids, ","))
	}
	peer := enabled[0]
	return &PeerBinding{PeerNodeID: peer.PeerNodeID, PeerName: peer.PeerName, PeerSignalingIP: peer.PeerSignalingIP, PeerSignalingPort: peer.PeerSignalingPort}, nil
}

func (d *handlerDeps) currentPeerBinding() (*PeerBinding, error) {
	if d.nodeStore == nil {
		return nil, fmt.Errorf("node config store not configured")
	}
	peers := d.nodeStore.ListPeers()
	d.mu.RLock()
	preferredID := strings.TrimSpace(d.tunnelConfig.PeerDeviceID)
	d.mu.RUnlock()
	if preferredID != "" {
		for _, peer := range peers {
			if peer.Enabled && strings.EqualFold(strings.TrimSpace(peer.PeerNodeID), preferredID) {
				return &PeerBinding{PeerNodeID: peer.PeerNodeID, PeerName: peer.PeerName, PeerSignalingIP: peer.PeerSignalingIP, PeerSignalingPort: peer.PeerSignalingPort}, nil
			}
		}
	}
	return resolveSinglePeerBinding(peers)
}

func (d *handlerDeps) enforceCurrentPeerBinding(mapping *TunnelMapping) error {
	if mapping == nil {
		return fmt.Errorf("mapping is required")
	}
	binding, err := d.currentPeerBinding()
	if err != nil {
		if mapping.Enabled {
			return err
		}
		return nil
	}
	mapping.PeerNodeID = binding.PeerNodeID
	return nil
}

func bindingFromMapping(mapping TunnelMapping) *PeerBinding {
	if strings.TrimSpace(mapping.PeerNodeID) == "" {
		return nil
	}
	return &PeerBinding{PeerNodeID: mapping.PeerNodeID}
}

func fallbackPositive(v int, fallback int) int {
	if v > 0 {
		return v
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func defaultTunnelConfigPayload(mode config.NetworkMode) TunnelConfigPayload {
	normalized := mode.Normalize()
	capability := config.DeriveCapability(normalized)
	now := formatTimestamp(time.Now().UTC())
	return TunnelConfigPayload{
		ChannelProtocol:            "GB/T 28181",
		ConnectionInitiator:        "LOCAL",
		MappingRelayMode:           "AUTO",
		HeartbeatIntervalSec:       60,
		RegisterRetryCount:         3,
		RegisterRetryIntervalSec:   10,
		RegistrationStatus:         "unregistered",
		LastRegisterTime:           "",
		LastHeartbeatTime:          now,
		HeartbeatStatus:            "unknown",
		SupportedCapabilities:      capabilityDescriptions(capability),
		RequestChannel:             "SIP",
		ResponseChannel:            "RTP",
		NetworkMode:                normalized,
		Capability:                 capability,
		CapabilityItems:            capability.Matrix(),
		RegisterAuthAlgorithm:      "MD5",
		CatalogSubscribeExpiresSec: 3600,
	}
}

func normalizeTunnelConfigPayload(cfg TunnelConfigPayload, fallbackMode config.NetworkMode) TunnelConfigPayload {
	defaults := defaultTunnelConfigPayload(fallbackMode)
	mode := cfg.NetworkMode.Normalize()
	if mode == "" {
		mode = defaults.NetworkMode
	}
	capability := config.DeriveCapability(mode)
	channelProtocol := strings.TrimSpace(cfg.ChannelProtocol)
	if channelProtocol == "" {
		channelProtocol = defaults.ChannelProtocol
	}
	connectionInitiator := strings.ToUpper(strings.TrimSpace(cfg.ConnectionInitiator))
	if connectionInitiator != "LOCAL" && connectionInitiator != "PEER" {
		connectionInitiator = defaults.ConnectionInitiator
	}
	heartbeatIntervalSec := cfg.HeartbeatIntervalSec
	if heartbeatIntervalSec < 5 {
		heartbeatIntervalSec = defaults.HeartbeatIntervalSec
	}
	registerRetryCount := cfg.RegisterRetryCount
	if registerRetryCount < 0 {
		registerRetryCount = defaults.RegisterRetryCount
	}
	registerRetryIntervalSec := cfg.RegisterRetryIntervalSec
	if registerRetryIntervalSec < 1 {
		registerRetryIntervalSec = defaults.RegisterRetryIntervalSec
	}
	mappingRelayMode := strings.ToUpper(strings.TrimSpace(cfg.MappingRelayMode))
	if mappingRelayMode == "" {
		mappingRelayMode = defaults.MappingRelayMode
	}
	if mappingRelayMode != "AUTO" && mappingRelayMode != "SIP_ONLY" {
		mappingRelayMode = defaults.MappingRelayMode
	}
	requestChannel := "SIP"
	responseChannel := "RTP"
	registerAuthAlgorithm := strings.ToUpper(strings.TrimSpace(cfg.RegisterAuthAlgorithm))
	if registerAuthAlgorithm == "" {
		registerAuthAlgorithm = defaults.RegisterAuthAlgorithm
	}
	if registerAuthAlgorithm != "MD5" {
		registerAuthAlgorithm = defaults.RegisterAuthAlgorithm
	}
	catalogSubscribeExpiresSec := cfg.CatalogSubscribeExpiresSec
	if catalogSubscribeExpiresSec < 60 {
		catalogSubscribeExpiresSec = defaults.CatalogSubscribeExpiresSec
	}
	if mode == config.NetworkModeSenderSIPReceiverSIP {
		responseChannel = "SIP"
	} else if mode != config.NetworkModeSenderSIPReceiverRTP {
		responseChannel = "SIP/RTP"
	}
	if mappingRelayMode == "SIP_ONLY" {
		responseChannel = "SIP"
	}
	cfg.ChannelProtocol = channelProtocol
	cfg.ConnectionInitiator = connectionInitiator
	cfg.MappingRelayMode = mappingRelayMode
	cfg.HeartbeatIntervalSec = heartbeatIntervalSec
	cfg.RegisterRetryCount = registerRetryCount
	cfg.RegisterRetryIntervalSec = registerRetryIntervalSec
	cfg.RequestChannel = requestChannel
	cfg.ResponseChannel = responseChannel
	cfg.NetworkMode = mode
	cfg.Capability = capability
	cfg.CapabilityItems = capability.Matrix()
	cfg.SupportedCapabilities = capabilityDescriptions(capability)
	cfg.RegisterAuthAlgorithm = registerAuthAlgorithm
	cfg.CatalogSubscribeExpiresSec = catalogSubscribeExpiresSec
	if strings.TrimSpace(cfg.RegistrationStatus) == "" {
		cfg.RegistrationStatus = defaults.RegistrationStatus
	}
	if strings.TrimSpace(cfg.HeartbeatStatus) == "" {
		cfg.HeartbeatStatus = defaults.HeartbeatStatus
	}
	cfg.RegisterAuthPasswordConfigured = strings.TrimSpace(cfg.RegisterAuthPassword) != ""
	return cfg
}

func sanitizeTunnelConfigPayload(cfg TunnelConfigPayload) TunnelConfigPayload {
	cfg.RegisterAuthPasswordConfigured = strings.TrimSpace(cfg.RegisterAuthPassword) != ""
	cfg.RegisterAuthPassword = ""
	return cfg
}

func capabilityDescriptions(capability config.Capability) []string {
	desc := make([]string, 0, 6)
	if capability.SupportsSmallRequestBody {
		desc = append(desc, "支持小请求体（典型 SIP JSON 负载）")
	}
	if capability.SupportsLargeRequestBody {
		desc = append(desc, "支持大请求体上传")
	}
	if capability.SupportsLargeResponseBody {
		desc = append(desc, "支持大响应体回传")
	}
	if capability.SupportsStreamingResponse {
		desc = append(desc, "支持流式响应")
	}
	if capability.SupportsBidirectionalHTTPTunnel {
		desc = append(desc, "支持双向 HTTP 隧道")
	}
	if capability.SupportsTransparentHTTPProxy {
		desc = append(desc, "支持透明代理")
	}
	if len(desc) == 0 {
		desc = append(desc, "当前网络模式下暂无可用扩展能力")
	}
	return desc
}

func (d *handlerDeps) upsertTunnelConfig(req TunnelConfigUpdatePayload) (TunnelConfigPayload, error) {
	channelProtocol := strings.ToUpper(strings.TrimSpace(req.ChannelProtocol))
	connectionInitiator := strings.ToUpper(strings.TrimSpace(req.ConnectionInitiator))
	if channelProtocol == "" {
		return TunnelConfigPayload{}, fmt.Errorf("channel_protocol is required")
	}
	if connectionInitiator != "LOCAL" && connectionInitiator != "PEER" {
		return TunnelConfigPayload{}, fmt.Errorf("connection_initiator must be LOCAL or PEER")
	}
	if req.HeartbeatIntervalSec <= 0 {
		return TunnelConfigPayload{}, fmt.Errorf("heartbeat_interval_sec must be greater than 0")
	}
	if req.RegisterRetryCount < 0 {
		return TunnelConfigPayload{}, fmt.Errorf("register_retry_count must be greater than or equal to 0")
	}
	if req.RegisterRetryIntervalSec <= 0 {
		return TunnelConfigPayload{}, fmt.Errorf("register_retry_interval_sec must be greater than 0")
	}
	mode := req.NetworkMode.Normalize()
	if err := mode.Validate(); err != nil {
		return TunnelConfigPayload{}, err
	}
	capability := config.DeriveCapability(mode)
	session := d.sessionMgr.Snapshot()
	mappingRelayMode := strings.ToUpper(strings.TrimSpace(req.MappingRelayMode))
	if mappingRelayMode == "" {
		mappingRelayMode = d.tunnelConfig.MappingRelayMode
	}
	existing := d.tunnelConfig
	registerAuthEnabled := existing.RegisterAuthEnabled
	if req.RegisterAuthEnabled != nil {
		registerAuthEnabled = *req.RegisterAuthEnabled
	}
	registerAuthUsername := strings.TrimSpace(req.RegisterAuthUsername)
	if registerAuthUsername == "" {
		registerAuthUsername = existing.RegisterAuthUsername
	}
	registerAuthPassword := strings.TrimSpace(req.RegisterAuthPassword)
	if registerAuthPassword == "" {
		registerAuthPassword = existing.RegisterAuthPassword
	}
	registerAuthRealm := strings.TrimSpace(req.RegisterAuthRealm)
	if registerAuthRealm == "" {
		registerAuthRealm = existing.RegisterAuthRealm
	}
	registerAuthAlgorithm := strings.TrimSpace(req.RegisterAuthAlgorithm)
	if registerAuthAlgorithm == "" {
		registerAuthAlgorithm = existing.RegisterAuthAlgorithm
	}
	catalogSubscribeExpiresSec := req.CatalogSubscribeExpiresSec
	if catalogSubscribeExpiresSec <= 0 {
		catalogSubscribeExpiresSec = existing.CatalogSubscribeExpiresSec
	}
	updated := normalizeTunnelConfigPayload(TunnelConfigPayload{
		ChannelProtocol:            channelProtocol,
		ConnectionInitiator:        connectionInitiator,
		MappingRelayMode:           mappingRelayMode,
		HeartbeatIntervalSec:       req.HeartbeatIntervalSec,
		RegisterRetryCount:         req.RegisterRetryCount,
		RegisterRetryIntervalSec:   req.RegisterRetryIntervalSec,
		RegistrationStatus:         session.RegistrationStatus,
		LastRegisterTime:           session.LastRegisterTime,
		LastHeartbeatTime:          session.LastHeartbeatTime,
		HeartbeatStatus:            session.HeartbeatStatus,
		LastFailureReason:          session.LastFailureReason,
		NextRetryTime:              session.NextRetryTime,
		ConsecutiveHBTimeout:       session.ConsecutiveHeartbeatTimeout,
		SupportedCapabilities:      capabilityDescriptions(capability),
		RequestChannel:             "SIP",
		ResponseChannel:            "RTP",
		NetworkMode:                mode,
		Capability:                 capability,
		CapabilityItems:            capability.Matrix(),
		RegisterAuthEnabled:        registerAuthEnabled,
		RegisterAuthUsername:       registerAuthUsername,
		RegisterAuthPassword:       registerAuthPassword,
		RegisterAuthRealm:          registerAuthRealm,
		RegisterAuthAlgorithm:      registerAuthAlgorithm,
		CatalogSubscribeExpiresSec: catalogSubscribeExpiresSec,
	}, mode)
	if updated.RegisterAuthEnabled && strings.TrimSpace(updated.RegisterAuthPassword) == "" {
		return TunnelConfigPayload{}, fmt.Errorf("register_auth_password is required when register_auth_enabled=true")
	}
	d.mu.Lock()
	d.tunnelConfig = updated
	d.mu.Unlock()
	return updated, nil
}

func (d *handlerDeps) derivedLocalDeviceID() string {
	if d.nodeStore == nil {
		return ""
	}
	return strings.TrimSpace(d.nodeStore.GetLocalNode().NodeID)
}

func (d *handlerDeps) derivedPeerDeviceID() string {
	if d.nodeStore == nil {
		return ""
	}
	peers := d.nodeStore.ListPeers()
	if len(peers) == 0 {
		return ""
	}
	return strings.TrimSpace(peers[0].PeerNodeID)
}

func (d *handlerDeps) handleTunnelConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		resp := d.tunnelConfig
		d.mu.RUnlock()
		resp = normalizeTunnelConfigPayload(resp, config.DefaultNetworkMode())
		session := d.sessionMgr.Snapshot()
		resp.RegistrationStatus = session.RegistrationStatus
		resp.HeartbeatStatus = session.HeartbeatStatus
		resp.LastRegisterTime = session.LastRegisterTime
		resp.LastHeartbeatTime = session.LastHeartbeatTime
		resp.LastFailureReason = session.LastFailureReason
		resp.NextRetryTime = session.NextRetryTime
		resp.ConsecutiveHBTimeout = session.ConsecutiveHeartbeatTimeout
		resp.LocalDeviceID = d.derivedLocalDeviceID()
		resp.PeerDeviceID = d.derivedPeerDeviceID()
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: sanitizeTunnelConfigPayload(resp)})
	case http.MethodPost:
		var req TunnelConfigUpdatePayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		updated, err := d.upsertTunnelConfig(req)
		if err == nil {
			updated.LocalDeviceID = d.derivedLocalDeviceID()
			updated.PeerDeviceID = d.derivedPeerDeviceID()
		}
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.sessionMgr.ApplyConfig(updated)
		_ = saveJSON(d.tunnelPath, updated)
		if d.sqliteStore != nil {
			_ = d.sqliteStore.SaveSystemConfig(r.Context(), "tunnel.config", updated)
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_TUNNEL_CONFIG", sanitizeTunnelConfigPayload(updated))
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: sanitizeTunnelConfigPayload(updated)})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

type tunnelCatalogActionRequest struct {
	Action string `json:"action"`
}

type tunnelCatalogActionResponse struct {
	Action             string          `json:"action"`
	SubscribeTriggered int             `json:"subscribe_triggered,omitempty"`
	NotifyTriggered    int             `json:"notify_triggered,omitempty"`
	GB28181            GB28181Snapshot `json:"gb28181"`
}

func (d *handlerDeps) onLocalCatalogChanged() {
	d.syncMappingRuntime()
	if d.gbService != nil {
		d.gbService.SyncLocalCatalog()
		go d.gbService.TriggerCatalogPush(context.Background())
	}
}

func (d *handlerDeps) handleTunnelCatalogActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var req tunnelCatalogActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
		return
	}
	if d.gbService == nil {
		writeError(w, http.StatusNotImplemented, "GB28181_NOT_ENABLED", "gb28181 tunnel service not configured")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	resp := tunnelCatalogActionResponse{Action: action}
	switch action {
	case "pull_remote", "subscribe_now":
		resp.SubscribeTriggered = d.gbService.TriggerCatalogPull(ctx)
	case "push_local", "notify_now":
		resp.NotifyTriggered = d.gbService.TriggerCatalogPush(ctx)
	case "refresh_all", "refresh":
		resp.SubscribeTriggered, resp.NotifyTriggered = d.gbService.TriggerCatalogRefresh(ctx)
	default:
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "action must be pull_remote, push_local or refresh_all")
		return
	}
	resp.GB28181 = d.gbService.Snapshot()
	d.recordOpsAudit(r, readOperator(r), "TUNNEL_CATALOG_ACTION", map[string]any{"action": action, "subscribe_triggered": resp.SubscribeTriggered, "notify_triggered": resp.NotifyTriggered})
	logTunnelCatalogAction(action, resp.SubscribeTriggered, resp.NotifyTriggered, resp.GB28181)
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
}

func (d *handlerDeps) handleTunnelSessionActions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var req tunnelSessionActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	switch action {
	case "register_now":
		d.sessionMgr.TriggerRegister()
	case "reregister":
		d.sessionMgr.TriggerReregister()
	case "heartbeat_once":
		d.sessionMgr.TriggerHeartbeat()
	default:
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "action must be register_now, reregister or heartbeat_once")
		return
	}
	state := d.sessionMgr.Snapshot()
	d.recordOpsAudit(r, readOperator(r), "TUNNEL_SESSION_ACTION", map[string]any{"action": action, "state": state})
	logTunnelSessionAction(action, state)
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: tunnelSessionActionResponse{Action: action, State: state}})
}

func (d *handlerDeps) handlePeers(w http.ResponseWriter, r *http.Request) {
	if d.nodeStore == nil {
		writeError(w, http.StatusNotImplemented, "NODE_STORE_NOT_ENABLED", "node config store not configured")
		return
	}
	peerID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/peers/"))
	hasID := peerID != "" && r.URL.Path != "/api/peers"
	switch r.Method {
	case http.MethodGet:
		if hasID {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "GET /api/peers/{id} not supported; use GET /api/peers")
			return
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": d.nodeStore.ListPeers()}})
	case http.MethodPost:
		if hasID {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "POST must target /api/peers")
			return
		}
		var req nodeconfig.PeerNodeConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		status := d.networkStatusFunc(r.Context())
		if req.SupportedNetworkMode.Normalize() != status.NetworkMode.Normalize() {
			writeError(w, http.StatusBadRequest, "PEER_NETWORK_MODE_INCOMPATIBLE", fmt.Sprintf("peer supported_network_mode=%s incompatible with current_network_mode=%s", req.SupportedNetworkMode.Normalize(), status.NetworkMode.Normalize()))
			return
		}
		created, err := d.nodeStore.CreatePeer(req)
		if err != nil {
			code := http.StatusBadRequest
			errCode := "INVALID_ARGUMENT"
			if errors.Is(err, filerepo.ErrPeerAlreadyExists) {
				code = http.StatusConflict
				errCode = "PEER_ALREADY_EXISTS"
			}
			writeError(w, code, errCode, err.Error())
			return
		}
		d.recordOpsAudit(r, readOperator(r), "CREATE_PEER_NODE", created)
		writeJSON(w, http.StatusCreated, responseEnvelope{Code: "OK", Message: "success", Data: created})
	case http.MethodPut:
		if !hasID {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "PUT must target /api/peers/{peer_node_id}")
			return
		}
		var req nodeconfig.PeerNodeConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		req.PeerNodeID = peerID
		status := d.networkStatusFunc(r.Context())
		if req.SupportedNetworkMode.Normalize() != status.NetworkMode.Normalize() {
			writeError(w, http.StatusBadRequest, "PEER_NETWORK_MODE_INCOMPATIBLE", fmt.Sprintf("peer supported_network_mode=%s incompatible with current_network_mode=%s", req.SupportedNetworkMode.Normalize(), status.NetworkMode.Normalize()))
			return
		}
		updated, err := d.nodeStore.UpdatePeer(req)
		if err != nil {
			code := http.StatusBadRequest
			errCode := "INVALID_ARGUMENT"
			if errors.Is(err, filerepo.ErrPeerNotFound) {
				code = http.StatusNotFound
				errCode = "PEER_NOT_FOUND"
			}
			writeError(w, code, errCode, err.Error())
			return
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_PEER_NODE", updated)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: updated})
	case http.MethodDelete:
		if !hasID {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "DELETE must target /api/peers/{peer_node_id}")
			return
		}
		if err := d.nodeStore.DeletePeer(peerID); err != nil {
			if errors.Is(err, filerepo.ErrPeerNotFound) {
				writeError(w, http.StatusNotFound, "PEER_NOT_FOUND", err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		d.recordOpsAudit(r, readOperator(r), "DELETE_PEER_NODE", map[string]string{"peer_node_id": peerID})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]string{"peer_node_id": peerID}})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
