package server

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"siptunnel/internal/nodeconfig"
	"siptunnel/internal/observability"
	"siptunnel/internal/repository"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/startupsummary"
)

func (d *handlerDeps) handleSelfCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if d.selfCheckProvider == nil {
		writeError(w, http.StatusNotImplemented, "SELF_CHECK_NOT_ENABLED", "self-check provider not configured")
		return
	}
	report := d.selfCheckProvider(r.Context())
	if d.nodeStore != nil {
		compat := d.compatibilitySnapshot(r.Context())
		report.Items = append(report.Items,
			selfcheck.Item{Name: "local_node_config_valid", Level: selfcheck.Level(compat.LocalNodeCheck.Level), Message: compat.LocalNodeCheck.Message, Suggestion: compat.LocalNodeCheck.Suggestion, ActionHint: compat.LocalNodeCheck.ActionHint},
			selfcheck.Item{Name: "peer_node_config_valid", Level: selfcheck.Level(compat.PeerNodeCheck.Level), Message: compat.PeerNodeCheck.Message, Suggestion: compat.PeerNodeCheck.Suggestion, ActionHint: compat.PeerNodeCheck.ActionHint},
			selfcheck.Item{Name: "network_mode_compatibility", Level: selfcheck.Level(compat.CompatibilityCheck.Level), Message: compat.CompatibilityCheck.Message, Suggestion: compat.CompatibilityCheck.Suggestion, ActionHint: compat.CompatibilityCheck.ActionHint},
		)
	}
	mappingValidation := d.validateMappingsAgainstCapability(d.mappings.List())
	mappingLevel := selfcheck.LevelInfo
	mappingMessage := "all mappings are compatible with current capability"
	mappingSuggestion := "继续按当前 network_mode 维护映射配置"
	mappingHint := "每次变更后复核 /api/mappings 与 /api/selfcheck。"
	if mappingValidation.HasErrors() {
		mappingLevel = selfcheck.LevelError
		mappingMessage = strings.Join(mappingValidation.Errors, "; ")
		mappingSuggestion = "调整 mapping 参数（body 限制/方法/流式要求）或切换 network_mode。"
		mappingHint = "修复后重新执行 /api/selfcheck。"
	} else if len(mappingValidation.Warnings) > 0 {
		mappingLevel = selfcheck.LevelWarn
		mappingMessage = strings.Join(mappingValidation.Warnings, "; ")
		mappingSuggestion = "建议收敛 mapping 配置以降低在受限模式下的不稳定风险。"
		mappingHint = "根据 warnings 逐条确认并保留运行记录。"
	}
	report.Items = append(report.Items, selfcheck.Item{Name: "mappings_capability_validation", Level: mappingLevel, Message: mappingMessage, Suggestion: mappingSuggestion, ActionHint: mappingHint})
	if d.nodeStore != nil {
		enabledMappings := 0
		for _, mapping := range d.mappings.List() {
			if mapping.Enabled {
				enabledMappings++
			}
		}
		d.mu.RLock()
		preferredPeerID := strings.TrimSpace(d.tunnelConfig.PeerDeviceID)
		d.mu.RUnlock()
		if binding, err := d.currentPeerBinding(); err != nil {
			level := selfcheck.LevelWarn
			message := err.Error()
			suggestion := "在 peer 配置页保持仅一个启用的对端节点"
			actionHint := "新增/禁用 peer 后重新执行 /api/selfcheck。"
			if enabledMappings == 0 && preferredPeerID == "" {
				level = selfcheck.LevelInfo
				message = "no enabled peer node configured; current environment is in protocol-layer ready state and business mappings are not yet activated"
				suggestion = "如需启用业务映射或目录同步，请先配置且仅启用一个 peer 节点。"
				actionHint = "受控联调阶段可先保持空闲；启用 mappings 前完成 peer 绑定。"
			}
			report.Items = append(report.Items, selfcheck.Item{Name: "mapping_peer_binding", Level: level, Message: message, Suggestion: suggestion, ActionHint: actionHint})
		} else {
			report.Items = append(report.Items, selfcheck.Item{Name: "mapping_peer_binding", Level: selfcheck.LevelInfo, Message: fmt.Sprintf("mappings are bound to peer %s (%s:%d)", binding.PeerNodeID, binding.PeerSignalingIP, binding.PeerSignalingPort), Suggestion: "映射规则默认绑定该对端节点（只读）", ActionHint: "如需切换，请先在 peer 配置页调整唯一启用对端。"})
		}
	}
	if d.nodeStore != nil {
		report.Overall, report.Summary = summarizeSelfCheckItems(report.Items)
	}
	if level := strings.TrimSpace(r.URL.Query().Get("level")); level != "" {
		report.Items = filterSelfCheckItemsByLevel(report.Items, level)
		report.Overall, report.Summary = summarizeSelfCheckItems(report.Items)
	}
	runtimeSecurity := currentManagementSecurityRuntime(d)
	authLevel := selfcheck.LevelWarn
	authMessage := "管理面认证未启用，当前仍依赖专网隔离或前置网关限制访问。"
	authSuggestion := "建议配置 GATEWAY_ADMIN_TOKEN，并在前端浏览器本地管理会话中录入令牌。"
	authHint := "专网交付前至少启用管理令牌与管理网 CIDR 白名单。"
	if runtimeSecurity.Enforced {
		authLevel = selfcheck.LevelInfo
		authMessage = "管理面认证已启用。"
		authSuggestion = "定期轮换 GATEWAY_ADMIN_TOKEN，并配套审计访问源。"
		authHint = "生产环境建议结合跳板机与反向代理进一步收敛访问路径。"
	}
	if runtimeSecurity.RequireMFA && !runtimeSecurity.MFAConfigured {
		authLevel = selfcheck.LevelError
		authMessage = "管理面要求 MFA，但未配置 GATEWAY_ADMIN_MFA_CODE。"
		authSuggestion = "请在运行环境注入 GATEWAY_ADMIN_MFA_CODE，再启用 admin_require_mfa。"
		authHint = "修复后重新执行 /api/selfcheck。"
	}
	report.Items = append(report.Items,
		selfcheck.Item{Name: "management_auth", Level: authLevel, Message: authMessage, Suggestion: authSuggestion, ActionHint: authHint},
		selfcheck.Item{Name: "config_encryption", Level: map[bool]selfcheck.Level{true: selfcheck.LevelInfo, false: selfcheck.LevelWarn}[runtimeSecurity.ConfigKeyEnabled], Message: map[bool]string{true: "配置落盘加密已启用。", false: "配置落盘加密未启用。"}[runtimeSecurity.ConfigKeyEnabled], Suggestion: map[bool]string{true: "继续妥善保管 GATEWAY_CONFIG_KEY 并纳入轮换制度。", false: "建议配置 GATEWAY_CONFIG_KEY，保护隧道配置与授权文件。"}[runtimeSecurity.ConfigKeyEnabled], ActionHint: "变更后重启并复核。"},
		selfcheck.Item{Name: "tunnel_signer_secret", Level: map[bool]selfcheck.Level{true: selfcheck.LevelInfo, false: selfcheck.LevelWarn}[runtimeSecurity.SignerSecretManaged], Message: map[bool]string{true: "隧道签名密钥已外置。", false: "隧道签名密钥仍使用默认内置值。"}[runtimeSecurity.SignerSecretManaged], Suggestion: map[bool]string{true: "继续按环境分离配置并纳入轮换。", false: "请配置 GATEWAY_TUNNEL_SIGNER_SECRET。"}[runtimeSecurity.SignerSecretManaged], ActionHint: "生产环境必须使用环境独立密钥。"},
	)
	report.Overall, report.Summary = summarizeSelfCheckItems(report.Items)

	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: report})
}

