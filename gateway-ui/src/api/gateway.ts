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

export const gatewayApi = {
  fetchDashboard() {
    if (useMock) {
      return fetchDashboardMock()
    }
    return unwrap(request<DashboardPayload>('/dashboard', { method: 'GET' }))
  },
  fetchCommandTasks(filters: TaskListFilters, page: number, pageSize: number) {
    if (useMock) {
      return fetchCommandTasksMock(filters, page, pageSize)
    }
    return unwrap(
      request<TaskListResult<CommandTask>>('/tasks/command', {
        method: 'GET',
        params: { ...filters, page, pageSize }
      })
    )
  },
  fetchFileTasks(filters: TaskListFilters, page: number, pageSize: number) {
    if (useMock) {
      return fetchFileTasksMock(filters, page, pageSize)
    }
    return unwrap(
      request<TaskListResult<FileTask>>('/tasks/file', {
        method: 'GET',
        params: { ...filters, page, pageSize }
      })
    )
  },
  fetchTaskDetail(id: string, taskKind: TaskKind) {
    if (useMock) {
      return fetchTaskDetailMock(id, taskKind)
    }
    return unwrap(request<TaskDetail>(`/tasks/${taskKind}/${id}`, { method: 'GET' }))
  }
}
