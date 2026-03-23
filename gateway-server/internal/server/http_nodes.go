package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/tunnelmapping"
)

func (d *handlerDeps) compatibilitySnapshot(ctx context.Context) nodeconfig.CompatibilityStatus {
	status := d.networkStatusFunc(ctx)
	local := d.nodeStore.GetLocalNode()
	peers := d.nodeStore.ListPeers()
	mode := local.NetworkMode.Normalize()
	if mode == "" {
		d.mu.RLock()
		mode = d.tunnelConfig.NetworkMode.Normalize()
		d.mu.RUnlock()
	}
	if mode == "" {
		mode = status.NetworkMode.Normalize()
	}
	capability := config.DeriveCapability(mode)
	return nodeconfig.EvaluateCompatibility(local, peers, mode, capability)
}

func (d *handlerDeps) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if d.nodeStore == nil {
		writeError(w, http.StatusNotImplemented, "NODE_STORE_NOT_ENABLED", "node config store not configured")
		return
	}
	local := d.nodeStore.GetLocalNode()
	nodeSource := d.nodeConfigSource
	if strings.TrimSpace(nodeSource) == "" {
		nodeSource = dataSourceLabel("", "node_config.json")
	}
	node := OpsNode{
		NodeID:     local.NodeID,
		Role:       local.NodeRole,
		Status:     "configured",
		Endpoint:   net.JoinHostPort(local.SIPListenIP, strconv.Itoa(local.SIPListenPort)),
		DataSource: nodeSource,
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": []OpsNode{node}}})
}