func filterSelfCheckItemsByLevel(items []selfcheck.Item, raw string) []selfcheck.Item {
	allowed := map[selfcheck.Level]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		l := selfcheck.Level(strings.ToLower(strings.TrimSpace(part)))
		if l == selfcheck.LevelInfo || l == selfcheck.LevelWarn || l == selfcheck.LevelError {
			allowed[l] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return items
	}
	out := make([]selfcheck.Item, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.Level]; ok {
			out = append(out, item)
		}
	}
	return out
}

func summarizeSelfCheckItems(items []selfcheck.Item) (selfcheck.Level, selfcheck.Summary) {
	summary := selfcheck.Summary{}
	overall := selfcheck.LevelInfo
	for _, item := range items {
		switch item.Level {
		case selfcheck.LevelError:
			summary.Error++
			overall = selfcheck.LevelError
		case selfcheck.LevelWarn:
			summary.Warn++
			if overall != selfcheck.LevelError {
				overall = selfcheck.LevelWarn
			}
		default:
			summary.Info++
		}
	}
	return overall, summary
}

func (d *handlerDeps) handleNodeNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	status := d.networkStatusFunc(r.Context())
	if d.nodeStore != nil {
		compat := d.compatibilitySnapshot(r.Context())
		status.CurrentNetworkMode = compat.CurrentNetworkMode
		status.CurrentCapability = compat.CurrentCapability
		status.CompatibilityStatus = compat.CompatibilityCheck
		binding, bindErr := d.currentPeerBinding()
		status.BoundPeer = binding
		if bindErr != nil {
			status.PeerBindingError = bindErr.Error()
		}
	}
	d.recordOpsAudit(r, readOperator(r), "QUERY_NODE_NETWORK_STATUS", map[string]any{"path": r.URL.Path})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: status})
}

