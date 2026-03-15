package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"siptunnel/internal/repository"
)

type dashboardOpsSummary struct {
	TopMappings         []summaryItem `json:"top_mappings"`
	TopSourceIPs        []summaryItem `json:"top_source_ips"`
	TopFailedMappings   []summaryItem `json:"top_failed_mappings"`
	TopFailedSourceIPs  []summaryItem `json:"top_failed_source_ips"`
	RateLimitStatus     string        `json:"rate_limit_status"`
	CircuitBreakerState string        `json:"circuit_breaker_state"`
	ProtectionStatus    string        `json:"protection_status"`
}

type dashboardSummary struct {
	SystemHealth        string `json:"system_health"`
	ActiveConnections   int    `json:"active_connections"`
	MappingTotal        int    `json:"mapping_total"`
	MappingErrorCount   int    `json:"mapping_error_count"`
	RecentFailureCount  int    `json:"recent_failure_count"`
	RateLimitState      string `json:"rate_limit_state"`
	CircuitBreakerState string `json:"circuit_breaker_state"`
}

type protectionState struct {
	AlertRules          []string `json:"alert_rules"`
	RateLimitRules      []string `json:"rate_limit_rules"`
	CircuitBreakerRules []string `json:"circuit_breaker_rules"`
	CurrentTriggered    []string `json:"current_triggered"`
	LastTriggeredTime   string   `json:"last_triggered_time"`
	LastTriggeredTarget string   `json:"last_triggered_target"`
}

type securityCenterState struct {
	LicenseStatus      string   `json:"license_status"`
	ExpiryTime         string   `json:"expiry_time"`
	LicensedFeatures   []string `json:"licensed_features"`
	LastValidation     string   `json:"last_validation"`
	ManagementSecurity string   `json:"management_security"`
	SigningAlgorithm   string   `json:"signing_algorithm"`
}

type nodeTunnelWorkspace struct {
	LocalNode          NodeConfigEndpoint      `json:"local_node"`
	PeerNode           NodeConfigEndpoint      `json:"peer_node"`
	NetworkMode        string                  `json:"network_mode"`
	CapabilityMatrix   []map[string]any        `json:"capability_matrix"`
	SIPCapability      map[string]any          `json:"sip_capability"`
	RTPCapability      map[string]any          `json:"rtp_capability"`
	SessionSettings    TunnelConfigPayload     `json:"session_settings"`
	SecuritySettings   SecuritySettingsPayload `json:"security_settings"`
	EncryptionSettings map[string]string       `json:"encryption_settings"`
}

type summaryItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func defaultSystemSettings(d handlerDeps, sqlitePath string) SystemSettingsPayload {
	if strings.TrimSpace(sqlitePath) == "" {
		sqlitePath = "./data/final/gateway.db"
	}
	return SystemSettingsPayload{
		SQLitePath:           sqlitePath,
		LogCleanupCron:       "*/30 * * * *",
		MaxTaskAgeDays:       7,
		MaxTaskRecords:       20000,
		MaxAccessLogAgeDays:  7,
		MaxAccessLogRecords:  20000,
		MaxAuditAgeDays:      30,
		MaxAuditRecords:      50000,
		MaxDiagnosticAgeDays: 15,
		MaxDiagnosticRecords: 2000,
		MaxLoadtestAgeDays:   15,
		MaxLoadtestRecords:   2000,
		AdminAllowCIDR:       "127.0.0.1/32",
		AdminRequireMFA:      false,
		CleanerLastResult:    "未执行",
	}
}

