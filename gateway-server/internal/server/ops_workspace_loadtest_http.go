package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"siptunnel/internal/config"
	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/tunnelmapping"
)

func isValidHostOrIP(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	if net.ParseIP(v) != nil {
		return true
	}
	for _, r := range v {
		if !(r == '.' || r == '-' || r == ':' || r == '_' || (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')) {
			return false
		}
	}
	return true
}

func validatePortRange(label string, start, end int) error {
	if start < 1 || start > 65535 || end < 1 || end > 65535 {
		return fmt.Errorf("%s端口必须在 1-65535 之间", label)
	}
	if start > end {
		return fmt.Errorf("%s起始端口不能大于结束端口", label)
	}
	if end-start < 1 {
		return fmt.Errorf("%s端口范围至少保留两个端口", label)
	}
	return nil
}

func normalizeNodeTunnelWorkspace(d *handlerDeps, req *nodeTunnelWorkspace) {
	if req == nil {
		return
	}
	currentLocal := nodeconfig.DefaultLocalNodeConfig()
	if d != nil && d.nodeStore != nil {
		currentLocal = d.nodeStore.GetLocalNode()
	}
	req.LocalNode.MappingPortStart, req.LocalNode.MappingPortEnd = normalizeMappingPortRange(
		req.LocalNode.MappingPortStart,
		req.LocalNode.MappingPortEnd,
		currentLocal.MappingPortStart,
		currentLocal.MappingPortEnd,
		req.LocalNode.SignalingPort,
		req.LocalNode.RTPPortStart,
		req.LocalNode.RTPPortEnd,
	)
}

func validateNodeTunnelWorkspace(d *handlerDeps, req nodeTunnelWorkspace) error {
	mode := config.NetworkMode(strings.TrimSpace(req.NetworkMode)).Normalize()
	if err := mode.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(req.LocalNode.DeviceID) == "" || strings.TrimSpace(req.PeerNode.DeviceID) == "" {
		return fmt.Errorf("本端编码与对端编码不能为空")
	}
	if !tunnelmapping.IsGBCode20(req.LocalNode.DeviceID) || !tunnelmapping.IsGBCode20(req.PeerNode.DeviceID) {
		return fmt.Errorf("本端编码与对端编码必须为 20 位国标编码")
	}
	if !isValidHostOrIP(req.LocalNode.NodeIP) || !isValidHostOrIP(req.PeerNode.NodeIP) {
		return fmt.Errorf("本端或对端地址格式不正确")
	}
	if req.LocalNode.SignalingPort < 1 || req.LocalNode.SignalingPort > 65535 || req.PeerNode.SignalingPort < 1 || req.PeerNode.SignalingPort > 65535 {
		return fmt.Errorf("信令端口必须在 1-65535 之间")
	}
	if err := validatePortRange("本端 RTP", req.LocalNode.RTPPortStart, req.LocalNode.RTPPortEnd); err != nil {
		return err
	}
	if err := validatePortRange("本地隧道映射", req.LocalNode.MappingPortStart, req.LocalNode.MappingPortEnd); err != nil {
		return err
	}
	if err := validatePortRange("对端 RTP", req.PeerNode.RTPPortStart, req.PeerNode.RTPPortEnd); err != nil {
		return err
	}
	if req.LocalNode.SignalingPort >= req.LocalNode.RTPPortStart && req.LocalNode.SignalingPort <= req.LocalNode.RTPPortEnd {
		return fmt.Errorf("本端信令端口不能落在本端 RTP 端口范围内")
	}
	if req.LocalNode.SignalingPort >= req.LocalNode.MappingPortStart && req.LocalNode.SignalingPort <= req.LocalNode.MappingPortEnd {
		return fmt.Errorf("本地隧道映射端口范围不能覆盖本端 SIP 信令端口")
	}
	if req.LocalNode.MappingPortStart <= req.LocalNode.RTPPortEnd && req.LocalNode.RTPPortEnd >= req.LocalNode.MappingPortStart && req.LocalNode.MappingPortEnd >= req.LocalNode.RTPPortStart {
		return fmt.Errorf("本地隧道映射端口范围不能与本端 RTP 端口范围重叠")
	}
	if d.uiConfig.Enabled && d.uiConfig.Mode == "embedded" && req.LocalNode.MappingPortStart <= d.uiConfig.ListenPort && d.uiConfig.ListenPort <= req.LocalNode.MappingPortEnd && bindAddrLikelyConflict(req.LocalNode.NodeIP, d.uiConfig.ListenIP) {
		return fmt.Errorf("本地隧道映射端口范围不能覆盖网关 UI 监听 %s:%d", d.uiConfig.ListenIP, d.uiConfig.ListenPort)
	}
	if req.PeerNode.SignalingPort >= req.PeerNode.RTPPortStart && req.PeerNode.SignalingPort <= req.PeerNode.RTPPortEnd {
		return fmt.Errorf("对端信令端口不能落在对端 RTP 端口范围内")
	}
	enc := strings.ToUpper(strings.TrimSpace(asString(req.SecuritySettings["encryption"])))
	if enc != "" && enc != "AES" && enc != "SM4" {
		return fmt.Errorf("加密算法仅支持 AES 或 SM4")
	}
	if asBool(req.SecuritySettings["admin_require_mfa"]) {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(asString(req.SecuritySettings["admin_allow_cidr"]))); err != nil {
			return fmt.Errorf("启用 MFA 时，必须提供合法的管理面 CIDR")
		}
	}
	connectionInitiator := strings.ToUpper(strings.TrimSpace(req.SessionSettings.ConnectionInitiator))
	if connectionInitiator != "LOCAL" && connectionInitiator != "PEER" {
		return fmt.Errorf("连接发起方仅支持 LOCAL 或 PEER")
	}
	mappingRelayMode := strings.ToUpper(strings.TrimSpace(req.SessionSettings.MappingRelayMode))
	if mappingRelayMode != "" && mappingRelayMode != "AUTO" && mappingRelayMode != "SIP_ONLY" {
		return fmt.Errorf("映射承载模式仅支持 AUTO 或 SIP_ONLY")
	}
	if req.SessionSettings.HeartbeatIntervalSec < 5 {
		return fmt.Errorf("心跳间隔至少为 5 秒")
	}
	if req.SessionSettings.RegisterRetryIntervalSec < 1 {
		return fmt.Errorf("注册重试间隔至少为 1 秒")
	}
	if req.SessionSettings.RegisterRetryCount < 0 {
		return fmt.Errorf("注册重试次数不能为负数")
	}
	return nil
}