func (d *handlerDeps) handleStartupSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if d.startupSummaryFn == nil {
		writeError(w, http.StatusNotImplemented, "STARTUP_SUMMARY_NOT_ENABLED", "startup summary provider not configured")
		return
	}
	summary := d.startupSummaryFn(r.Context())
	nodeSource := d.nodeConfigSource
	if strings.TrimSpace(nodeSource) == "" {
		nodeSource = dataSourceLabel("", "node_config.json")
	}
	mappingSource := d.mappingSource
	if strings.TrimSpace(mappingSource) == "" {
		mappingSource = dataSourceLabel("", "tunnel_mappings.json")
	}
	if strings.TrimSpace(summary.DataSources.NodeConfig) == "" {
		summary.DataSources.NodeConfig = nodeSource
	}
	if strings.TrimSpace(summary.DataSources.Peers) == "" {
		summary.DataSources.Peers = nodeSource
	}
	if strings.TrimSpace(summary.DataSources.Mappings) == "" {
		summary.DataSources.Mappings = mappingSource
	}
	if strings.TrimSpace(summary.DataSources.Mode) == "" {
		summary.DataSources.Mode = "runtime_network_config"
	}
	if strings.TrimSpace(summary.DataSources.Capability) == "" {
		summary.DataSources.Capability = "derived_from_network_mode"
	}
	session := d.sessionMgr.Snapshot()
	summary.RegistrationStatus = session.RegistrationStatus
	summary.HeartbeatStatus = session.HeartbeatStatus
	summary.LastRegisterTime = session.LastRegisterTime
	summary.LastHeartbeatTime = session.LastHeartbeatTime
	summary.LastFailureReason = session.LastFailureReason
	summary.NextRetryTime = session.NextRetryTime
	if d.nodeStore != nil {
		compat := d.compatibilitySnapshot(r.Context())
		summary.CurrentNetworkMode = compat.CurrentNetworkMode
		summary.CurrentCapability = compat.CurrentCapability
		summary.CompatibilityStatus = compat.CompatibilityCheck
		binding, bindErr := d.currentPeerBinding()
		if binding != nil {
			summary.BoundPeer = &startupsummary.PeerBinding{PeerNodeID: binding.PeerNodeID, PeerName: binding.PeerName, PeerSignalingIP: binding.PeerSignalingIP, PeerSignalingPort: binding.PeerSignalingPort}
		}
		if bindErr != nil {
			summary.PeerBindingError = bindErr.Error()
		}
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: summary})
}

