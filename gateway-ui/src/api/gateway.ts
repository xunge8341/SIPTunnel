import { request } from './http'
import { callMockGateway } from './mockGatewayLoader'
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
  TunnelCatalogActionPayload,
  TunnelCatalogActionResponse,
  LocalResourceListPayload,
  LocalResourceSavePayload,
  TunnelCatalogPayload,
  TunnelMappingOverviewPayload,
  GB28181StatePayload,
  LinkMonitorPayload,
  ConfigTransferPayload,
  ConfigTransferImportResult,
  MappingTestPayload,
  AccessLogEntry,
  AccessLogFilters,
  SystemSettingsPayload,
  DashboardOpsSummaryPayload,
  DashboardSummary,
  DashboardOpsSummary,
  DashboardTrendSeries,
  NodeTunnelWorkspace,
  MappingWorkspaceList,
  AccessLogQuery,
  AccessLogSummary,
  AlertProtectionState,
  ProtectionCircuitRecoverResponse,
  ProtectionRestrictionActionResponse,
  SystemSettingsState,
  SystemResourceUsage,
  SecurityEventRecord,
  SecurityCenterState,
  SecurityStatePayload,
  MachineCodePayload,
  LicensePayload,
  LoadtestJob,
  DiagnosticExportData,
  GatewayRestartResponse
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

const mapProtectionState = (payload: any): AlertProtectionState => ({
  alertRules: payload?.alert_rules ?? [],
  rateLimitRules: payload?.rate_limit_rules ?? [],
  circuitBreakerRules: payload?.circuit_breaker_rules ?? [],
  currentTriggered: payload?.current_triggered ?? [],
  lastTriggeredTime: payload?.last_triggered_time ?? '',
  lastTriggeredTarget: payload?.last_triggered_target ?? '',
  rps: payload?.rps,
  burst: payload?.burst,
  maxConcurrent: payload?.max_concurrent,
  failureThreshold: payload?.failure_threshold,
  recoveryWindowSec: payload?.recovery_window_sec,
  rateLimitStatus: payload?.rate_limit_status,
  circuitBreakerStatus: payload?.circuit_breaker_status,
  protectionStatus: payload?.protection_status,
  analysisWindow: payload?.analysis_window,
  recentFailureCount: payload?.recent_failure_count,
  recentSlowRequestCount: payload?.recent_slow_request_count,
  currentActiveRequests: payload?.current_active_requests,
  rateLimitHitsTotal: payload?.rate_limit_hits_total,
  concurrentRejectsTotal: payload?.concurrent_rejects_total,
  allowedRequestsTotal: payload?.allowed_requests_total,
  lastTriggeredType: payload?.last_triggered_type,
  circuitOpenCount: payload?.circuit_open_count,
  circuitHalfOpenCount: payload?.circuit_half_open_count,
  circuitActiveState: payload?.circuit_active_state,
  circuitLastOpenUntil: payload?.circuit_last_open_until,
  circuitLastOpenReason: payload?.circuit_last_open_reason,
  circuitEntries: payload?.circuit_entries ?? [],
  topRateLimitTargets: payload?.top_rate_limit_targets ?? [],
  topConcurrentTargets: payload?.top_concurrent_targets ?? [],
  topAllowedTargets: payload?.top_allowed_targets ?? [],
  scopes: payload?.scopes ?? [],
  restrictions: payload?.restrictions ?? []
})


const mapNodeEndpoint = (payload?: Partial<{ node_ip: string; signaling_port: number; device_id: string; node_type?: string; rtp_port_start?: number; rtp_port_end?: number; mapping_port_start?: number; mapping_port_end?: number }>) => ({
  node_ip: payload?.node_ip ?? '',
  signaling_port: payload?.signaling_port ?? 0,
  device_id: payload?.device_id ?? '',
  node_type: payload?.node_type ?? 'SERVER',
  rtp_port_start: payload?.rtp_port_start ?? undefined,
  rtp_port_end: payload?.rtp_port_end ?? undefined,
  mapping_port_start: payload?.mapping_port_start ?? undefined,
  mapping_port_end: payload?.mapping_port_end ?? undefined
})

