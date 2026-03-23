package server

import "net/http"

// newMux 集中注册运维 API；接口清单见 gateway-server/docs/openapi-ops.yaml，
// 排障动作与升级路径见 docs/runbook.md、docs/oncall-handbook.md。
func newMux(deps handlerDeps) http.Handler {
	if deps.runtime == nil {
		deps.runtime = newMappingRuntimeManager(nil)
		deps.syncMappingRuntime()
	}
	dp := &deps
	mux := http.NewServeMux()

	registerCoreRoutes(mux, dp)
	registerMappingRoutes(mux, dp)
	registerTunnelRoutes(mux, dp)
	registerObservabilityRoutes(mux, dp)
	registerManagementRoutes(mux, dp)

	wrapped := dp.withObservability(dp.withManagementSecurity(mux))
	return wrapHTTPRecovery("gateway-api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r != nil && isHealthProbePath(r.URL) {
			mux.ServeHTTP(w, r)
			return
		}
		wrapped.ServeHTTP(w, r)
	}))
}

func registerCoreRoutes(mux *http.ServeMux, dp *handlerDeps) {
	mux.HandleFunc("/healthz", dp.healthz)
	mux.HandleFunc("/readyz", dp.readyz)
	mux.HandleFunc("/demo/process", dp.demoProcess)
	mux.HandleFunc("/api/tasks", dp.handleTasks)
	mux.HandleFunc("/api/tasks/", dp.handleTaskByID)
	mux.HandleFunc("/api/limits", dp.handleLimits)
	mux.HandleFunc("/api/routes", dp.handleRoutes)
	mux.HandleFunc("/api/nodes", dp.handleNodes)
	mux.HandleFunc("/api/selfcheck", dp.handleSelfCheck)
	mux.HandleFunc("/api/startup-summary", dp.handleStartupSummary)
	mux.HandleFunc("/api/ops/link-test", dp.handleLinkTest)
	mux.HandleFunc("/api/capacity/recommendation", dp.handleCapacityRecommendation)
	mux.HandleFunc("/api/loadtests", dp.handleLoadtests)
	mux.HandleFunc("/api/loadtests/", dp.handleLoadtests)
}

func registerMappingRoutes(mux *http.ServeMux, dp *handlerDeps) {
	mux.HandleFunc("/api/mappings", dp.handleMappings)
	mux.HandleFunc("/api/mappings/", dp.handleMappings)
	mux.HandleFunc("/api/mapping/test", dp.handleMappingTest)
	mux.HandleFunc("/api/resources/local", dp.handleLocalResources)
	mux.HandleFunc("/api/resources/local/", dp.handleLocalResources)
	mux.HandleFunc("/api/link-monitor", dp.handleLinkMonitor)
}

func registerTunnelRoutes(mux *http.ServeMux, dp *handlerDeps) {
	mux.HandleFunc("/api/system/status", dp.handleSystemStatus)
	mux.HandleFunc("/api/node/network-status", dp.handleNodeNetworkStatus)
	mux.HandleFunc("/api/node", dp.handleNode)
	mux.HandleFunc("/api/node/config", dp.handleNodeConfig)
	mux.HandleFunc("/api/peers", dp.handlePeers)
	mux.HandleFunc("/api/peers/", dp.handlePeers)
	mux.HandleFunc("/api/tunnel/config", dp.handleTunnelConfig)
	mux.HandleFunc("/api/tunnel/catalog", dp.handleTunnelCatalog)
	mux.HandleFunc("/api/tunnel/mappings", dp.handleTunnelMappingOverview)
	mux.HandleFunc("/api/tunnel/gb28181/state", dp.handleGB28181State)
	mux.HandleFunc("/api/tunnel/session/actions", dp.handleTunnelSessionActions)
	mux.HandleFunc("/api/tunnel/catalog/actions", dp.handleTunnelCatalogActions)
	mux.HandleFunc("/api/node-tunnel/workspace", dp.handleNodeTunnelWorkspace)
}

func registerObservabilityRoutes(mux *http.ServeMux, dp *handlerDeps) {
	mux.HandleFunc("/metrics", dp.handleMetrics)
	mux.HandleFunc("/audit/events", dp.listAuditEvents)
	mux.HandleFunc("/api/audits", dp.handleAudits)
	mux.HandleFunc("/api/access-logs", dp.handleAccessLogs)
	mux.HandleFunc("/api/dashboard/summary", dp.handleDashboardSummary)
	mux.HandleFunc("/api/dashboard/ops-summary", dp.handleDashboardOpsSummary)
	mux.HandleFunc("/api/dashboard/trends", dp.handleDashboardTrends)
	mux.HandleFunc("/api/diagnostics/export", dp.handleDiagnosticsExport)
}

func registerManagementRoutes(mux *http.ServeMux, dp *handlerDeps) {
	mux.HandleFunc("/api/security/settings", dp.handleSecuritySettings)
	mux.HandleFunc("/api/security/state", dp.handleSecurityState)
	mux.HandleFunc("/api/security/events", dp.handleSecurityEvents)
	mux.HandleFunc("/api/license", dp.handleLicense)
	mux.HandleFunc("/api/license/machine-code", dp.handleLicenseMachineCode)
	mux.HandleFunc("/api/system/settings", dp.handleSystemSettings)
	mux.HandleFunc("/api/system/resource-usage", dp.handleSystemResourceUsage)
	mux.HandleFunc("/api/gateway/restart", dp.handleGatewayRestart)
	mux.HandleFunc("/api/protection/state", dp.handleProtectionState)
	mux.HandleFunc("/api/protection/restrictions", dp.handleProtectionRestrictions)
	mux.HandleFunc("/api/protection/circuit/recover", dp.handleProtectionCircuitRecover)
}