func (d *handlerDeps) handleSystemSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		d.mu.RLock()
		resp := d.systemSettings
		d.mu.RUnlock()
		if d.cleaner != nil {
			resp.CleanerLastRunAt = d.cleaner.LastRunAt
			resp.CleanerLastResult = d.cleaner.LastResult
			resp.CleanerLastRemovedRecords = d.cleaner.LastRemoved
		}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: resp})
	case http.MethodPut, http.MethodPost:
		var req SystemSettingsPayload
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		req.SQLitePath = strings.TrimSpace(req.SQLitePath)
		if req.SQLitePath == "" {
			req.SQLitePath = d.systemSettings.SQLitePath
		}
		d.mu.Lock()
		d.systemSettings = req
		d.mu.Unlock()
		_ = saveJSON(d.systemPath, req)
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: req})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) handleAccessLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize = 50
	}
	status := repository.TaskStatus(strings.TrimSpace(q.Get("status")))
	mapping := strings.TrimSpace(q.Get("mapping"))
	sourceIP := strings.TrimSpace(q.Get("source_ip"))
	methodFilter := strings.ToUpper(strings.TrimSpace(q.Get("method")))
	slowOnly := strings.EqualFold(strings.TrimSpace(q.Get("slow_only")), "true")
	tasks, _ := d.repo.ListTasks(r.Context(), repository.TaskFilter{TaskType: repository.TaskTypeCommand, Status: status, RequestID: q.Get("request_id"), TraceID: q.Get("trace_id"), Limit: pageSize, Offset: (page - 1) * pageSize})
	items := make([]AccessLogEntry, 0, len(tasks))
	for _, t := range tasks {
		entry := mapTaskToAccessLogEntry(t)
		if mapping != "" && entry.MappingName != mapping {
			continue
		}
		if sourceIP != "" && !strings.Contains(entry.SourceIP, sourceIP) {
			continue
		}
		if methodFilter != "" && entry.Method != methodFilter {
			continue
		}
		if slowOnly && entry.DurationMS < 500 {
			continue
		}
		items = append(items, entry)
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: listData[AccessLogEntry]{Items: items, Pagination: pagination{Page: page, PageSize: pageSize, Total: len(items)}}})
}

func mapTaskToAccessLogEntry(t repository.Task) AccessLogEntry {
	method := "POST"
	path := "/api/" + strings.TrimSpace(t.APICode)
	if strings.TrimSpace(t.APICode) == "" {
		path = "/unknown"
	}
	statusCode := 200
	if t.Status == repository.TaskStatusFailed || t.Status == repository.TaskStatusDeadLettered {
		statusCode = 500
	}
	return AccessLogEntry{ID: t.ID, OccurredAt: t.UpdatedAt.UTC().Format(time.RFC3339), MappingName: defaultString(t.APICode, "未命名映射"), SourceIP: defaultString(t.SourceSystem, "unknown"), Method: method, Path: path, StatusCode: statusCode, DurationMS: t.UpdatedAt.Sub(t.CreatedAt).Milliseconds(), FailureReason: t.LastError, RequestID: t.RequestID, TraceID: t.TraceID}
}

func (d *handlerDeps) aggregateAccessStats(ctx context.Context) (map[string]int, map[string]int, map[string]int, map[string]int, int) {
	tasks, _ := d.repo.ListTasks(ctx, repository.TaskFilter{TaskType: repository.TaskTypeCommand, Limit: 1000})
	countByMapping := map[string]int{}
	countBySource := map[string]int{}
	failedByMapping := map[string]int{}
	failedBySource := map[string]int{}
	failedCount := 0
	for _, t := range tasks {
		m := defaultString(t.APICode, "未命名映射")
		s := defaultString(t.SourceSystem, "unknown")
		countByMapping[m]++
		countBySource[s]++
		if t.Status == repository.TaskStatusFailed || t.Status == repository.TaskStatusDeadLettered {
			failedByMapping[m]++
			failedBySource[s]++
			failedCount++
		}
	}
	return countByMapping, countBySource, failedByMapping, failedBySource, failedCount
}

func defaultString(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func (d *handlerDeps) handleDashboardOpsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	countByMapping, countBySource, failedByMapping, failedBySource, _ := d.aggregateAccessStats(r.Context())
	summary := dashboardOpsSummary{TopMappings: topN(countByMapping, 5), TopSourceIPs: topN(countBySource, 5), TopFailedMappings: topN(failedByMapping, 5), TopFailedSourceIPs: topN(failedBySource, 5), RateLimitStatus: "正常", CircuitBreakerState: "关闭", ProtectionStatus: "未触发保护"}
	if d.limits.RPS < 50 {
		summary.RateLimitStatus = "限流阈值较低"
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: summary})
}