func deriveTunnelStatus(status NodeNetworkStatus) (string, string) {
	if status.SIP.ListenPort <= 0 || status.RTP.PortStart <= 0 || status.RTP.PortEnd <= 0 {
		return "disconnected", "SIP 或 RTP 监听未就绪"
	}
	if status.RTP.AvailablePorts <= 0 {
		return "degraded", "RTP 端口池已耗尽"
	}
	if len(status.RecentNetworkErrors) > 0 {
		return "degraded", status.RecentNetworkErrors[0]
	}
	return "connected", "SIP 控制面与 RTP 文件面链路正常"
}

func (d *handlerDeps) handleSystemStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	status := d.networkStatusFunc(r.Context())
	tunnelStatus, reason := deriveTunnelStatus(status)
	items := d.decorateMappings(d.mappings.List())
	mappingTotal := len(items)
	mappingAbnormalTotal := 0
	latestMappingErrorReason := ""
	for _, item := range items {
		if item.LinkStatus == mappingStateStartFailed || item.LinkStatus == mappingStateInterrupted {
			mappingAbnormalTotal++
			if latestMappingErrorReason == "" {
				latestMappingErrorReason = fmt.Sprintf("%s：%s", item.MappingID, item.StatusReason)
			}
		}
	}
	if status.PeerBindingError != "" {
		mappingAbnormalTotal = mappingTotal
		latestMappingErrorReason = status.PeerBindingError
	} else if tunnelStatus != "connected" && mappingAbnormalTotal == 0 {
		mappingAbnormalTotal = mappingTotal
		latestMappingErrorReason = reason
	}
	session := d.sessionMgr.Snapshot()
	resp := SystemStatusResponse{
		TunnelStatus:             tunnelStatus,
		ConnectionReason:         reason,
		NetworkMode:              status.NetworkMode,
		RegistrationStatus:       session.RegistrationStatus,
		HeartbeatStatus:          session.HeartbeatStatus,
		LastRegisterTime:         session.LastRegisterTime,
		LastHeartbeatTime:        session.LastHeartbeatTime,
		LastFailureReason:        session.LastFailureReason,
		NextRetryTime:            session.NextRetryTime,
		MappingTotal:             mappingTotal,
		MappingAbnormalTotal:     mappingAbnormalTotal,
		LatestMappingErrorReason: latestMappingErrorReason,
		BoundPeer:                status.BoundPeer,
		PeerBindingError:         status.PeerBindingError,
		Capability: SystemStatusCapability{
			SupportsSmallRequestBody:        status.Capability.SupportsSmallRequestBody,
			SupportsLargeResponseBody:       status.Capability.SupportsLargeResponseBody,
			SupportsStreamingResponse:       status.Capability.SupportsStreamingResponse,
			SupportsLargeFileUpload:         status.Capability.SupportsLargeRequestBody,
			SupportsBidirectionalHTTPTunnel: status.Capability.SupportsBidirectionalHTTPTunnel,
		},
	}
	d.recordOpsAudit(r, readOperator(r), "QUERY_SYSTEM_STATUS", map[string]any{"path": r.URL.Path})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
}