const emptySessionSettings = (): TunnelConfigPayload => ({
  channel_protocol: 'SIP',
  connection_initiator: 'LOCAL',
  mapping_relay_mode: 'AUTO',
  local_device_id: '',
  peer_device_id: '',
  heartbeat_interval_sec: 30,
  register_retry_count: 3,
  register_retry_interval_sec: 5,
  registration_status: '',
  last_register_time: '',
  last_heartbeat_time: '',
  heartbeat_status: '',
  last_failure_reason: '',
  next_retry_time: '',
  consecutive_heartbeat_timeout: 0,
  supported_capabilities: [],
  request_channel: 'SIP',
  response_channel: 'RTP',
  network_mode: 'SENDER_SIP__RECEIVER_RTP',
  capability: {
    supports_small_request_body: true,
    supports_large_request_body: false,
    supports_large_response_body: true,
    supports_streaming_response: true,
    supports_bidirectional_http_tunnel: false,
    supports_transparent_http_proxy: false
  },
  capability_items: [],
  register_auth_enabled: false,
  register_auth_username: '',
  register_auth_password: '',
  register_auth_password_configured: false,
  register_auth_realm: '',
  register_auth_algorithm: 'MD5',
  catalog_subscribe_expires_sec: 3600
})

const mapNodeTunnelWorkspace = (payload: any): NodeTunnelWorkspace => ({
  localNode: mapNodeEndpoint(payload?.local_node),
  peerNode: mapNodeEndpoint(payload?.peer_node),
  networkMode: payload?.network_mode ?? 'SENDER_SIP__RECEIVER_RTP',
  capabilityMatrix: Array.isArray(payload?.capability_matrix) ? payload.capability_matrix : [],
  sipCapability: { ...(payload?.sip_capability ?? {}), transport: String(payload?.sip_capability?.transport ?? 'TCP').toUpperCase() },
  rtpCapability: { ...(payload?.rtp_capability ?? {}), transport: String(payload?.rtp_capability?.transport ?? 'UDP').toUpperCase() },
  sessionSettings: { ...emptySessionSettings(), ...(payload?.session_settings ?? {}), mapping_relay_mode: payload?.session_settings?.mapping_relay_mode ?? 'AUTO' },
  securitySettings: {
    signer: payload?.security_settings?.signer ?? 'HMAC-SHA256',
    encryption: payload?.security_settings?.encryption ?? 'AES',
    verify_interval_min: payload?.security_settings?.verify_interval_min ?? 30,
    admin_allow_cidr: payload?.security_settings?.admin_allow_cidr ?? '',
    admin_require_mfa: Boolean(payload?.security_settings?.admin_require_mfa)
  },
  encryptionSettings: payload?.encryption_settings ?? { algorithm: payload?.security_settings?.encryption ?? 'AES' }
})

