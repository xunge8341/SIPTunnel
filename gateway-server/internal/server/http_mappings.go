package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"siptunnel/internal/observability"
	filerepo "siptunnel/internal/repository/file"
	"siptunnel/loadtest"
)

func bindAddrLikelyConflict(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	return a == "0.0.0.0" || b == "0.0.0.0" || a == "::" || b == "::"
}

func (d *handlerDeps) validateMappingPorts(mapping TunnelMapping) error {
	local := d.nodeStore.GetLocalNode()
	if mapping.LocalBindPort == local.SIPListenPort {
		return fmt.Errorf("映射监听端口 %d 不能与本端 SIP 监听端口相同", mapping.LocalBindPort)
	}
	if mapping.LocalBindPort >= local.RTPPortStart && mapping.LocalBindPort <= local.RTPPortEnd {
		return fmt.Errorf("映射监听端口 %d 不能落在本端 RTP 端口范围 [%d,%d] 内；RTP 接收端口由系统从该范围内动态分配", mapping.LocalBindPort, local.RTPPortStart, local.RTPPortEnd)
	}
	if local.MappingPortStart > 0 && local.MappingPortEnd > 0 {
		if mapping.LocalBindPort < local.MappingPortStart || mapping.LocalBindPort > local.MappingPortEnd {
			return fmt.Errorf("映射监听端口 %d 必须落在本地隧道映射端口范围 [%d,%d] 内", mapping.LocalBindPort, local.MappingPortStart, local.MappingPortEnd)
		}
	}
	if d.uiConfig.Enabled && d.uiConfig.Mode == "embedded" && mapping.LocalBindPort == d.uiConfig.ListenPort && bindAddrLikelyConflict(mapping.LocalBindIP, d.uiConfig.ListenIP) {
		return fmt.Errorf("映射监听端口 %d 不能与网关 UI 监听 %s:%d 冲突", mapping.LocalBindPort, d.uiConfig.ListenIP, d.uiConfig.ListenPort)
	}
	return nil
}