func (d *handlerDeps) handleNode(w http.ResponseWriter, r *http.Request) {
	if d.nodeStore == nil {
		writeError(w, http.StatusNotImplemented, "NODE_STORE_NOT_ENABLED", "node config store not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		compat := d.compatibilitySnapshot(r.Context())
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: NodeDetailResponse{LocalNode: d.nodeStore.GetLocalNode(), CurrentNetworkMode: compat.CurrentNetworkMode, CurrentCapability: compat.CurrentCapability, CompatibilityStatus: compat.CompatibilityCheck}})
	case http.MethodPut:
		var req nodeconfig.LocalNodeConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		status := d.networkStatusFunc(r.Context())
		if req.NetworkMode.Normalize() != status.NetworkMode.Normalize() {
			writeError(w, http.StatusBadRequest, "NETWORK_MODE_MISMATCH", fmt.Sprintf("local node network_mode=%s must match current network_mode=%s", req.NetworkMode.Normalize(), status.NetworkMode.Normalize()))
			return
		}
		updated, err := d.nodeStore.UpdateLocalNode(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_LOCAL_NODE", updated)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: updated})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleNodeConfig(w http.ResponseWriter, r *http.Request) {
	if d.nodeStore == nil {
		writeError(w, http.StatusNotImplemented, "NODE_STORE_NOT_ENABLED", "node config store not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		local := d.nodeStore.GetLocalNode()
		payload := NodeConfigPayload{LocalNode: NodeConfigEndpoint{NodeIP: local.SIPListenIP, SignalingPort: local.SIPListenPort, DeviceID: local.NodeID, NodeType: tunnelmapping.NormalizeNodeType(local.NodeRole), RTPPortStart: local.RTPPortStart, RTPPortEnd: local.RTPPortEnd, MappingPortStart: local.MappingPortStart, MappingPortEnd: local.MappingPortEnd}}
		if binding, err := d.currentPeerBinding(); err == nil && binding != nil {
			payload.PeerNode = NodeConfigEndpoint{NodeIP: binding.PeerSignalingIP, SignalingPort: binding.PeerSignalingPort, DeviceID: binding.PeerNodeID, NodeType: tunnelmapping.DefaultNodeType}
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: payload})
	case http.MethodPost:
		var req NodeConfigPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		currentLocal := d.nodeStore.GetLocalNode()
		req.LocalNode.MappingPortStart, req.LocalNode.MappingPortEnd = normalizeMappingPortRange(
			req.LocalNode.MappingPortStart,
			req.LocalNode.MappingPortEnd,
			currentLocal.MappingPortStart,
			currentLocal.MappingPortEnd,
			req.LocalNode.SignalingPort,
			req.LocalNode.RTPPortStart,
			req.LocalNode.RTPPortEnd,
		)
		if strings.TrimSpace(req.LocalNode.NodeIP) == "" || strings.TrimSpace(req.LocalNode.DeviceID) == "" || req.LocalNode.SignalingPort <= 0 || req.LocalNode.RTPPortStart <= 0 || req.LocalNode.RTPPortEnd <= 0 || req.LocalNode.MappingPortStart <= 0 || req.LocalNode.MappingPortEnd <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "local_node fields are required")
			return
		}
		if strings.TrimSpace(req.PeerNode.NodeIP) == "" || strings.TrimSpace(req.PeerNode.DeviceID) == "" || req.PeerNode.SignalingPort <= 0 {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "peer_node fields are required")
			return
		}
		if !tunnelmapping.IsGBCode20(req.LocalNode.DeviceID) || !tunnelmapping.IsGBCode20(req.PeerNode.DeviceID) {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "local_node.device_id and peer_node.device_id must be 20-digit GB/T 28181 codes")
			return
		}

		local := d.nodeStore.GetLocalNode()
		local.NodeID = strings.TrimSpace(req.LocalNode.DeviceID)
		local.NodeName = local.NodeID
		local.NodeRole = tunnelmapping.NormalizeNodeType(req.LocalNode.NodeType)
		local.SIPListenIP = strings.TrimSpace(req.LocalNode.NodeIP)
		local.RTPListenIP = local.SIPListenIP
		local.SIPListenPort = req.LocalNode.SignalingPort
		local.RTPPortStart = req.LocalNode.RTPPortStart
		local.RTPPortEnd = req.LocalNode.RTPPortEnd
		local.MappingPortStart = req.LocalNode.MappingPortStart
		local.MappingPortEnd = req.LocalNode.MappingPortEnd
		updatedLocal, err := d.nodeStore.UpdateLocalNode(local)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}

		peerID := strings.TrimSpace(req.PeerNode.DeviceID)
		peerCfg := nodeconfig.PeerNodeConfig{
			PeerNodeID:           peerID,
			PeerName:             peerID,
			PeerSignalingIP:      strings.TrimSpace(req.PeerNode.NodeIP),
			PeerSignalingPort:    req.PeerNode.SignalingPort,
			PeerMediaIP:          strings.TrimSpace(req.PeerNode.NodeIP),
			PeerMediaPortStart:   req.PeerNode.RTPPortStart,
			PeerMediaPortEnd:     req.PeerNode.RTPPortEnd,
			SupportedNetworkMode: updatedLocal.NetworkMode,
			Enabled:              true,
		}
		if peerCfg.PeerMediaPortStart <= 0 {
			peerCfg.PeerMediaPortStart = req.LocalNode.RTPPortStart
		}
		if peerCfg.PeerMediaPortEnd <= 0 {
			peerCfg.PeerMediaPortEnd = req.LocalNode.RTPPortEnd
		}
		if _, _, err := d.nodeStore.ApplyWorkspace(updatedLocal, peerCfg); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.mu.Lock()
		d.tunnelConfig.PeerDeviceID = peerCfg.PeerNodeID
		d.mu.Unlock()

		resp := NodeConfigPayload{
			LocalNode: NodeConfigEndpoint{NodeIP: updatedLocal.SIPListenIP, SignalingPort: updatedLocal.SIPListenPort, DeviceID: updatedLocal.NodeID, NodeType: tunnelmapping.NormalizeNodeType(updatedLocal.NodeRole), RTPPortStart: updatedLocal.RTPPortStart, RTPPortEnd: updatedLocal.RTPPortEnd, MappingPortStart: updatedLocal.MappingPortStart, MappingPortEnd: updatedLocal.MappingPortEnd},
			PeerNode:  NodeConfigEndpoint{NodeIP: peerCfg.PeerSignalingIP, SignalingPort: peerCfg.PeerSignalingPort, DeviceID: peerCfg.PeerNodeID, NodeType: tunnelmapping.DefaultNodeType},
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_NODE_CONFIG_AND_RESTART_TUNNEL", resp)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "节点配置已保存并重启隧道", Data: map[string]any{"config": resp, "tunnel_restarted": true}})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
