package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"siptunnel/internal/observability"
	"siptunnel/internal/selfcheck"
	"siptunnel/internal/tunnelmapping"
)

func (d *handlerDeps) handleLinkTest(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Target string `json:"target"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		report := d.runLinkTest(r.Context(), strings.TrimSpace(req.Target))
		d.mu.Lock()
		d.lastLinkTest = report
		d.mu.Unlock()
		d.recordOpsAudit(r, readOperator(r), "RUN_LINK_TEST", map[string]any{"status": report.Status, "passed": report.Passed})
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
			LinkTestItem{Name: "management_auth", Passed: authLevel != selfcheck.LevelError, Status: string(authLevel), Detail: strings.TrimSpace(authMessage + " | " + authSuggestion + " | " + authHint)},
			LinkTestItem{Name: "config_encryption", Passed: runtimeSecurity.ConfigKeyEnabled, Status: map[bool]string{true: string(selfcheck.LevelInfo), false: string(selfcheck.LevelWarn)}[runtimeSecurity.ConfigKeyEnabled], Detail: strings.TrimSpace(map[bool]string{true: "配置落盘加密已启用。", false: "配置落盘加密未启用。"}[runtimeSecurity.ConfigKeyEnabled] + " | " + map[bool]string{true: "继续妥善保管 GATEWAY_CONFIG_KEY 并纳入轮换制度。", false: "建议配置 GATEWAY_CONFIG_KEY，保护隧道配置与授权文件。"}[runtimeSecurity.ConfigKeyEnabled] + " | 变更后重启并复核。")},
			LinkTestItem{Name: "tunnel_signer_secret", Passed: runtimeSecurity.SignerSecretManaged, Status: map[bool]string{true: string(selfcheck.LevelInfo), false: string(selfcheck.LevelWarn)}[runtimeSecurity.SignerSecretManaged], Detail: strings.TrimSpace(map[bool]string{true: "隧道签名密钥已外置。", false: "隧道签名密钥仍使用默认内置值。"}[runtimeSecurity.SignerSecretManaged] + " | " + map[bool]string{true: "继续按环境分离配置并纳入轮换。", false: "请配置 GATEWAY_TUNNEL_SIGNER_SECRET。"}[runtimeSecurity.SignerSecretManaged] + " | 生产环境必须使用环境独立密钥。")},
		)

		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: report})
	case http.MethodGet:
		d.mu.RLock()
		report := d.lastLinkTest
		d.mu.RUnlock()
		if report.CheckedAt.IsZero() {
			writeError(w, http.StatusNotFound, "LINK_TEST_NOT_FOUND", "no link test report yet")
			return
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
			LinkTestItem{Name: "management_auth", Passed: authLevel != selfcheck.LevelError, Status: string(authLevel), Detail: strings.TrimSpace(authMessage + " | " + authSuggestion + " | " + authHint)},
			LinkTestItem{Name: "config_encryption", Passed: runtimeSecurity.ConfigKeyEnabled, Status: map[bool]string{true: string(selfcheck.LevelInfo), false: string(selfcheck.LevelWarn)}[runtimeSecurity.ConfigKeyEnabled], Detail: strings.TrimSpace(map[bool]string{true: "配置落盘加密已启用。", false: "配置落盘加密未启用。"}[runtimeSecurity.ConfigKeyEnabled] + " | " + map[bool]string{true: "继续妥善保管 GATEWAY_CONFIG_KEY 并纳入轮换制度。", false: "建议配置 GATEWAY_CONFIG_KEY，保护隧道配置与授权文件。"}[runtimeSecurity.ConfigKeyEnabled] + " | 变更后重启并复核。")},
			LinkTestItem{Name: "tunnel_signer_secret", Passed: runtimeSecurity.SignerSecretManaged, Status: map[bool]string{true: string(selfcheck.LevelInfo), false: string(selfcheck.LevelWarn)}[runtimeSecurity.SignerSecretManaged], Detail: strings.TrimSpace(map[bool]string{true: "隧道签名密钥已外置。", false: "隧道签名密钥仍使用默认内置值。"}[runtimeSecurity.SignerSecretManaged] + " | " + map[bool]string{true: "继续按环境分离配置并纳入轮换。", false: "请配置 GATEWAY_TUNNEL_SIGNER_SECRET。"}[runtimeSecurity.SignerSecretManaged] + " | 生产环境必须使用环境独立密钥。")},
		)

		writeJSON(w, http.StatusOK, responseEnvelope{Code: "OK", Message: "success", Data: report})
	default:
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func (d *handlerDeps) runLinkTest(ctx context.Context, target string) LinkTestReport {
	started := time.Now()
	core := observability.CoreFieldsFromContext(ctx)
	status := d.networkStatusFunc(ctx)

	httpTarget := d.resolveLinkTestHTTPTarget()
	items := []LinkTestItem{d.checkSIPControlPath(ctx, status), d.checkRTPPortPool(status), d.checkHTTPConfiguredReachability(ctx, httpTarget)}
	if strings.EqualFold(strings.TrimSpace(target), "peer") {
		items = []LinkTestItem{mappingStageToLinkTestItem(d.checkPeerReachabilityStage(ctx))}
	}
	passed := true
	for _, item := range items {
		if !item.Passed {
			passed = false
			break
		}
	}

	report := LinkTestReport{
		Passed:     passed,
		Status:     map[bool]string{true: "passed", false: "failed"}[passed],
		RequestID:  core.RequestID,
		TraceID:    core.TraceID,
		DurationMS: time.Since(started).Milliseconds(),
		CheckedAt:  time.Now().UTC(),
		Items:      items,
		MockTarget: httpTarget,
	}
	return report
}

func mappingStageToLinkTestItem(stage MappingTestStage) LinkTestItem {
	return LinkTestItem{
		Name:   stage.Name,
		Passed: stage.Passed,
		Status: stage.Status,
		Detail: normalizeValue(stage.BlockingReason, stage.Detail),
	}
}

func (d *handlerDeps) checkSIPControlPath(ctx context.Context, status NodeNetworkStatus) LinkTestItem {
	start := time.Now()
	transport := strings.ToUpper(strings.TrimSpace(status.SIP.Transport))
	if transport == "" || status.SIP.ListenPort <= 0 {
		return LinkTestItem{Name: "sip_control", Passed: false, Status: "failed", Detail: "SIP 监听参数无效", DurationMS: time.Since(start).Milliseconds()}
	}
	if transport == "TCP" {
		host := strings.TrimSpace(status.SIP.ListenIP)
		if host == "" || host == "0.0.0.0" {
			host = "127.0.0.1"
		}
		addr := net.JoinHostPort(host, strconv.Itoa(status.SIP.ListenPort))
		dialer := &net.Dialer{Timeout: 600 * time.Millisecond}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return LinkTestItem{Name: "sip_control", Passed: false, Status: "failed", Detail: fmt.Sprintf("TCP 握手失败: %v", err), DurationMS: time.Since(start).Milliseconds()}
		}
		_ = conn.Close()
		return LinkTestItem{Name: "sip_control", Passed: true, Status: "passed", Detail: "SIP TCP 控制面握手成功（无业务载荷）", DurationMS: time.Since(start).Milliseconds()}
	}
	if len(status.RecentBindErrors) > 0 {
		return LinkTestItem{Name: "sip_control", Passed: false, Status: "failed", Detail: "发现 SIP 最近绑定错误，判定控制链路不可用", DurationMS: time.Since(start).Milliseconds()}
	}
	return LinkTestItem{Name: "sip_control", Passed: true, Status: "passed", Detail: "SIP UDP 采用监听状态与错误计数进行最小通路验证（无业务载荷）", DurationMS: time.Since(start).Milliseconds()}
}

func (d *handlerDeps) checkRTPPortPool(status NodeNetworkStatus) LinkTestItem {
	start := time.Now()
	available := status.RTP.AvailablePorts
	if available <= 0 || status.RTP.PortPoolTotal <= 0 {
		return LinkTestItem{Name: "rtp_port_pool", Passed: false, Status: "failed", Detail: "RTP 端口池不可用或已耗尽", DurationMS: time.Since(start).Milliseconds()}
	}
	return LinkTestItem{Name: "rtp_port_pool", Passed: true, Status: "passed", Detail: fmt.Sprintf("RTP 端口池可用: %d/%d", available, status.RTP.PortPoolTotal), DurationMS: time.Since(start).Milliseconds()}
}

func (d *handlerDeps) checkHTTPConfiguredReachability(ctx context.Context, target string) LinkTestItem {
	start := time.Now()
	if strings.TrimSpace(target) == "" {
		return LinkTestItem{Name: "http_downstream", Passed: false, Status: "failed", Detail: "未找到可用的 HTTP 目标配置，请先配置并启用本地资源/隧道映射", DurationMS: time.Since(start).Milliseconds()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return LinkTestItem{Name: "http_downstream", Passed: false, Status: "failed", Detail: fmt.Sprintf("构建 HTTP 配置目标探测请求失败: %v", err), DurationMS: time.Since(start).Milliseconds()}
	}
	req.Header.Set("X-Link-Test", "true")
	req.Header.Set("X-Api-Code", "ops.link_test")
	client := d.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return LinkTestItem{Name: "http_downstream", Passed: false, Status: "failed", Detail: fmt.Sprintf("HTTP 已配置目标不可达: %v", err), DurationMS: time.Since(start).Milliseconds()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return LinkTestItem{Name: "http_downstream", Passed: false, Status: "failed", Detail: fmt.Sprintf("HTTP 已配置目标返回状态异常: %d", resp.StatusCode), DurationMS: time.Since(start).Milliseconds()}
	}
	return LinkTestItem{Name: "http_downstream", Passed: true, Status: "passed", Detail: fmt.Sprintf("HTTP 已配置目标探测成功: %s", target), DurationMS: time.Since(start).Milliseconds()}
}

func (d *handlerDeps) resolveLinkTestHTTPTarget() string {
	target := strings.TrimSpace(os.Getenv("GATEWAY_LINK_TEST_HTTP_TARGET"))
	if target != "" {
		return target
	}
	if d == nil || d.mappings == nil {
		return ""
	}
	items := append([]tunnelmapping.TunnelMapping(nil), d.mappings.List()...)
	sort.SliceStable(items, func(i, j int) bool {
		return strings.TrimSpace(items[i].MappingID) < strings.TrimSpace(items[j].MappingID)
	})
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		host := strings.TrimSpace(item.RemoteTargetIP)
		port := item.RemoteTargetPort
		if host == "" || port <= 0 {
			continue
		}
		path := strings.TrimSpace(item.RemoteBasePath)
		if path == "" {
			path = "/"
		}
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		scheme := "http"
		if port == 443 {
			scheme = "https"
		}
		return fmt.Sprintf("%s://%s:%d%s", scheme, host, port, path)
	}
	return ""
}

func probeSignalingConnectivity(ctx context.Context, transport, addr string) (string, error) {
	transport = strings.ToUpper(strings.TrimSpace(transport))
	if transport == "" {
		transport = "TCP"
	}
	dialer := &net.Dialer{Timeout: 1200 * time.Millisecond}
	network := strings.ToLower(transport)
	conn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return fmt.Sprintf("%s 探测 %s 失败", transport, addr), err
	}
	defer conn.Close()
	if transport == "UDP" {
		return fmt.Sprintf("UDP 探测 %s 成功（最小路由/套接字验证）", addr), nil
	}
	return fmt.Sprintf("TCP 探测 %s 成功", addr), nil
}

func shouldLogInboundRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if isHealthProbePath(r.URL) {
		return false
	}
	switch r.URL.Path {
	case "/api/node-tunnel/workspace", "/api/dashboard/summary", "/api/dashboard/ops-summary", "/api/dashboard/trends", "/api/access-logs", "/api/audits", "/api/protection/state", "/api/system/settings", "/api/startup-summary", "/api/ops/link-test", "/api/loadtests", "/api/link-monitor", "/api/mappings", "/api/tunnel/mappings", "/api/resources/local", "/api/tunnel/catalog", "/api/tunnel/gb28181/state":
		return false
	}
	if r.Method != http.MethodGet {
		switch r.URL.Path {
		case "/api/tunnel/catalog/actions", "/api/tunnel/session/actions", "/api/resources/local", "/api/mapping/test", "/api/node-tunnel/workspace":
			return false
		default:
			return true
		}
	}
	return true
}