func (d *handlerDeps) handleMappings(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/mappings/")
	if r.URL.Path == "/api/mappings" || r.URL.Path == "/api/mappings/" {
		id = ""
	}
	switch r.Method {
	case http.MethodGet:
		if id != "" {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		items := d.mappings.List()
		validation := d.validateMappingsAgainstCapability(items)
		binding, bindErr := d.currentPeerBinding()
		resp := mappingListResponse{Items: d.decorateMappings(items), BoundPeer: binding, Warnings: validation.Warnings}
		if bindErr != nil {
			resp.BindingError = bindErr.Error()
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
	case http.MethodPost:
		if id != "" {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		var req TunnelMapping
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		req.Normalize()
		if err := d.enforceCurrentPeerBinding(&req); err != nil {
			writeError(w, http.StatusBadRequest, "PEER_BINDING_INVALID", err.Error())
			return
		}
		if err := d.validateMappingPorts(req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		validation := d.validateMappingAgainstCapability(req)
		if validation.HasErrors() {
			writeError(w, http.StatusBadRequest, "MAPPING_CAPABILITY_INVALID", strings.Join(validation.Errors, "; "))
			return
		}
		created, err := d.mappings.Create(req)
		if err != nil {
			status := http.StatusBadRequest
			code := "INVALID_ARGUMENT"
			if errors.Is(err, filerepo.ErrMappingExists) {
				status = http.StatusConflict
				code = "MAPPING_EXISTS"
			}
			writeError(w, status, code, err.Error())
			return
		}
		d.onLocalCatalogChanged()
		if d.logger != nil {
			fields := observability.CoreFieldsFromContext(r.Context())
			fields.ResultCode = "OK"
			status := NodeNetworkStatus{}
			if d.networkStatusFunc != nil {
				status = d.networkStatusFunc(r.Context())
			}
			d.logger.Info(r.Context(), "mapping_config_applied", fields,
				"action", "create",
				"mapping_id", created.MappingID,
				"mapping_name", created.Name,
				"local_endpoint", mappingLocalEndpoint(created),
				"local_base_path", created.LocalBasePath,
				"target_endpoint", mappingTargetEndpoint(created),
				"allowed_methods", strings.Join(created.AllowedMethods, ","),
				"response_mode", created.ResponseMode,
				"max_request_body_bytes", created.MaxRequestBodyBytes,
				"max_response_body_bytes", created.MaxResponseBodyBytes,
				"max_inline_response_body", created.MaxInlineResponseBody,
				"network_mode", status.NetworkMode,
				"supports_large_request_body", status.Capability.SupportsLargeRequestBody,
				"supports_large_response_body", status.Capability.SupportsLargeResponseBody,
				"warnings", validation.Warnings,
			)
		}
		d.recordOpsAudit(r, readOperator(r), "CREATE_MAPPING", map[string]any{"mapping_id": created.MappingID})
		writeJSON(w, http.StatusCreated, responseEnvelope{Code: "OK", Message: "success", Data: MappingWithWarnings{Mapping: d.decorateMapping(created), BoundPeer: bindingFromMapping(created), Warnings: validation.Warnings}})
	case http.MethodPut, http.MethodPatch:
		if id == "" {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "mapping id is required in path")
			return
		}
		var req TunnelMapping
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
			return
		}
		req.Normalize()
		if err := d.enforceCurrentPeerBinding(&req); err != nil {
			writeError(w, http.StatusBadRequest, "PEER_BINDING_INVALID", err.Error())
			return
		}
		if err := d.validateMappingPorts(req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		validation := d.validateMappingAgainstCapability(req)
		if validation.HasErrors() {
			writeError(w, http.StatusBadRequest, "MAPPING_CAPABILITY_INVALID", strings.Join(validation.Errors, "; "))
			return
		}
		updated, err := d.mappings.Update(id, req)
		if err != nil {
			status := http.StatusBadRequest
			code := "INVALID_ARGUMENT"
			if errors.Is(err, filerepo.ErrMappingNotFound) {
				status = http.StatusNotFound
				code = "MAPPING_NOT_FOUND"
			}
			writeError(w, status, code, err.Error())
			return
		}
		d.onLocalCatalogChanged()
		if d.logger != nil {
			fields := observability.CoreFieldsFromContext(r.Context())
			fields.ResultCode = "OK"
			status := NodeNetworkStatus{}
			if d.networkStatusFunc != nil {
				status = d.networkStatusFunc(r.Context())
			}
			d.logger.Info(r.Context(), "mapping_config_applied", fields,
				"action", "update",
				"mapping_id", updated.MappingID,
				"mapping_name", updated.Name,
				"local_endpoint", mappingLocalEndpoint(updated),
				"local_base_path", updated.LocalBasePath,
				"target_endpoint", mappingTargetEndpoint(updated),
				"allowed_methods", strings.Join(updated.AllowedMethods, ","),
				"response_mode", updated.ResponseMode,
				"max_request_body_bytes", updated.MaxRequestBodyBytes,
				"max_response_body_bytes", updated.MaxResponseBodyBytes,
				"max_inline_response_body", updated.MaxInlineResponseBody,
				"network_mode", status.NetworkMode,
				"supports_large_request_body", status.Capability.SupportsLargeRequestBody,
				"supports_large_response_body", status.Capability.SupportsLargeResponseBody,
				"warnings", validation.Warnings,
			)
		}
		d.recordOpsAudit(r, readOperator(r), "UPDATE_MAPPING", map[string]any{"mapping_id": updated.MappingID})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: MappingWithWarnings{Mapping: d.decorateMapping(updated), BoundPeer: bindingFromMapping(updated), Warnings: validation.Warnings}})
	case http.MethodDelete:
		if id == "" {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "mapping id is required in path")
			return
		}
		if err := d.mappings.Delete(id); err != nil {
			if errors.Is(err, filerepo.ErrMappingNotFound) {
				writeError(w, http.StatusNotFound, "MAPPING_NOT_FOUND", err.Error())
				return
			}
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		d.onLocalCatalogChanged()
		d.recordOpsAudit(r, readOperator(r), "DELETE_MAPPING", map[string]any{"mapping_id": id})
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success"})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleMappingTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	status := d.networkStatusFunc(r.Context())
	sip := d.checkSIPControlPath(r.Context(), status)
	rtp := d.checkRTPPortPool(status)
	localListeningPassed := sip.Passed && rtp.Passed
	session := d.sessionMgr.Snapshot()
	registrationNormal := strings.EqualFold(strings.TrimSpace(session.RegistrationStatus), "registered")
	heartbeatNormal := strings.EqualFold(strings.TrimSpace(session.HeartbeatStatus), "healthy")
	peerStage := d.checkPeerReachabilityStage(r.Context())
	sessionReady := registrationNormal && heartbeatNormal && peerStage.Passed
	forwardStage := d.checkMappingForwardReadinessStage(sessionReady)

	stages := []MappingTestStage{
		{Key: "local_listening", Name: "本地监听可用", Status: boolLabel(localListeningPassed, "passed", "failed"), Passed: localListeningPassed, Detail: fmt.Sprintf("SIP=%s；RTP=%s", sip.Detail, rtp.Detail), BlockingReason: firstNonEmpty(failedReason(sip), failedReason(rtp)), SuggestedAction: "检查本端 SIP 监听与 RTP 端口池配置。"},
		{Key: "registration", Name: "注册状态正常", Status: boolLabel(registrationNormal, "passed", "failed"), Passed: registrationNormal, Detail: fmt.Sprintf("当前注册状态：%s", normalizeValue(session.RegistrationStatus, "unknown")), BlockingReason: boolLabel(registrationNormal, "", normalizeValue(session.LastFailureReason, "注册尚未完成")), SuggestedAction: "检查鉴权参数并触发重新注册。"},
		{Key: "heartbeat", Name: "心跳状态正常", Status: boolLabel(heartbeatNormal, "passed", "failed"), Passed: heartbeatNormal, Detail: fmt.Sprintf("当前心跳状态：%s", normalizeValue(session.HeartbeatStatus, "unknown")), BlockingReason: boolLabel(heartbeatNormal, "", normalizeValue(session.LastFailureReason, "心跳未恢复健康")), SuggestedAction: "检查心跳周期、网络时延与丢包。"},
		peerStage,
		{Key: "session_ready", Name: "会话已准备", Status: ternary(sessionReady, "passed", "blocked"), Passed: sessionReady, Detail: "会话准备要求：注册正常 + 心跳正常 + 对端可达。", BlockingReason: blockingReasonsForSession(registrationNormal, heartbeatNormal, peerStage.Passed), SuggestedAction: "按前置阶段提示恢复会话条件后重试。"},
		forwardStage,
	}

	passed := allMappingStagesPassed(stages)
	result := MappingTestResponse{
		Passed:             passed,
		Status:             boolLabel(passed, "passed", "failed"),
		Stages:             stages,
		SignalingRequest:   boolLabel(localListeningPassed, "成功", "失败"),
		ResponseChannel:    boolLabel(rtp.Passed, "正常", "异常"),
		RegistrationStatus: boolLabel(registrationNormal, "正常", "未注册"),
	}

	if failed := firstFailedMappingStage(stages); failed != nil {
		result.FailureStage = failed.Name
		result.FailureReason = normalizeValue(failed.BlockingReason, failed.Detail)
		result.SuggestedAction = failed.SuggestedAction
	}

	d.recordOpsAudit(r, readOperator(r), "RUN_MAPPING_TEST", map[string]any{"status": result.Status, "failure_stage": result.FailureStage, "signaling_request": result.SignalingRequest, "response_channel": result.ResponseChannel, "registration_status": result.RegistrationStatus})
	logMappingTestResult(result)
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: result})
}