func (d *handlerDeps) handleDiagnosticsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	requestID := strings.TrimSpace(r.URL.Query().Get("request_id"))
	traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))
	status := d.networkStatusFunc(r.Context())
	compat := nodeconfig.CompatibilityStatus{}
	if d.nodeStore != nil {
		compat = d.compatibilitySnapshot(r.Context())
	}
	nodeID := "gateway"
	if d.nodeStore != nil {
		if id := strings.TrimSpace(d.nodeStore.GetLocalNode().NodeID); id != "" {
			nodeID = id
		}
	}
	generatedAt := time.Now().UTC()
	jobID := generatedAt.Format("150405")
	nodeToken := safeExportToken(nodeID)
	prefix := fmt.Sprintf("diag_%s_%s", nodeToken, generatedAt.Format("20060102T150405Z"))
	if requestID != "" {
		prefix += "_req_" + safeExportToken(requestID)
	}
	if traceID != "" {
		prefix += "_trace_" + safeExportToken(traceID)
	}
	prefix += "_" + jobID

	tasks, _ := d.repo.ListTasks(r.Context(), repository.TaskFilter{Status: repository.TaskStatusFailed, RequestID: requestID, TraceID: traceID, Limit: 20})
	taskSummary := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		taskSummary = append(taskSummary, map[string]any{
			"id":          task.ID,
			"request_id":  task.RequestID,
			"trace_id":    task.TraceID,
			"api_code":    task.APICode,
			"status":      task.Status,
			"result_code": task.ResultCode,
			"updated_at":  formatTimestamp(task.UpdatedAt),
			"last_error":  maskText(task.LastError),
		})
	}

	audits, _ := d.audit.List(r.Context(), observability.AuditQuery{RequestID: requestID, TraceID: traceID, Limit: 50})
	rateLimitHits := make([]map[string]any, 0, 20)
	for _, evt := range audits {
		if !strings.Contains(strings.ToUpper(evt.FinalResult), "RATE_LIMIT") {
			continue
		}
		rateLimitHits = append(rateLimitHits, map[string]any{
			"when":       formatTimestamp(evt.When),
			"request_id": evt.Core.RequestID,
			"trace_id":   evt.Core.TraceID,
			"api_code":   evt.Core.APICode,
			"result":     maskText(evt.FinalResult),
		})
	}

	readmeLines := []string{
		"诊断包文件说明（字段已脱敏，可用于人工排障）",
		"00_startup_summary.json: 统一启动与运行摘要，供日志/API/UI/诊断复用。",
		"01_transport_config.json: 当前 NetworkMode/Capability/TunnelTransportPlan + SIP/RTP transport 与关键网络参数快照。",
		"01_transport_config.json.data_sources: 明确 node/peers/mappings/mode/capability 的真实来源。",
		"02_connection_stats_snapshot.json: SIP/RTP 连接计数与错误累计值。",
		"03_port_pool_status.json: RTP 端口池使用情况与分配失败累计值。",
		"04_transport_error_summary.json: 最近 transport 绑定/网络错误摘要。",
		"05_task_failure_summary.json: 最近失败任务摘要，支持 request_id/trace_id 定向过滤。",
		"06_rate_limit_hit_summary.json: 最近 rate limit 命中记录，支持 request_id/trace_id 定向过滤。",
		"07_profile_entry.json: pprof 采集入口与启用状态（不包含凭据）。",
	}
	nodeSource := d.nodeConfigSource
	if strings.TrimSpace(nodeSource) == "" {
		nodeSource = dataSourceLabel("", "node_config.json")
	}
	mappingSource := d.mappingSource
	if strings.TrimSpace(mappingSource) == "" {
		mappingSource = dataSourceLabel("", "tunnel_mappings.json")
	}

	files := []DiagFile{
		{Name: "00_startup_summary.json", Description: "统一启动与运行摘要", Content: d.startupSummaryFn(r.Context())},
		{Name: "README.md", Description: "诊断包文件说明", Content: map[string]any{"filters": map[string]any{"request_id": requestID, "trace_id": traceID}, "files": readmeLines}},
		{Name: "01_transport_config.json", Description: "当前 transport 配置", Content: map[string]any{"network_mode": status.NetworkMode, "capability": status.Capability, "capability_summary": status.CapabilitySummary, "transport_plan": status.TransportPlan, "current_network_mode": compat.CurrentNetworkMode, "current_capability": compat.CurrentCapability, "compatibility_status": compat.CompatibilityCheck, "sip": map[string]any{"listen_ip": status.SIP.ListenIP, "listen_port": status.SIP.ListenPort, "transport": status.SIP.Transport}, "rtp": map[string]any{"listen_ip": status.RTP.ListenIP, "port_start": status.RTP.PortStart, "port_end": status.RTP.PortEnd, "transport": status.RTP.Transport}, "mappings_capability_validation": d.validateMappingsAgainstCapability(d.mappings.List()), "data_sources": map[string]any{"node_config": nodeSource, "peers": nodeSource, "mappings": mappingSource, "mode": "runtime_network_config", "capability": "derived_from_network_mode"}}},
		{Name: "02_connection_stats_snapshot.json", Description: "连接统计快照", Content: map[string]any{"sip": map[string]any{"current_sessions": status.SIP.CurrentSessions, "current_connections": status.SIP.CurrentConnections, "accepted_connections_total": status.SIP.AcceptedConnectionsTotal, "closed_connections_total": status.SIP.ClosedConnectionsTotal, "read_timeout_total": status.SIP.ReadTimeoutTotal, "write_timeout_total": status.SIP.WriteTimeoutTotal, "connection_error_total": status.SIP.ConnectionErrorTotal}, "rtp": map[string]any{"active_transfers": status.RTP.ActiveTransfers, "rtp_tcp_sessions_current": status.RTP.TCPSessionsCurrent, "rtp_tcp_sessions_total": status.RTP.TCPSessionsTotal, "rtp_tcp_read_errors_total": status.RTP.TCPReadErrorsTotal, "rtp_tcp_write_errors_total": status.RTP.TCPWriteErrorsTotal}}},
		{Name: "03_port_pool_status.json", Description: "端口池状态", Content: map[string]any{"used_ports": status.RTP.UsedPorts, "available_ports": status.RTP.AvailablePorts, "rtp_port_pool_total": status.RTP.PortPoolTotal, "rtp_port_pool_used": status.RTP.PortPoolUsed, "rtp_port_alloc_fail_total": status.RTP.PortAllocFailTotal}},
		{Name: "04_transport_error_summary.json", Description: "最近 transport 错误摘要", Content: map[string]any{"recent_bind_errors": maskStringSlice(status.RecentBindErrors), "recent_network_errors": maskStringSlice(status.RecentNetworkErrors)}},
		{Name: "05_task_failure_summary.json", Description: "最近 task failure 摘要", Content: taskSummary},
		{Name: "06_rate_limit_hit_summary.json", Description: "最近 rate limit 命中摘要", Content: rateLimitHits},
		{Name: "07_profile_entry.json", Description: "profile 采集入口信息（如果启用）", Content: map[string]any{"enabled": strings.EqualFold(strings.TrimSpace(os.Getenv("GATEWAY_PPROF_ENABLED")), "true") || strings.TrimSpace(os.Getenv("GATEWAY_PPROF_ENABLED")) == "1", "listen_address": strings.TrimSpace(os.Getenv("GATEWAY_PPROF_LISTEN_ADDR")), "profile_url": "/debug/pprof/profile"}},
	}
	if d.sqliteStore != nil {
		_ = d.sqliteStore.SaveDiagnosticRecord(r.Context(), jobID, nodeID, prefix+".zip", files)
	}
	d.recordOpsAudit(r, readOperator(r), "EXPORT_DIAGNOSTICS", map[string]any{"request_id": requestID, "trace_id": traceID})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: DiagnosticExportData{GeneratedAt: generatedAt, JobID: jobID, NodeID: nodeID, RequestID: requestID, TraceID: traceID, FileName: prefix + ".zip", OutputDir: prefix, Files: files}})
}

func maskText(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) <= 12 {
		return "***"
	}
	return v[:12] + "***"
}

func maskStringSlice(values []string) []string {
	masked := make([]string, 0, len(values))
	for _, item := range values {
		masked = append(masked, maskText(item))
	}
	return masked
}

func safeExportToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "na"
	}
	safe := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		if r == '-' || r == '_' {
			return '_'
		}
		return '_'
	}, v)
	return strings.Trim(safe, "_")
}