const toNodeTunnelWorkspacePayload = (payload: NodeTunnelWorkspace) => ({
  local_node: payload.localNode,
  peer_node: payload.peerNode,
  network_mode: payload.networkMode,
  capability_matrix: payload.capabilityMatrix,
  sip_capability: payload.sipCapability,
  rtp_capability: payload.rtpCapability,
  session_settings: payload.sessionSettings,
  security_settings: payload.securitySettings,
  encryption_settings: payload.encryptionSettings
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
      return callMockGateway('fetchDashboardMock')
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
      return callMockGateway('fetchCommandTasksMock', filters, page, pageSize)
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
      return callMockGateway('fetchFileTasksMock', filters, page, pageSize)
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
      return callMockGateway('fetchTaskDetailMock', id, taskKind)
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


  async fetchStartupSummary() {
    if (useMockMode()) {
      return callMockGateway('fetchStartupSummaryMock')
    }
    return unwrap(request<StartupSummaryPayload>('/startup-summary', { method: 'GET' }))
  },

  async fetchSystemStatus() {
    if (useMockMode()) {
      return callMockGateway('fetchSystemStatusMock')
    }
    return unwrap(request<SystemStatusPayload>('/system/status', { method: 'GET' }))
  },


  async fetchTunnelConfig() {
    if (useMockMode()) {
      return callMockGateway('fetchTunnelConfigMock')
    }
    return unwrap(request<TunnelConfigPayload>('/tunnel/config', { method: 'GET' }))
  },

  async saveTunnelConfig(payload: TunnelConfigUpdatePayload) {
    if (useMockMode()) {
      return callMockGateway('saveTunnelConfigMock', payload)
    }
    return unwrap(request<TunnelConfigPayload>('/tunnel/config', { method: 'POST', body: payload }))
  },

  async fetchLocalResources() {
    if (useMockMode()) {
      return callMockGateway('fetchLocalResourcesMock')
    }
    return unwrap<LocalResourceListPayload>(request('/resources/local', { method: 'GET' }))
  },


  async createLocalResource(payload: LocalResourceSavePayload) {
    if (useMockMode()) {
      return callMockGateway('createLocalResourceMock', payload)
    }
    return unwrap(request('/resources/local', { method: 'POST', body: payload }))
  },

  async updateLocalResource(resourceCode: string, payload: LocalResourceSavePayload) {
    if (useMockMode()) {
      return callMockGateway('updateLocalResourceMock', resourceCode, payload)
    }
    return unwrap(request(`/resources/local/${encodeURIComponent(resourceCode)}`, { method: 'PUT', body: payload }))
  },

  async deleteLocalResource(resourceCode: string) {
    if (useMockMode()) {
      return callMockGateway('deleteLocalResourceMock', resourceCode)
    }
    return unwrap(request(`/resources/local/${encodeURIComponent(resourceCode)}`, { method: 'DELETE' }))
  },

  async fetchTunnelCatalog() {
    if (useMockMode()) {
      return callMockGateway('fetchTunnelCatalogMock')
    }
    const payload = await unwrap<any>(request('/tunnel/catalog', { method: 'GET' }))
    return {
      resources: Array.isArray(payload?.resources)
        ? payload.resources.map((item: any) => ({
            resource_code: item?.resource_code ?? item?.device_id ?? '',
            device_id: item?.device_id ?? item?.resource_code ?? '',
            resource_type: item?.resource_type ?? 'SERVICE',
            name: item?.name ?? item?.device_id ?? item?.resource_code ?? '',
            local_port: Array.isArray(item?.local_ports) && item.local_ports.length > 0 ? Number(item.local_ports[0]) : (item?.local_port ?? undefined),
            local_ports: Array.isArray(item?.local_ports) ? item.local_ports.map((port: unknown) => Number(port)).filter((port: number) => Number.isFinite(port)) : [],
            exposure_mode: item?.exposure_mode ?? 'UNEXPOSED',
            methods: Array.isArray(item?.method_list) ? item.method_list : (Array.isArray(item?.methods) ? item.methods : []),
            method_list: Array.isArray(item?.method_list) ? item.method_list : undefined,
            response_mode: item?.response_mode ?? 'RTP',
            source: item?.source ?? 'REMOTE',
            status: item?.status ?? 'UNKNOWN',
            max_inline_response_body: item?.max_inline_response_body ?? undefined,
            max_request_body: item?.max_request_body ?? undefined,
            mapping_ids: Array.isArray(item?.mapping_ids) ? item.mapping_ids : []
          }))
        : [],
      summary: {
        resource_total: payload?.summary?.resource_total ?? 0,
        manual_expose_num: payload?.summary?.manual_expose_num ?? 0,
        unexposed_num: payload?.summary?.unexposed_num ?? 0
      }
    } as TunnelCatalogPayload
  },

  async fetchTunnelMappingOverview() {
    if (useMockMode()) {
      return callMockGateway('fetchTunnelMappingOverviewMock')
    }
    return unwrap<TunnelMappingOverviewPayload>(request('/tunnel/mappings', { method: 'GET' }))
  },

  async fetchGB28181State() {
    if (useMockMode()) {
      return callMockGateway('fetchGB28181StateMock')
    }
    const payload = await unwrap(request<GB28181StatePayload>('/tunnel/gb28181/state', { method: 'GET' }))
    return {
      ...payload,
      gb28181: payload.gb28181
        ? {
            ...payload.gb28181,
            peers: Array.isArray(payload.gb28181.peers) ? payload.gb28181.peers : [],
            pending_sessions: Array.isArray(payload.gb28181.pending_sessions) ? payload.gb28181.pending_sessions : [],
            inbound_sessions: Array.isArray(payload.gb28181.inbound_sessions) ? payload.gb28181.inbound_sessions : []
          }
        : undefined
    }
  },

  async fetchLinkMonitor() {
    if (useMockMode()) {
      return callMockGateway('fetchLinkMonitorMock')
    }
    return unwrap<LinkMonitorPayload>(request('/link-monitor', { method: 'GET' }))
  },

  async triggerTunnelSessionAction(payload: TunnelSessionActionPayload) {
    return unwrap(request<TunnelSessionActionResponse>('/tunnel/session/actions', { method: 'POST', body: payload }))
  },

  async triggerTunnelCatalogAction(payload: TunnelCatalogActionPayload) {
    if (useMockMode()) {
      return callMockGateway('triggerTunnelCatalogActionMock', payload)
    }
    return unwrap(request<TunnelCatalogActionResponse>('/tunnel/catalog/actions', { method: 'POST', body: payload }))
  },


  async fetchNodeConfig() {
    if (useMockMode()) {
      return callMockGateway('fetchNodeConfigMock')
    }
    return unwrap(request<NodeConfigPayload>('/node/config', { method: 'GET' }))
  },

  async saveNodeConfig(payload: NodeConfigPayload) {
    if (useMockMode()) {
      return callMockGateway('saveNodeConfigMock', payload)
    }
    return unwrap(request<{ config: NodeConfigPayload; tunnel_restarted: boolean }>('/node/config', { method: 'POST', body: payload }))
  },


  async fetchNodeDetail() {
    if (useMockMode()) {
      return callMockGateway('fetchNodeDetailMock')
    }
    return unwrap(request<NodeDetailPayload>('/node', { method: 'GET' }))
  },

  async updateLocalNode(payload: LocalNodeConfig) {
    if (useMockMode()) {
      return callMockGateway('updateLocalNodeMock', payload)
    }
    return unwrap(request<LocalNodeConfig>('/node', { method: 'PUT', body: payload }))
  },

  async fetchPeers() {
    if (useMockMode()) {
      return callMockGateway('fetchPeersMock')
    }
    return unwrap<{ items: PeerNodeConfig[] }>(request('/peers', { method: 'GET' }))
  },

  async createPeer(payload: PeerNodeConfig) {
    if (useMockMode()) {
      return callMockGateway('createPeerMock', payload)
    }
    return unwrap(request<PeerNodeConfig>('/peers', { method: 'POST', body: payload }))
  },

  async updatePeer(peerNodeId: string, payload: Omit<PeerNodeConfig, 'peer_node_id'>) {
    if (useMockMode()) {
      return callMockGateway('updatePeerMock', peerNodeId, payload)
    }
    return unwrap(request<PeerNodeConfig>(`/peers/${peerNodeId}`, { method: 'PUT', body: payload }))
  },

  async deletePeer(peerNodeId: string) {
    if (useMockMode()) {
      return callMockGateway('deletePeerMock', peerNodeId)
    }
    return unwrap<{ peer_node_id: string }>(request(`/peers/${peerNodeId}`, { method: 'DELETE' }))
  },

  async fetchNodeNetworkStatus() {
    if (useMockMode()) {
      return callMockGateway('fetchNodeNetworkStatusMock')
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
      return callMockGateway('fetchMappingsMock')
    }
    return unwrap<TunnelMappingListPayload>(request('/mappings', { method: 'GET' }))
  },
  async createMapping(payload: TunnelMapping) {
    if (useMockMode()) {
      return callMockGateway('createMappingMock', payload)
    }
    return unwrap<TunnelMappingSavePayload>(request('/mappings', { method: 'POST', body: payload }))
  },
  async updateMapping(id: string, payload: TunnelMapping) {
    if (useMockMode()) {
      return callMockGateway('updateMappingMock', id, payload)
    }
    return unwrap<TunnelMappingSavePayload>(request(`/mappings/${id}`, { method: 'PUT', body: payload }))
  },
  async deleteMapping(id: string) {
    if (useMockMode()) {
      return callMockGateway('deleteMappingMock', id)
    }
    await unwrap<Record<string, never>>(request(`/mappings/${id}`, { method: 'DELETE' }))
  },
  async testMapping() {
    if (useMockMode()) {
      return callMockGateway('testMappingMock')
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
  async runLinkTest(payload?: { target?: string }) {
    if (useMockMode()) {
      return {
        passed: true,
        status: 'passed',
        request_id: 'req-link-mock-001',
        trace_id: 'trace-link-mock-001',
        duration_ms: 42,
        checked_at: new Date().toISOString(),
        mock_target: 'http://10.20.0.20:8080/healthz',
        items: [
          { name: 'sip_control', passed: true, status: 'passed', detail: 'SIP TCP 控制面握手成功（无业务载荷）', duration_ms: 11 },
          { name: 'rtp_port_pool', passed: true, status: 'passed', detail: 'RTP 端口池可用: 874/1000', duration_ms: 8 },
          { name: 'http_downstream', passed: true, status: 'passed', detail: 'HTTP 已配置目标探测成功', duration_ms: 23 }
        ]
      } as OpsLinkTestReport
    }
    return unwrap(request<OpsLinkTestReport>('/ops/link-test', { method: 'POST', body: payload || {} }))
  },
  async fetchLatestLinkTest() {
    return unwrap(request<OpsLinkTestReport>('/ops/link-test', { method: 'GET' }))
  },

  async fetchAccessLogs(filters: AccessLogQuery, page: number, pageSize: number) {
    const result = await unwrap<{ items: AccessLogEntry[]; pagination: { total: number; page: number; page_size: number }; summary: AccessLogSummary }>(
      request('/access-logs', {
        method: 'GET',
        params: {
          status: filters.status,
          mapping: filters.mapping,
          source_ip: filters.sourceIP,
          method: filters.method,
          slow_only: filters.slowOnly,
          failed_only: filters.failedOnly,
          start_time: filters.startTime,
          end_time: filters.endTime,
          page,
          page_size: pageSize
        }
      })
    )
    return {
      list: result.items,
      total: result.pagination.total,
      page: result.pagination.page,
      pageSize: result.pagination.page_size,
      summary: result.summary
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

  async fetchDashboardTrends(range: string, granularity: string) {
    return unwrap<DashboardTrendSeries>(request('/dashboard/trends', { method: 'GET', params: { range, granularity } }))
  },

  async fetchNodeTunnelWorkspace() {
    const payload = await unwrap<any>(request('/node-tunnel/workspace', { method: 'GET' }))
    return mapNodeTunnelWorkspace(payload)
  },

  async saveNodeTunnelWorkspace(payload: NodeTunnelWorkspace) {
    const response = await unwrap<any>(request('/node-tunnel/workspace', { method: 'POST', body: toNodeTunnelWorkspacePayload(payload) }))
    return mapNodeTunnelWorkspace(response)
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
    return mapProtectionState(await unwrap<any>(request('/protection/state', { method: 'GET' })))
  },

  async fetchSystemResourceUsage() {
    return unwrap<SystemResourceUsage>(request('/system/resource-usage', { method: 'GET' }))
  },

  async upsertProtectionRestriction(payload: { scope: string; target: string; minutes: number; reason?: string }) {
    const data = await unwrap<any>(request('/protection/restrictions', { method: 'POST', body: payload }))
    return { ...data, state: data?.state ? mapProtectionState(data.state) : undefined } as ProtectionRestrictionActionResponse
  },

  async removeProtectionRestriction(scope: string, target: string) {
    const data = await unwrap<any>(request('/protection/restrictions', { method: 'DELETE', body: { scope, target } }))
    return { ...data, state: data?.state ? mapProtectionState(data.state) : undefined } as ProtectionRestrictionActionResponse
  },

  async restartGateway() {
    if (useMockMode()) {
      return { accepted: true, command: 'mock restart gateway', scheduled_at: new Date().toISOString() } as GatewayRestartResponse
    }
    return unwrap<GatewayRestartResponse>(request('/gateway/restart', { method: 'POST' }))
  },

  async fetchSystemSettings() {
    const [payload, summary] = await Promise.all([
      unwrap<SystemSettingsPayload>(request('/system/settings', { method: 'GET' })),
      this.fetchStartupSummary()
    ])
    return {
      sqlitePath: payload.sqlite_path,
      logPath: '/var/log/siptunnel',
      uiMode: summary.ui_mode,
      apiBaseUrl: summary.api_url,
      metricsEndpoint: `${summary.api_url.replace(/\/api$/, '')}/metrics`,
      readyEndpoint: `${summary.api_url.replace(/\/api$/, '')}/readyz`,
      selfCheckEndpoint: `${summary.api_url}/selfcheck`,
      startupSummaryEndpoint: `${summary.api_url}/startup-summary`,
      uiConsistencyStatus: summary.ui_delivery_summary?.consistency_status,
      uiConsistencyDetail: summary.ui_delivery_summary?.consistency_detail,
      uiEmbedBuildNonce: summary.ui_delivery_summary?.build_nonce,
      uiEmbeddedAt: summary.ui_delivery_summary?.embedded_at,
      uiSourceLatestWrite: summary.ui_delivery_summary?.ui_source_latest_write,
      uiEmbeddedHash: summary.ui_delivery_summary?.embedded_hash_sha256,
      uiAssetBaseMode: summary.ui_delivery_summary?.asset_base_mode,
      uiRouterBasePathPolicy: summary.ui_delivery_summary?.router_base_path_policy,
      uiDeliveryGuardStatus: summary.ui_delivery_summary?.delivery_guard_status,
      uiDeliveryGuardDetail: summary.ui_delivery_summary?.delivery_guard_detail,
      uiDeliveryGuardRemovedCount: summary.ui_delivery_summary?.delivery_guard_removed_count,
      uiDeliveryGuardRemainingCount: summary.ui_delivery_summary?.delivery_guard_remaining_count,
      uiDeliveryGuardHitCount: summary.ui_delivery_summary?.delivery_guard_hit_count,
      entrySelectionPolicy: summary.active_strategy_summary?.entry_selection_policy,
      udpControlHeaderPolicy: summary.active_strategy_summary?.udp_control_header_policy,
      genericDownloadRTPToleranceProfile: summary.active_strategy_summary?.generic_download_rtp_tolerance_profile,
      genericDownloadGuardPolicy: summary.active_strategy_summary?.generic_download_guard_policy,
      logRetentionDays: payload.max_task_age_days,
      logRetentionRecords: payload.max_task_records,
      auditRetentionDays: payload.max_audit_age_days,
      auditRetentionRecords: payload.max_audit_records,
      accessLogRetentionDays: payload.max_access_log_age_days,
      accessLogRetentionRecords: payload.max_access_log_records,
      diagnosticsRetentionDays: payload.max_diagnostic_age_days,
      diagnosticsRetentionRecords: payload.max_diagnostic_records,
      loadtestRetentionDays: payload.max_loadtest_age_days,
      loadtestRetentionRecords: payload.max_loadtest_records,
      cleanupCron: payload.log_cleanup_cron,
      adminCIDR: payload.admin_allow_cidr,
      mfaEnabled: payload.admin_require_mfa,
      genericDownloadTotalMbps: payload.generic_download_total_mbps ?? 0,
      genericDownloadPerTransferMbps: payload.generic_download_per_transfer_mbps ?? 0,
      genericDownloadWindowMB: payload.generic_download_window_mb ?? 0,
      adaptiveHotCacheMB: payload.adaptive_hot_cache_mb ?? 0,
      adaptiveHotWindowMB: payload.adaptive_hot_window_mb ?? 0,
      genericDownloadSegmentConcurrency: payload.generic_download_segment_concurrency ?? 0,
      genericDownloadRTPReorderWindowPackets: payload.generic_download_rtp_reorder_window_packets ?? 0,
      genericDownloadRTPLossTolerancePackets: payload.generic_download_rtp_loss_tolerance_packets ?? 0,
      genericDownloadRTPGapTimeoutMS: payload.generic_download_rtp_gap_timeout_ms ?? 0,
      genericDownloadRTPFECEnabled: payload.generic_download_rtp_fec_enabled ?? false,
      genericDownloadRTPFECGroupPackets: payload.generic_download_rtp_fec_group_packets ?? 0,
      adminTokenConfigured: payload.admin_token_configured,
      adminMfaConfigured: payload.admin_mfa_configured,
      configEncryptionEnabled: payload.config_encryption_enabled,
      tunnelSignerExternalized: payload.tunnel_signer_externalized,
      lastCleanupStatus: payload.cleaner_last_result,
      lastCleanupRemovedRecords: payload.cleaner_last_removed_records
    } as SystemSettingsState
  },

  async updateSystemSettings(payload: SystemSettingsState) {
    const req: SystemSettingsPayload = {
      sqlite_path: payload.sqlitePath,
      log_cleanup_cron: payload.cleanupCron,
      max_task_age_days: payload.logRetentionDays,
      max_task_records: payload.logRetentionRecords,
      max_access_log_age_days: payload.accessLogRetentionDays,
      max_access_log_records: payload.accessLogRetentionRecords,
      max_audit_age_days: payload.auditRetentionDays,
      max_audit_records: payload.auditRetentionRecords,
      max_diagnostic_age_days: payload.diagnosticsRetentionDays,
      max_diagnostic_records: payload.diagnosticsRetentionRecords,
      max_loadtest_age_days: payload.loadtestRetentionDays,
      max_loadtest_records: payload.loadtestRetentionRecords,
      admin_allow_cidr: payload.adminCIDR,
      admin_require_mfa: payload.mfaEnabled,
      generic_download_total_mbps: payload.genericDownloadTotalMbps,
      generic_download_per_transfer_mbps: payload.genericDownloadPerTransferMbps,
      generic_download_window_mb: payload.genericDownloadWindowMB,
      adaptive_hot_cache_mb: payload.adaptiveHotCacheMB,
      adaptive_hot_window_mb: payload.adaptiveHotWindowMB,
      generic_download_segment_concurrency: payload.genericDownloadSegmentConcurrency,
      generic_download_rtp_reorder_window_packets: payload.genericDownloadRTPReorderWindowPackets,
      generic_download_rtp_loss_tolerance_packets: payload.genericDownloadRTPLossTolerancePackets,
      generic_download_rtp_gap_timeout_ms: payload.genericDownloadRTPGapTimeoutMS,
      generic_download_rtp_fec_enabled: payload.genericDownloadRTPFECEnabled,
      generic_download_rtp_fec_group_packets: payload.genericDownloadRTPFECGroupPackets,
      cleaner_last_run_at: '',
      cleaner_last_result: payload.lastCleanupStatus,
      cleaner_last_removed_records: payload.lastCleanupRemovedRecords
    }
    await unwrap(request<SystemSettingsPayload>('/system/settings', { method: 'POST', body: req }))
    return this.fetchSystemSettings()
  },

  async fetchSecurityState() {
    const state = await unwrap<SecurityStatePayload>(request('/security/state', { method: 'GET' }))
    return {
      licenseStatus: state.license_status,
      expiryTime: state.expiry_time,
      activeTime: state.active_time,
      maintenanceExpireTime: state.maintenance_expire_time,
      licenseTime: state.license_time,
      productType: state.product_type,
      productTypeName: state.product_type_name || (state.product_type === '6' ? 'SIP隧道网关' : ''),
      licenseType: state.license_type,
      licenseCounter: state.license_counter,
      machineCode: state.machine_code,
      projectCode: state.project_code,
      licensedFeatures: state.licensed_features ?? [],
      lastValidation: state.last_validation,
      managementSecurity: state.management_security,
      signingAlgorithm: state.signing_algorithm,
      adminTokenConfigured: state.admin_token_configured,
      adminMFARequired: state.admin_mfa_required,
      adminMFAConfigured: state.admin_mfa_configured,
      configEncryption: state.config_encryption,
      signerExternalized: state.signer_externalized,
      adminTokenFingerprint: state.admin_token_fingerprint
    } satisfies SecurityCenterState
  },
  async fetchSecuritySettings() {
    return unwrap(request<{ signer: string; encryption: string; verify_interval_min: number }>('/security/settings', { method: 'GET' }))
  },
  async updateSecuritySettings(payload: { signer: string; encryption: string; verify_interval_min: number }) {
    return unwrap(request<{ signer: string; encryption: string; verify_interval_min: number }>('/security/settings', { method: 'PUT', body: payload }))
  },
  async fetchLicense() {
    return unwrap(request<LicensePayload>('/license', { method: 'GET' }))
  },
  async updateLicense(payload: { token?: string; content?: string }) {
    return unwrap(request<LicensePayload>('/license', { method: 'PUT', body: payload }))
  },

  async fetchMachineCode() {
    return unwrap<MachineCodePayload>(request('/license/machine-code', { method: 'GET' }))
  },

  async fetchSecurityEvents() {
    return unwrap<SecurityEventRecord[]>(request('/security/events', { method: 'GET' }))
  },

  async exportDiagnostics(requestId?: string, traceId?: string) {
    return unwrap<DiagnosticExportData>(request('/diagnostics/export', { method: 'GET', params: { request_id: requestId, trace_id: traceId } }))
  },

  async updateProtectionState(payload: Partial<AlertProtectionState> & { rps?: number; burst?: number; maxConcurrent?: number; failureThreshold?: number; recoveryWindowSec?: number }) {
    return mapProtectionState(await unwrap<any>(request('/protection/state', { method: 'PUT', body: { alert_rules: payload.alertRules, circuit_breaker_rules: payload.circuitBreakerRules, rps: payload.rps, burst: payload.burst, max_concurrent: payload.maxConcurrent, failure_threshold: payload.failureThreshold, recovery_window_sec: payload.recoveryWindowSec } })))
  },
  async recoverProtectionCircuit(target?: string) {
    const data = await unwrap<any>(request('/protection/circuit/recover', { method: 'POST', body: { target } }))
    return { ...data, state: data?.state ? mapProtectionState(data.state) : undefined } as ProtectionCircuitRecoverResponse
  },

  async fetchLoadtestJobs() {
    const data = await unwrap<{ items: LoadtestJob[] }>(request('/loadtests', { method: 'GET' }))
    return data.items
  },

  async startLoadtest(payload: { targets?: string[]; http_url?: string; sip_address?: string; rtp_address?: string; gateway_base_url?: string; concurrency?: number; qps?: number; duration_sec?: number; output_dir?: string }) {
    return unwrap<LoadtestJob>(request('/loadtests', { method: 'POST', body: payload }))
  },

  async fetchLoadtestJob(jobId: string) {
    return unwrap<LoadtestJob>(request(`/loadtests/${jobId}`, { method: 'GET' }))
  },

}