func (d *handlerDeps) handleNodeTunnelWorkspace(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		node := d.nodeStore.GetLocalNode()
		peer := NodeConfigEndpoint{}
		if binding, err := resolveSinglePeerBinding(d.nodeStore.ListPeers()); err == nil && binding != nil {
			peer = NodeConfigEndpoint{NodeIP: binding.PeerSignalingIP, SignalingPort: binding.PeerSignalingPort, DeviceID: binding.PeerNodeID, NodeType: tunnelmapping.DefaultNodeType}
			for _, item := range d.nodeStore.ListPeers() {
				if strings.EqualFold(strings.TrimSpace(item.PeerNodeID), strings.TrimSpace(binding.PeerNodeID)) {
					peer.RTPPortStart = item.PeerMediaPortStart
					peer.RTPPortEnd = item.PeerMediaPortEnd
					break
				}
			}
		}
		capability := config.DeriveCapability(node.NetworkMode)
		session := d.sessionMgr.Snapshot()
		sessionSettings := sanitizeTunnelConfigPayload(normalizeTunnelConfigPayload(d.tunnelConfig, node.NetworkMode))
		sessionSettings.RegistrationStatus = session.RegistrationStatus
		sessionSettings.HeartbeatStatus = session.HeartbeatStatus
		sessionSettings.LastRegisterTime = session.LastRegisterTime
		sessionSettings.LastHeartbeatTime = session.LastHeartbeatTime
		sessionSettings.LastFailureReason = session.LastFailureReason
		sessionSettings.NextRetryTime = session.NextRetryTime
		sessionSettings.ConsecutiveHBTimeout = session.ConsecutiveHeartbeatTimeout
		workspace := nodeTunnelWorkspace{
			LocalNode:   NodeConfigEndpoint{NodeIP: node.SIPListenIP, SignalingPort: node.SIPListenPort, DeviceID: node.NodeID, NodeType: tunnelmapping.NormalizeNodeType(node.NodeRole), RTPPortStart: node.RTPPortStart, RTPPortEnd: node.RTPPortEnd, MappingPortStart: node.MappingPortStart, MappingPortEnd: node.MappingPortEnd},
			PeerNode:    peer,
			NetworkMode: string(node.NetworkMode),
			CapabilityMatrix: []map[string]any{
				{"key": "supports_small_request_body", "supported": capability.SupportsSmallRequestBody},
				{"key": "supports_large_request_body", "supported": capability.SupportsLargeRequestBody},
				{"key": "supports_large_response_body", "supported": capability.SupportsLargeResponseBody},
				{"key": "supports_streaming_response", "supported": capability.SupportsStreamingResponse},
				{"key": "supports_bidirectional_http_tunnel", "supported": capability.SupportsBidirectionalHTTPTunnel},
			},
			SIPCapability:      map[string]any{"transport": strings.ToUpper(strings.TrimSpace(defaultString(node.SIPTransport, "TCP"))), "listen_ip": node.SIPListenIP, "listen_port": node.SIPListenPort},
			RTPCapability:      map[string]any{"transport": strings.ToUpper(strings.TrimSpace(defaultString(node.RTPTransport, "UDP"))), "port_start": node.RTPPortStart, "port_end": node.RTPPortEnd},
			SessionSettings:    sessionSettings,
			SecuritySettings:   map[string]any{"signer": d.securitySettings.Signer, "encryption": d.securitySettings.Encryption, "verify_interval_min": d.securitySettings.VerifyIntervalMin, "admin_allow_cidr": d.systemSettings.AdminAllowCIDR, "admin_require_mfa": d.systemSettings.AdminRequireMFA},
			EncryptionSettings: map[string]string{"algorithm": d.securitySettings.Encryption},
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: workspace})
	case http.MethodPost:
		var req nodeTunnelWorkspace
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		normalizeNodeTunnelWorkspace(d, &req)
		if err := validateNodeTunnelWorkspace(d, req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		local := d.nodeStore.GetLocalNode()
		local.NodeID = strings.TrimSpace(req.LocalNode.DeviceID)
		if local.NodeID == "" {
			local.NodeID = d.nodeStore.GetLocalNode().NodeID
		}
		local.NodeName = local.NodeID
		local.NodeRole = tunnelmapping.NormalizeNodeType(req.LocalNode.NodeType)
		local.NetworkMode = config.NetworkMode(strings.TrimSpace(req.NetworkMode)).Normalize()
		local.SIPListenIP = strings.TrimSpace(req.LocalNode.NodeIP)
		local.RTPListenIP = local.SIPListenIP
		local.SIPListenPort = req.LocalNode.SignalingPort
		local.RTPPortStart = req.LocalNode.RTPPortStart
		local.RTPPortEnd = req.LocalNode.RTPPortEnd
		local.MappingPortStart = req.LocalNode.MappingPortStart
		local.MappingPortEnd = req.LocalNode.MappingPortEnd
		local.SIPTransport = strings.ToUpper(defaultString(asString(req.SIPCapability["transport"]), local.SIPTransport))
		local.RTPTransport = strings.ToUpper(defaultString(asString(req.RTPCapability["transport"]), local.RTPTransport))
		peerID := strings.TrimSpace(req.PeerNode.DeviceID)
		var peerCfg nodeconfig.PeerNodeConfig
		var hasPeer bool
		if peerID != "" && strings.TrimSpace(req.PeerNode.NodeIP) != "" && req.PeerNode.SignalingPort > 0 {
			hasPeer = true
			peerCfg = nodeconfig.PeerNodeConfig{PeerNodeID: peerID, PeerName: peerID, PeerSignalingIP: strings.TrimSpace(req.PeerNode.NodeIP), PeerSignalingPort: req.PeerNode.SignalingPort, PeerMediaIP: strings.TrimSpace(req.PeerNode.NodeIP), PeerMediaPortStart: req.PeerNode.RTPPortStart, PeerMediaPortEnd: req.PeerNode.RTPPortEnd, SupportedNetworkMode: local.NetworkMode, Enabled: true}
		}
		if hasPeer {
			if _, _, err := d.nodeStore.ApplyWorkspace(local, peerCfg); err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
				return
			}
		} else if _, err := d.nodeStore.UpdateLocalNode(local); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		existingTunnel := d.tunnelConfig
		if strings.TrimSpace(req.SessionSettings.RegisterAuthPassword) == "" {
			req.SessionSettings.RegisterAuthPassword = existingTunnel.RegisterAuthPassword
		}
		req.SessionSettings.NetworkMode = local.NetworkMode
		req.SessionSettings.LocalDeviceID = local.NodeID
		req.SessionSettings.PeerDeviceID = peerID
		req.SessionSettings = normalizeTunnelConfigPayload(req.SessionSettings, local.NetworkMode)
		d.mu.Lock()
		d.tunnelConfig = req.SessionSettings
		d.securitySettings = SecuritySettingsPayload{Signer: defaultString(asString(req.SecuritySettings["signer"]), "HMAC-SHA256"), Encryption: defaultString(asString(req.SecuritySettings["encryption"]), d.securitySettings.Encryption), VerifyIntervalMin: int(asFloatSetting(req.SecuritySettings["verify_interval_min"]))}
		if d.securitySettings.VerifyIntervalMin <= 0 {
			d.securitySettings.VerifyIntervalMin = 30
		}
		d.systemSettings.AdminAllowCIDR = defaultString(asString(req.SecuritySettings["admin_allow_cidr"]), d.systemSettings.AdminAllowCIDR)
		d.systemSettings.AdminRequireMFA = asBool(req.SecuritySettings["admin_require_mfa"])
		sec := d.securitySettings
		sys := d.systemSettings
		d.mu.Unlock()
		if d.sessionMgr != nil {
			d.sessionMgr.ApplyConfig(req.SessionSettings)
		}
		d.onLocalCatalogChanged()
		_ = saveJSON(d.tunnelPath, req.SessionSettings)
		_ = saveJSON(d.securityPath, sec)
		_ = saveJSON(d.systemPath, sys)
		logWorkspaceApplied(req)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: req})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleLoadtests(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/loadtests/") && len(strings.TrimPrefix(r.URL.Path, "/api/loadtests/")) > 0 {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/loadtests/")
		if job, ok := d.loadtestJobs.get(id); ok {
			writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: job})
			return
		}
		writeError(w, http.StatusNotFound, "NOT_FOUND", "loadtest job not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		items := []loadtestJob{}
		if d.loadtestJobs != nil {
			items = d.loadtestJobs.list()
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: map[string]any{"items": items}})
	case http.MethodPost:
		var req struct {
			Targets        []string `json:"targets"`
			HTTPURL        string   `json:"http_url"`
			SIPAddress     string   `json:"sip_address"`
			SIPTransport   string   `json:"sip_transport"`
			RTPAddress     string   `json:"rtp_address"`
			RTPTransport   string   `json:"rtp_transport"`
			GatewayBaseURL string   `json:"gateway_base_url"`
			Concurrency    int      `json:"concurrency"`
			QPS            int      `json:"qps"`
			DurationSec    int      `json:"duration_sec"`
			OutputDir      string   `json:"output_dir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		if len(req.Targets) == 0 {
			req.Targets = []string{"http-invoke"}
		}
		if req.Concurrency <= 0 {
			req.Concurrency = 10
		}
		if req.DurationSec <= 0 {
			req.DurationSec = 30
		}
		status := d.networkStatusFunc(r.Context())
		if strings.TrimSpace(req.SIPTransport) == "" {
			req.SIPTransport = strings.ToUpper(strings.TrimSpace(status.SIP.Transport))
		}
		if strings.TrimSpace(req.RTPTransport) == "" {
			req.RTPTransport = strings.ToUpper(strings.TrimSpace(status.RTP.Transport))
		}
		if strings.TrimSpace(req.SIPAddress) == "" && strings.TrimSpace(status.SIP.ListenIP) != "" && status.SIP.ListenPort > 0 {
			req.SIPAddress = net.JoinHostPort(strings.TrimSpace(status.SIP.ListenIP), strconv.Itoa(status.SIP.ListenPort))
		}
		if strings.TrimSpace(req.RTPAddress) == "" && strings.TrimSpace(status.RTP.ListenIP) != "" && status.RTP.PortStart > 0 {
			req.RTPAddress = net.JoinHostPort(strings.TrimSpace(status.RTP.ListenIP), strconv.Itoa(status.RTP.PortStart))
		}
		job := loadtestJob{JobID: time.Now().UTC().Format("20060102T150405.000000000"), Status: "pending", CreatedAt: formatTimestamp(time.Now().UTC()), UpdatedAt: formatTimestamp(time.Now().UTC()), Targets: req.Targets, HTTPURL: req.HTTPURL, SIPAddress: req.SIPAddress, SIPTransport: req.SIPTransport, RTPAddress: req.RTPAddress, RTPTransport: req.RTPTransport, GatewayBaseURL: req.GatewayBaseURL, Concurrency: req.Concurrency, QPS: req.QPS, DurationSec: req.DurationSec, OutputDir: ensureDir(req.OutputDir)}
		if d.loadtestJobs != nil {
			d.loadtestJobs.upsert(job)
		}
		d.mu.RLock()
		limits := d.limits
		d.mu.RUnlock()
		startLoadtestJob(context.Background(), d.loadtestJobs, limits, status, job)
		writeJSON(w, http.StatusAccepted, responseEnvelope{Code: "OK", Message: "started", Data: job})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
