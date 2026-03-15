import { request } from './http'
import {
  fetchCommandTasksMock,
  fetchDashboardMock,
  fetchFileTasksMock,
  fetchTaskDetailMock,
  fetchNetworkConfigMock,
  updateNetworkConfigMock,
  fetchConfigGovernanceMock,
  rollbackConfigMock,
  exportConfigYamlMock,
  createDiagnosticExportMock,
  getDiagnosticExportMock,
  retryDiagnosticExportMock,
  fetchDeploymentModeMock,
  fetchStartupSummaryMock,
  fetchMappingsMock,
  createMappingMock,
  updateMappingMock,
  deleteMappingMock,
  fetchNodeDetailMock,
  updateLocalNodeMock,
  fetchPeersMock,
  createPeerMock,
  updatePeerMock,
  deletePeerMock,
  fetchNodeNetworkStatusMock,
  fetchSystemStatusMock,
  fetchNodeConfigMock,
  saveNodeConfigMock,
  fetchTunnelConfigMock,
  saveTunnelConfigMock,
  exportConfigJsonMock,
  importConfigJsonMock,
  downloadConfigTemplateMock,
  testMappingMock
} from './mockGateway'
import type {
  CommandTask,
  DashboardPayload,
  FileTask,
  OpsAuditEvent,
  OpsAuditFilters,
  OpsLimits,
  OpsNode,
  OpsRoute,
  TunnelMapping,
  TunnelMappingListPayload,
  TunnelMappingSavePayload,
  NetworkConfigPayload,
  UpdateNetworkConfigPayload,
  TaskDetail,
  TaskKind,
  TaskListFilters,
  TaskListResult,
  ConfigGovernancePayload,
  ConfigSnapshotFilters,
  DiagnosticExportCreatePayload,
  DiagnosticExportJob,
  DeploymentModePayload,
  StartupSummaryPayload,
  OpsLinkTestReport,
  NodeDetailPayload,
  LocalNodeConfig,
  PeerNodeConfig,
  NodeNetworkStatusPayload,
  SystemStatusPayload,
  NodeConfigPayload,
  TunnelConfigPayload,
  TunnelConfigUpdatePayload,
  TunnelSessionActionPayload,
  TunnelSessionActionResponse,
  ConfigTransferPayload,
  ConfigTransferImportResult,
  MappingTestPayload,
  AccessLogEntry,
  AccessLogFilters,
  SystemSettingsPayload,
  DashboardOpsSummaryPayload,
  DashboardSummary,
  DashboardOpsSummary,
  NodeTunnelWorkspace,
  MappingWorkspaceList,
  AccessLogQuery,
  AlertProtectionState,
  SystemSettingsState,
  SecurityCenterState
} from '../types/gateway'

const useMockMode = () => ((import.meta.env.VITE_API_MODE ?? 'real').toLowerCase() === 'mock')

const unwrap = async <T>(promise: Promise<{ data: T }>) => {
  const response = await promise
  return response.data
}

interface TaskApiModel {
  ID: string
  RequestID: string
  TraceID: string
  APICode: string
  SourceSystem: string
  Status: string
  CreatedAt: string
  UpdatedAt: string
}

const normalizeTaskStatus = (status: string) => status as CommandTask['status']


const mapDashboardSummary = (payload: {
  system_health: string
  active_connections: number
  mapping_total: number
  mapping_error_count: number
  recent_failure_count: number
  rate_limit_state: string
  circuit_breaker_state: string
}): DashboardSummary => ({
  systemHealth: payload.system_health,
  activeConnections: payload.active_connections,
  mappingTotal: payload.mapping_total,
  mappingErrorCount: payload.mapping_error_count,
  recentFailureCount: payload.recent_failure_count,
  rateLimitState: payload.rate_limit_state,
  circuitBreakerState: payload.circuit_breaker_state
})

const mapDashboardOpsSummary = (payload: DashboardOpsSummaryPayload): DashboardOpsSummary => ({
  hotMappings: payload.top_mappings,
  topFailureMappings: payload.top_failed_mappings,
  hotSourceIPs: payload.top_source_ips,
  topFailureIPs: payload.top_failed_source_ips
})

