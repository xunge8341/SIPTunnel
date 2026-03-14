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
  fetchNodeNetworkStatusMock
} from './mockGateway'
import type {
  CommandTask,
  DashboardPayload,
  FileTask,
  OpsAuditEvent,
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
  NodeNetworkStatusPayload
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
  async fetchNodes() {
    const result = await unwrap<{ items: OpsNode[] }>(request('/nodes', { method: 'GET' }))
    return result.items
  },
  async fetchAudits(page: number, pageSize: number, query?: { requestId?: string; traceId?: string }) {
    const result = await unwrap<{ items: OpsAuditEvent[]; pagination: { total: number; page: number; page_size: number } }>(
      request('/audits', {
        method: 'GET',
        params: { page, page_size: pageSize, request_id: query?.requestId, trace_id: query?.traceId }
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
  }
}