func (d *handlerDeps) checkPeerReachabilityStage(ctx context.Context) MappingTestStage {
	binding, err := d.currentPeerBinding()
	if err != nil {
		return MappingTestStage{Key: "peer_reachability", Name: "对端可达", Status: "failed", Passed: false, Detail: "对端绑定检查失败", BlockingReason: err.Error(), SuggestedAction: "在对端配置页面保持且仅保持一个启用对端。"}
	}
	if strings.TrimSpace(binding.PeerSignalingIP) == "" || binding.PeerSignalingPort <= 0 {
		return MappingTestStage{Key: "peer_reachability", Name: "对端可达", Status: "failed", Passed: false, Detail: "对端信令地址未配置", BlockingReason: "peer_signaling_ip 或 peer_signaling_port 未配置", SuggestedAction: "补齐对端信令地址后再测试。"}
	}
	status := d.networkStatusFunc(ctx)
	transport := strings.ToUpper(strings.TrimSpace(status.SIP.Transport))
	if transport == "" {
		transport = "TCP"
	}
	addr := net.JoinHostPort(strings.TrimSpace(binding.PeerSignalingIP), strconv.Itoa(binding.PeerSignalingPort))
	detail, probeErr := probeSignalingConnectivity(ctx, transport, addr)
	if probeErr != nil {
		return MappingTestStage{Key: "peer_reachability", Name: "对端可达", Status: "failed", Passed: false, Detail: detail, BlockingReason: probeErr.Error(), SuggestedAction: "检查对端进程、ACL、路由以及承载 transport 是否匹配。"}
	}
	return MappingTestStage{Key: "peer_reachability", Name: "对端可达", Status: "passed", Passed: true, Detail: detail}
}