const mapTask = (item: TaskApiModel): CommandTask => ({
  id: item.ID,
  requestId: item.RequestID,
  traceId: item.TraceID,
  apiCode: item.APICode,
  nodeId: item.SourceSystem,
  status: normalizeTaskStatus(item.Status),
  createdAt: item.CreatedAt,
  updatedAt: item.UpdatedAt,
  latencyMs: 0
})

export const gatewayApi = {
  async fetchDashboard() {
    if (useMockMode()) {
      return fetchDashboardMock()
    }
    const [commandTasks, fileTasks] = await Promise.all([
      this.fetchCommandTasks({}, 1, 200),
      this.fetchFileTasks({}, 1, 200)
    ])
    const allTasks = [...commandTasks.list, ...fileTasks.list]
    const total = allTasks.length || 1
    const successCount = allTasks.filter((item) => item.status === 'succeeded').length
    const failedCount = allTasks.filter((item) => item.status === 'failed' || item.status === 'dead_lettered').length
    return {
      metrics: {
        successRate: (successCount / total) * 100,
        failureRate: (failedCount / total) * 100,
        concurrency: allTasks.filter((item) => item.status === 'running' || item.status === 'transferring').length,
        rtpLossRate: 0,
        rateLimitHits: 0,
        sipProtocol: 'UDP',
        sipListenPort: 5060,
        rtpProtocol: 'UDP',
        rtpPortRange: '20000-20999',
        activeSessions: allTasks.filter((item) => item.status !== 'cancelled').length,
        activeTransfers: allTasks.filter((item) => item.status === 'transferring').length,
        currentConnections: allTasks.filter((item) => item.status !== 'cancelled').length,
        failedTasks1h: failedCount,
        transportErrors1h: 0,
        rateLimitHits1h: 0
      },
      recentTrends: []
    } as DashboardPayload
  },
  async fetchCommandTasks(filters: TaskListFilters, page: number, pageSize: number) {
    if (useMockMode()) {
      return fetchCommandTasksMock(filters, page, pageSize)
    }
    const result = await unwrap(
      request<{ items: TaskApiModel[]; pagination: { total: number; page: number; page_size: number } }>('/tasks', {
        method: 'GET',
        params: {
          task_type: 'command',
          status: filters.status,
          request_id: filters.requestId,
          trace_id: filters.traceId,
          page,
          page_size: pageSize
        }
      })
    )
    return {
      list: result.items.map(mapTask),
      total: result.pagination.total,
      page: result.pagination.page,
      pageSize: result.pagination.page_size
    } as TaskListResult<CommandTask>
  },
  async fetchFileTasks(filters: TaskListFilters, page: number, pageSize: number) {
    if (useMockMode()) {
      return fetchFileTasksMock(filters, page, pageSize)
    }
    const result = await unwrap(
      request<{ items: TaskApiModel[]; pagination: { total: number; page: number; page_size: number } }>('/tasks', {
        method: 'GET',
        params: {
          task_type: 'file',
          status: filters.status,
          request_id: filters.requestId,
          trace_id: filters.traceId,
          page,
          page_size: pageSize
        }
      })
    )
    return {
      list: result.items.map((item) => ({
        id: item.ID,
        requestId: item.RequestID,
        traceId: item.TraceID,
        filename: item.APICode,
        status: normalizeTaskStatus(item.Status),
        totalShards: 0,
        missingShards: 0,
        retryRounds: 0,
        checksumPassed: true,
        progress: item.Status === 'succeeded' ? 100 : 0,
        createdAt: item.CreatedAt,
        updatedAt: item.UpdatedAt
      })),
      total: result.pagination.total,
      page: result.pagination.page,
      pageSize: result.pagination.page_size
    } as TaskListResult<FileTask>
  },
  async fetchTaskDetail(id: string, taskKind: TaskKind) {
    if (useMockMode()) {
      return fetchTaskDetailMock(id, taskKind)
    }
    const task = await unwrap<TaskApiModel>(request(`/tasks/${id}`, { method: 'GET' }))
    return {
      id: task.ID,
      taskKind,
      requestId: task.RequestID,
      traceId: task.TraceID,
      status: normalizeTaskStatus(task.Status),
      nodeId: task.SourceSystem,
      createdAt: task.CreatedAt,
      updatedAt: task.UpdatedAt,
      timeline: [],
      sipEvents: [],
      rtpStats: { totalShards: 0, receivedShards: 0, missingShards: 0, retransmittedShards: 0, bitrateMbps: 0 },
      httpResult: { apiCode: task.APICode, url: '-', method: '-', statusCode: 0, durationMs: 0, responseSnippet: '-' },
      auditSnippets: []
    } as TaskDetail
  },

  async fetchNetworkConfig() {
    if (useMockMode()) {
      return fetchNetworkConfigMock()
    }
    return unwrap(request<NetworkConfigPayload>('/network/config', { method: 'GET' }))
  },
  async updateNetworkConfig(payload: UpdateNetworkConfigPayload) {
    if (useMockMode()) {
      return updateNetworkConfigMock(payload)
    }
    return unwrap(request<NetworkConfigPayload>('/network/config', { method: 'PUT', body: payload }))
  },


  async fetchConfigGovernance(filters: ConfigSnapshotFilters) {
    if (useMockMode()) {
      return fetchConfigGovernanceMock(filters)
    }
    return unwrap(
      request<ConfigGovernancePayload>('/config-governance', {
        method: 'GET',
        params: {
          startTime: filters.startTime,
          endTime: filters.endTime,
          operator: filters.operator,
          version: filters.version
        }
      })
    )
  },

  async rollbackConfig(version: string) {
    if (useMockMode()) {
      return rollbackConfigMock(version)
    }
    return unwrap(request<ConfigGovernancePayload>('/config-governance/rollback', { method: 'POST', body: { version } }))
  },


  async createDiagnosticExport(payload: DiagnosticExportCreatePayload) {
    if (useMockMode()) {
      return createDiagnosticExportMock(payload)
    }
    return unwrap(request<DiagnosticExportJob>('/diagnostics/exports', { method: 'POST', body: payload }))
  },

  async fetchDiagnosticExport(jobId: string) {
    if (useMockMode()) {
      return getDiagnosticExportMock(jobId)
    }
    return unwrap(request<DiagnosticExportJob>(`/diagnostics/exports/${jobId}`, { method: 'GET' }))
  },

  async retryDiagnosticExport(jobId: string) {
    if (useMockMode()) {
      return retryDiagnosticExportMock(jobId)
    }
    return unwrap(request<DiagnosticExportJob>(`/diagnostics/exports/${jobId}/retry`, { method: 'POST' }))
  },

  async exportConfigYaml(target: 'current' | 'pending') {
    if (useMockMode()) {
      return exportConfigYamlMock(target)
    }
    const result = await unwrap<{ content: string }>(
      request('/config-governance/export', { method: 'GET', params: { target } })
    )
    return result.content
  },

  async fetchDeploymentMode() {
    if (useMockMode()) {
      return fetchDeploymentModeMock()
    }
    return unwrap(request<DeploymentModePayload>('/system/deployment-mode', { method: 'GET' }))
  },


  async fetchStartupSummary() {
    if (useMockMode()) {
      return fetchStartupSummaryMock()
    }
    return unwrap(request<StartupSummaryPayload>('/startup-summary', { method: 'GET' }))
  },

  async fetchSystemStatus() {
    if (useMockMode()) {
      return fetchSystemStatusMock()
    }
    return unwrap(request<SystemStatusPayload>('/system/status', { method: 'GET' }))
  },


  async fetchTunnelConfig() {
    if (useMockMode()) {
      return fetchTunnelConfigMock()
    }
    return unwrap(request<TunnelConfigPayload>('/tunnel/config', { method: 'GET' }))
  },

  async saveTunnelConfig(payload: TunnelConfigUpdatePayload) {
    if (useMockMode()) {
      return saveTunnelConfigMock(payload)
    }
    return unwrap(request<TunnelConfigPayload>('/tunnel/config', { method: 'POST', body: payload }))
  },

  async triggerTunnelSessionAction(payload: TunnelSessionActionPayload) {
    return unwrap(request<TunnelSessionActionResponse>('/tunnel/session/actions', { method: 'POST', body: payload }))
  },


  async fetchNodeConfig() {
    if (useMockMode()) {
      return fetchNodeConfigMock()
    }
    return unwrap(request<NodeConfigPayload>('/node/config', { method: 'GET' }))
  },

  async saveNodeConfig(payload: NodeConfigPayload) {
    if (useMockMode()) {
      return saveNodeConfigMock(payload)
    }
    return unwrap(request<{ config: NodeConfigPayload; tunnel_restarted: boolean }>('/node/config', { method: 'POST', body: payload }))
  },


  async exportConfigJson() {
    if (useMockMode()) {
      return exportConfigJsonMock()
    }
    return unwrap(request<ConfigTransferPayload>('/config/transfer/export', { method: 'GET' }))
  },

  async importConfigJson(payload: ConfigTransferPayload) {
    if (useMockMode()) {
      return importConfigJsonMock(payload)
    }
    return unwrap(request<ConfigTransferImportResult>('/config/transfer/import', { method: 'POST', body: payload }))
  },

  async downloadConfigTemplate() {
    if (useMockMode()) {
      return downloadConfigTemplateMock()
    }
    return unwrap(request<ConfigTransferPayload>('/config/transfer/template', { method: 'GET' }))
  },
  async fetchNodeDetail() {
    if (useMockMode()) {
      return fetchNodeDetailMock()
    }
    return unwrap(request<NodeDetailPayload>('/node', { method: 'GET' }))
  },

  async updateLocalNode(payload: LocalNodeConfig) {
    if (useMockMode()) {
      return updateLocalNodeMock(payload)
    }
    return unwrap(request<LocalNodeConfig>('/node', { method: 'PUT', body: payload }))
  },

  async fetchPeers() {
    if (useMockMode()) {
      return fetchPeersMock()
    }
    return unwrap<{ items: PeerNodeConfig[] }>(request('/peers', { method: 'GET' }))
  },

  async createPeer(payload: PeerNodeConfig) {
    if (useMockMode()) {
      return createPeerMock(payload)
    }
    return unwrap(request<PeerNodeConfig>('/peers', { method: 'POST', body: payload }))
  },

  async updatePeer(peerNodeId: string, payload: Omit<PeerNodeConfig, 'peer_node_id'>) {
    if (useMockMode()) {
      return updatePeerMock(peerNodeId, payload)
    }
    return unwrap(request<PeerNodeConfig>(`/peers/${peerNodeId}`, { method: 'PUT', body: payload }))
  },

  async deletePeer(peerNodeId: string) {
    if (useMockMode()) {
      return deletePeerMock(peerNodeId)
    }
    return unwrap<{ peer_node_id: string }>(request(`/peers/${peerNodeId}`, { method: 'DELETE' }))
  },

  async fetchNodeNetworkStatus() {
    if (useMockMode()) {
      return fetchNodeNetworkStatusMock()
    }
    return unwrap<NodeNetworkStatusPayload>(request('/node/network-status', { method: 'GET' }))
  },

  fetchLimits() {
    return unwrap(request<OpsLimits>('/limits', { method: 'GET' }))
  },
  updateLimits(payload: OpsLimits) {
    return unwrap(request<OpsLimits>('/limits', { method: 'PUT', body: payload }))
  },
  async fetchRoutes() {
    const result = await unwrap<{ items: OpsRoute[] }>(request('/routes', { method: 'GET' }))
    return result.items
  },
  async updateRoutes(routes: OpsRoute[]) {
    const result = await unwrap<{ items: OpsRoute[] }>(request('/routes', { method: 'PUT', body: { routes } }))
    return result.items
  },
  async fetchMappings() {
    if (useMockMode()) {
      return fetchMappingsMock()
    }
    return unwrap<TunnelMappingListPayload>(request('/mappings', { method: 'GET' }))
  },
  async createMapping(payload: TunnelMapping) {
    if (useMockMode()) {
      return createMappingMock(payload)
    }
    return unwrap<TunnelMappingSavePayload>(request('/mappings', { method: 'POST', body: payload }))
  },
  async updateMapping(id: string, payload: TunnelMapping) {
    if (useMockMode()) {
      return updateMappingMock(id, payload)
    }
    return unwrap<TunnelMappingSavePayload>(request(`/mappings/${id}`, { method: 'PUT', body: payload }))
  },
  async deleteMapping(id: string) {
    if (useMockMode()) {
      return deleteMappingMock(id)
    }
    await unwrap<Record<string, never>>(request(`/mappings/${id}`, { method: 'DELETE' }))
  },
  async testMapping() {
    if (useMockMode()) {
      return testMappingMock()
    }
    return unwrap<MappingTestPayload>(request('/mapping/test', { method: 'POST' }))
  },
  async fetchNodes() {
    const result = await unwrap<{ items: OpsNode[] }>(request('/nodes', { method: 'GET' }))
    return result.items
  },
  async fetchAudits(page: number, pageSize: number, query?: OpsAuditFilters) {
    const result = await unwrap<{ items: OpsAuditEvent[]; pagination: { total: number; page: number; page_size: number } }>(
      request('/audits', {
        method: 'GET',
        params: {
          page,
          page_size: pageSize,
          request_id: query?.requestId,
          trace_id: query?.traceId,
          rule: query?.rule,
          error_only: query?.errorOnly,
          start_time: query?.startTime,
          end_time: query?.endTime
        }
      })
    )
    return {
      list: result.items,
      total: result.pagination.total,
      page: result.pagination.page,
      pageSize: result.pagination.page_size
    }
  },
  async runLinkTest() {
    if (useMockMode()) {
      return {
        passed: true,
        status: 'passed',
        request_id: 'req-link-mock-001',
        trace_id: 'trace-link-mock-001',
        duration_ms: 42,
        checked_at: new Date().toISOString(),
        mock_target: 'http://127.0.0.1:18080/healthz',
        items: [
          { name: 'sip_control', passed: true, status: 'passed', detail: 'SIP TCP 控制面握手成功（无业务载荷）', duration_ms: 11 },
          { name: 'rtp_port_pool', passed: true, status: 'passed', detail: 'RTP 端口池可用: 874/1000', duration_ms: 8 },
          { name: 'http_downstream', passed: true, status: 'passed', detail: 'HTTP mock/downstream 探测成功', duration_ms: 23 }
        ]
      } as OpsLinkTestReport
    }
    return unwrap(request<OpsLinkTestReport>('/ops/link-test', { method: 'POST' }))
  },
  async fetchLatestLinkTest() {
    return unwrap(request<OpsLinkTestReport>('/ops/link-test', { method: 'GET' }))
  },

  async fetchAccessLogs(filters: AccessLogQuery, page: number, pageSize: number) {
    const result = await unwrap<{ items: AccessLogEntry[]; pagination: { total: number; page: number; page_size: number } }>(
      request('/access-logs', {
        method: 'GET',
        params: {
          status: filters.status,
          mapping: filters.mapping,
          source_ip: filters.sourceIP,
          method: filters.method,
          slow_only: filters.slowOnly,
          page,
          page_size: pageSize
        }
      })
    )
    return {
      list: result.items,
      total: result.pagination.total,
      page: result.pagination.page,
      pageSize: result.pagination.page_size
    }
  },

  async fetchDashboardSummary() {
    const payload = await unwrap<{
      system_health: string
      active_connections: number
      mapping_total: number
      mapping_error_count: number
      recent_failure_count: number
      rate_limit_state: string
      circuit_breaker_state: string
    }>(request('/dashboard/summary', { method: 'GET' }))
    return mapDashboardSummary(payload)
  },

  async fetchDashboardOpsSummary() {
    const payload = await unwrap<DashboardOpsSummaryPayload>(request('/dashboard/ops-summary', { method: 'GET' }))
    return mapDashboardOpsSummary(payload)
  },

  async fetchNodeTunnelWorkspace() {
    return unwrap<NodeTunnelWorkspace>(request('/node-tunnel/workspace', { method: 'GET' }))
  },

  async saveNodeTunnelWorkspace(payload: NodeTunnelWorkspace) {
    return unwrap<NodeTunnelWorkspace>(request('/node-tunnel/workspace', { method: 'POST', body: payload }))
  },

  async fetchMappingWorkspaceList() {
    const payload = await unwrap<TunnelMappingListPayload>(request('/mappings', { method: 'GET' }))
    return {
      items: payload.items.map((item) => ({
        mappingId: item.mapping_id,
        mappingName: item.name || item.mapping_id,
        localEntry: `${item.local_bind_ip}:${item.local_bind_port}${item.local_base_path}`,
        peerTarget: `${item.remote_target_ip}:${item.remote_target_port}${item.remote_base_path}`,
        status: item.enabled ? 'enabled' : 'disabled',
        lastTestResult: item.failure_reason ? 'failed' : 'passed',
        requestCount: 0,
        failureCount: item.failure_reason ? 1 : 0,
        avgLatency: 0,
        riskLevel: item.failure_reason ? 'high' : 'normal'
      }))
    } as MappingWorkspaceList
  },

  async fetchProtectionState() {
    return unwrap<AlertProtectionState>(request('/protection/state', { method: 'GET' }))
  },

  async fetchSystemSettings() {
    const payload = await unwrap<SystemSettingsPayload>(request('/system/settings', { method: 'GET' }))
    return {
      sqlitePath: payload.sqlite_path,
      logPath: '/var/log/siptunnel',
      logRetentionDays: payload.max_task_age_days,
      auditRetentionDays: payload.max_audit_age_days,
      accessLogRetentionDays: payload.max_access_log_age_days,
      diagnosticsRetentionDays: payload.max_diagnostic_age_days,
      loadtestRetentionDays: payload.max_loadtest_age_days,
      cleanupCron: payload.log_cleanup_cron,
      adminCIDR: payload.admin_allow_cidr,
      mfaEnabled: payload.admin_require_mfa,
      lastCleanupStatus: payload.cleaner_last_result
    } as SystemSettingsState
  },

  async updateSystemSettings(payload: SystemSettingsState) {
    const req: SystemSettingsPayload = {
      sqlite_path: payload.sqlitePath,
      log_cleanup_cron: payload.cleanupCron,
      max_task_age_days: payload.logRetentionDays,
      max_task_records: 20000,
      max_access_log_age_days: payload.accessLogRetentionDays,
      max_access_log_records: 20000,
      max_audit_age_days: payload.auditRetentionDays,
      max_audit_records: 50000,
      max_diagnostic_age_days: payload.diagnosticsRetentionDays,
      max_diagnostic_records: 2000,
      max_loadtest_age_days: payload.loadtestRetentionDays,
      max_loadtest_records: 2000,
      admin_allow_cidr: payload.adminCIDR,
      admin_require_mfa: payload.mfaEnabled,
      cleaner_last_run_at: '',
      cleaner_last_result: payload.lastCleanupStatus,
      cleaner_last_removed_records: 0
    }
    await unwrap(request<SystemSettingsPayload>('/system/settings', { method: 'POST', body: req }))
    return this.fetchSystemSettings()
  },

  async fetchSecurityState() {
    return unwrap<SecurityCenterState>(request('/security/state', { method: 'GET' }))
  },
  async fetchSecuritySettings() {
    return unwrap(request<{ signer: string; encryption: string; verify_interval_min: number }>('/security/settings', { method: 'GET' }))
  },
  async updateSecuritySettings(payload: { signer: string; encryption: string; verify_interval_min: number }) {
    return unwrap(request<{ signer: string; encryption: string; verify_interval_min: number }>('/security/settings', { method: 'PUT', body: payload }))
  },
  async fetchLicense() {
    return unwrap(request<{ status: string; expire_at: string; features: string[]; last_verify_result: string }>('/license', { method: 'GET' }))
  },
  async updateLicense(payload: { token: string }) {
    return unwrap(request<{ status: string; expire_at: string; features: string[]; last_verify_result: string }>('/license', { method: 'PUT', body: payload }))
  },

}