func (d *handlerDeps) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	_, _, failedByMapping, _, failedCount := d.aggregateAccessStats(r.Context())
	status := d.networkStatusFunc(r.Context())
	summary := dashboardSummary{
		SystemHealth:        status.TunnelStatus(),
		ActiveConnections:   status.SIP.CurrentConnections,
		MappingTotal:        len(d.mappings.List()),
		MappingErrorCount:   len(failedByMapping),
		RecentFailureCount:  failedCount,
		RateLimitState:      "normal",
		CircuitBreakerState: "closed",
	}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: summary})
}

func (s NodeNetworkStatus) TunnelStatus() string {
	if len(s.RecentBindErrors) > 0 {
		return "degraded"
	}
	return "healthy"
}

func (d *handlerDeps) handleProtectionState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	state := protectionState{AlertRules: []string{"失败率 > 5% 持续 3 分钟"}, RateLimitRules: []string{"rps=" + strconv.Itoa(d.limits.RPS)}, CircuitBreakerRules: []string{"连续失败 10 次熔断"}, CurrentTriggered: []string{}, LastTriggeredTime: "", LastTriggeredTarget: ""}
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: state})
}

func (d *handlerDeps) handleSecurityState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	d.mu.RLock()
	state := securityCenterState{LicenseStatus: d.licenseInfo.Status, ExpiryTime: d.licenseInfo.ExpireAt, LicensedFeatures: d.licenseInfo.Features, LastValidation: d.licenseInfo.LastVerifyResult, ManagementSecurity: d.systemSettings.AdminAllowCIDR, SigningAlgorithm: d.securitySettings.Signer}
	d.mu.RUnlock()
	writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: state})
}

func (d *handlerDeps) handleNodeTunnelWorkspace(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		node := d.nodeStore.GetLocalNode()
		peers := d.nodeStore.ListPeers()
		peer := NodeConfigEndpoint{}
		if len(peers) > 0 {
			peer = NodeConfigEndpoint{NodeIP: peers[0].PeerSignalingIP, SignalingPort: peers[0].PeerSignalingPort, DeviceID: peers[0].PeerNodeID, RTPPortStart: peers[0].PeerMediaPortStart, RTPPortEnd: peers[0].PeerMediaPortEnd}
		}
		status := d.networkStatusFunc(r.Context())
		workspace := nodeTunnelWorkspace{LocalNode: NodeConfigEndpoint{NodeIP: node.SIPListenIP, SignalingPort: node.SIPListenPort, DeviceID: node.NodeID, RTPPortStart: node.RTPPortStart, RTPPortEnd: node.RTPPortEnd}, PeerNode: peer, NetworkMode: string(node.NetworkMode), CapabilityMatrix: []map[string]any{{"key": "supports_large_request_body", "supported": status.Capability.SupportsLargeRequestBody}, {"key": "supports_large_response_body", "supported": status.Capability.SupportsLargeResponseBody}}, SIPCapability: map[string]any{"transport": node.SIPTransport, "listen_ip": node.SIPListenIP, "listen_port": node.SIPListenPort}, RTPCapability: map[string]any{"transport": node.RTPTransport, "port_start": node.RTPPortStart, "port_end": node.RTPPortEnd}, SessionSettings: d.tunnelConfig, SecuritySettings: d.securitySettings, EncryptionSettings: map[string]string{"algorithm": d.securitySettings.Encryption}}
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: workspace})
	case http.MethodPost:
		var req nodeTunnelWorkspace
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid body")
			return
		}
		d.mu.Lock()
		d.tunnelConfig = req.SessionSettings
		d.securitySettings = req.SecuritySettings
		d.mu.Unlock()
		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: req})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func topN(m map[string]int, n int) []summaryItem {
	items := make([]summaryItem, 0, len(m))
	for k, v := range m {
		items = append(items, summaryItem{Name: k, Count: v})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Count > items[j].Count })
	if len(items) > n {
		items = items[:n]
	}
	return items
}