func (d *handlerDeps) checkMappingForwardReadinessStage(sessionReady bool) MappingTestStage {
	if !sessionReady {
		return MappingTestStage{Key: "mapping_forward", Name: "映射转发准备就绪", Status: "blocked", Passed: false, Detail: "会话尚未准备完成，暂不执行转发准备判定", BlockingReason: "依赖阶段“会话已准备”未通过", SuggestedAction: "先恢复注册/心跳/对端可达后重试。"}
	}
	items := d.mappings.List()
	enabled := make([]TunnelMapping, 0, len(items))
	for _, item := range items {
		if item.Enabled {
			enabled = append(enabled, item)
		}
	}
	if len(enabled) == 0 {
		return MappingTestStage{Key: "mapping_forward", Name: "映射转发准备就绪", Status: "failed", Passed: false, Detail: "未找到启用的映射规则", BlockingReason: "至少需要一个 enabled=true 的映射规则", SuggestedAction: "启用至少一个映射规则后再执行联调。"}
	}
	runtime := d.runtime.Snapshot()
	notReady := make([]string, 0)
	for _, item := range enabled {
		rs, ok := runtime[item.MappingID]
		if !ok {
			notReady = append(notReady, fmt.Sprintf("%s: 运行时状态缺失", item.MappingID))
			continue
		}
		if rs.State != mappingStateListening && rs.State != mappingStateForwarding && rs.State != "connected" {
			notReady = append(notReady, fmt.Sprintf("%s: %s", item.MappingID, normalizeValue(rs.Reason, rs.State)))
		}
	}
	if len(notReady) > 0 {
		return MappingTestStage{Key: "mapping_forward", Name: "映射转发准备就绪", Status: "failed", Passed: false, Detail: fmt.Sprintf("共有 %d 条启用规则未就绪", len(notReady)), BlockingReason: strings.Join(notReady, "；"), SuggestedAction: suggestMappingForwardAction(notReady)}
	}
	return MappingTestStage{Key: "mapping_forward", Name: "映射转发准备就绪", Status: "passed", Passed: true, Detail: fmt.Sprintf("%d 条启用规则已进入监听态", len(enabled))}
}

func allMappingStagesPassed(stages []MappingTestStage) bool {
	for _, stage := range stages {
		if !stage.Passed {
			return false
		}
	}
	return true
}

func firstFailedMappingStage(stages []MappingTestStage) *MappingTestStage {
	for _, stage := range stages {
		if !stage.Passed {
			item := stage
			return &item
		}
	}
	return nil
}

