import { request } from './http'
import {
  fetchCommandTasksMock,
  fetchDashboardMock,
  fetchFileTasksMock,
  fetchTaskDetailMock
} from './mockGateway'
import type {
  CommandTask,
  DashboardPayload,
  FileTask,
  OpsAuditEvent,
  OpsLimits,
  OpsNode,
  OpsRoute,
  TaskDetail,
  TaskKind,
  TaskListFilters,
  TaskListResult
} from '../types/gateway'

const useMock = import.meta.env.VITE_API_MODE !== 'real'

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
    if (useMock) {
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
        rateLimitHits: 0
      },
      recentTrends: []
    } as DashboardPayload
  },
  async fetchCommandTasks(filters: TaskListFilters, page: number, pageSize: number) {
    if (useMock) {
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
    if (useMock) {
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
    if (useMock) {
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
  }
}