func failedReason(item LinkTestItem) string {
	if item.Passed {
		return ""
	}
	return item.Detail
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func normalizeValue(v string, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func blockingReasonsForSession(registrationNormal, heartbeatNormal, peerReachable bool) string {
	reasons := make([]string, 0, 3)
	if !registrationNormal {
		reasons = append(reasons, "注册状态未就绪")
	}
	if !heartbeatNormal {
		reasons = append(reasons, "心跳状态未恢复")
	}
	if !peerReachable {
		reasons = append(reasons, "对端不可达")
	}
	if len(reasons) == 0 {
		return ""
	}
	return strings.Join(reasons, "；")
}

func suggestMappingForwardAction(reasons []string) string {
	joined := strings.ToLower(strings.Join(reasons, " "))
	switch {
	case strings.Contains(joined, "rtp port pool exhausted"):
		return "RTP 端口池已耗尽；请检查是否存在浏览器附带请求风暴（如 /favicon.ico）、并发会话过多，或扩大本端 RTP 端口范围 / 降低并发。"
	case strings.Contains(joined, "temporarily suppressed") || strings.Contains(joined, "暂时退避中"):
		return "上游目标已进入短路退避窗口；请优先检查真实 HTTP 地址、端口监听、IP 是否已变更，待退避窗口结束后再重试。"
	case strings.Contains(joined, "connection refused") || strings.Contains(joined, "actively refused") || strings.Contains(joined, "被拒绝连接"):
		return "真实 HTTP 目标拒绝连接；请检查下级域配置的 TargetURL、服务是否启动、端口是否监听，以及 IP/域名是否已变更。"
	case strings.Contains(joined, "dns") || strings.Contains(joined, "no such host"):
		return "真实 HTTP 目标域名解析失败；请检查 TargetURL 主机名、DNS 配置，或确认目标地址是否已变更。"
	case strings.Contains(joined, "timeout") || strings.Contains(joined, "deadline exceeded"):
		return "真实 HTTP 目标访问超时；请检查目标服务健康、网络链路、请求超时设置，或确认目标 IP 是否已变更。"
	case strings.Contains(joined, "wait response start"):
		return "对端已收到 INVITE/MESSAGE，但未在时限内回调响应起始控制消息；请检查下级资源真实 HTTP 目标、请求超时与回调 SIP 端口。"
	case strings.Contains(joined, "invite device") || strings.Contains(joined, "invite"):
		return "INVITE 建链未完成；请检查上级域到下级域的 SIP 信令端口、UDP/TCP 传输配置与防火墙。"
	default:
		return "修复映射监听失败/中断后重试。"
	}
}

func ternary(ok bool, pass, fail string) string {
	if ok {
		return pass
	}
	return fail
}

func (d *handlerDeps) listLegacyRoutesFromMappings() []OpsRoute {
	items := d.mappings.List()
	legacy := make([]OpsRoute, 0, len(items))
	for _, item := range items {
		method := "ANY"
		if len(item.AllowedMethods) > 0 {
			method = item.AllowedMethods[0]
		}
		legacy = append(legacy, OpsRoute{APICode: item.MappingID, HTTPMethod: method, HTTPPath: item.LocalBasePath, Enabled: item.Enabled})
	}
	return legacy
}

func (d *handlerDeps) decorateMappings(items []TunnelMapping) []TunnelMappingView {
	runtime := d.runtime.Snapshot()
	analysis := d.aggregateAccessStats(context.Background())
	out := make([]TunnelMappingView, 0, len(items))
	for _, item := range items {
		out = append(out, d.decorateMappingWithRuntime(item, runtime, analysis))
	}
	return out
}

func (d *handlerDeps) decorateMapping(item TunnelMapping) TunnelMappingView {
	runtime := d.runtime.Snapshot()
	analysis := d.aggregateAccessStats(context.Background())
	return d.decorateMappingWithRuntime(item, runtime, analysis)
}

func (d *handlerDeps) decorateMappingWithRuntime(item TunnelMapping, runtime map[string]mappingRuntimeStatus, analysis accessAnalysis) TunnelMappingView {
	view := TunnelMappingView{TunnelMapping: item, RequestCount: analysis.CountByMapping[item.MappingID], FailureCount: analysis.FailedByMapping[item.MappingID], AvgLatencyMS: analysis.AvgLatencyByMap[item.MappingID]}
	if rs, ok := runtime[item.MappingID]; ok {
		view.LinkStatus = rs.State
		view.LinkStatusText, view.StatusReason, view.SuggestedAction = mappingStatusDiagnosis(rs.State, rs.Reason)
		view.FailureReason = view.StatusReason
		return view
	}
	if item.Enabled {
		view.LinkStatus = mappingStateStartFailed
		view.LinkStatusText, view.StatusReason, view.SuggestedAction = mappingStatusDiagnosis(mappingStateStartFailed, "监听状态未知，请检查运行时管理器")
		view.FailureReason = view.StatusReason
		return view
	}
	view.LinkStatus = mappingStateDisabled
	view.LinkStatusText, view.StatusReason, view.SuggestedAction = mappingStatusDiagnosis(mappingStateDisabled, "映射未启用")
	view.FailureReason = view.StatusReason
	return view
}

func mappingStatusDiagnosis(state, reason string) (statusText, failureReason, suggestedAction string) {
	trimmedReason := strings.TrimSpace(reason)
	switch state {
	case mappingStateDisabled:
		return "未启用", "映射规则未启用。", "按需开启规则后再观察链路状态。"
	case mappingStateListening:
		return "监听中", "本端入口监听已建立，等待业务请求。", "可发起联调请求，确认转发链路状态。"
	case mappingStateForwarding:
		return "转发中", normalizeValue(trimmedReason, "映射链路正在转发请求。"), "持续观察吞吐与延迟指标，确认请求成功率。"
	case mappingStateDegraded:
		return "降级", normalizeValue(trimmedReason, "映射链路出现转发异常。"), "检查对端目标可达性、超时与请求体大小配置后重试。"
	case "connected":
		return "已连接", "映射链路已连接。", "无需处理，持续观察心跳与延迟指标。"
	case mappingStateInterrupted:
		if trimmedReason == "" {
			trimmedReason = "监听线程异常中断。"
		}
		return "异常", trimmedReason, "查看节点状态与最近网络错误，恢复后重新启用映射规则。"
	case mappingStateStartFailed:
		if trimmedReason == "" {
			trimmedReason = "映射启动失败。"
		}
		return "启动失败", trimmedReason, "检查本端监听地址、端口占用与权限，再执行重启。"
	default:
		if trimmedReason == "" {
			trimmedReason = "映射链路状态异常。"
		}
		return "异常", trimmedReason, "检查注册、心跳与对端可达性，定位后再恢复流量。"
	}
}

func boolLabel(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

func (d *handlerDeps) handleCapacityRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var req capacityAssessmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid json body")
		return
	}
	if strings.TrimSpace(req.SummaryFile) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "summary_file is required")
		return
	}
	report, err := loadtest.LoadReportFromSummary(req.SummaryFile)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", fmt.Sprintf("read summary failed: %v", err))
		return
	}
	d.mu.RLock()
	limits := d.limits
	d.mu.RUnlock()
	status := d.networkStatusFunc(r.Context())
	current := loadtest.CapacityCurrentConfig{
		CommandMaxConcurrent:      fallbackPositive(req.Current.CommandMaxConcurrent, limits.MaxConcurrent),
		FileTransferMaxConcurrent: fallbackPositive(req.Current.FileTransferMaxConcurrent, status.RTP.ActiveTransfers),
		RTPPortPoolSize:           fallbackPositive(req.Current.RTPPortPoolSize, status.RTP.PortPoolTotal),
		MaxConnections:            fallbackPositive(req.Current.MaxConnections, status.SIP.MaxConnections),
		RateLimitRPS:              fallbackPositive(req.Current.RateLimitRPS, limits.RPS),
		RateLimitBurst:            fallbackPositive(req.Current.RateLimitBurst, limits.Burst),
	}
	assessment := loadtest.AssessCapacity(report, current)
	response := map[string]any{
		"assessment": assessment,
		"summary": map[string]any{
			"run_id":       report.RunID,
			"generated_at": formatTimestamp(report.Generated),
			"targets":      len(report.Summaries),
		},
	}
	d.recordOpsAudit(r, readOperator(r), "QUERY_CAPACITY_RECOMMENDATION", map[string]any{"summary_file": req.SummaryFile})
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: response})
}
