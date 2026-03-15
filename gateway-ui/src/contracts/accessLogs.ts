import type { AccessLogEntry, AccessLogQuery, TaskListResult } from '../types/gateway'

export interface ViewContract<TRequest, TResponse> {
  feature: string
  apiPath: string
  handler: string
  dataSource: string
  request: TRequest
  response: TResponse
}

export type AccessLogsContract = ViewContract<AccessLogQuery, TaskListResult<AccessLogEntry>>

export const ACCESS_LOGS_CONTRACT: AccessLogsContract = {
  feature: '访问日志',
  apiPath: '/access-logs',
  handler: 'gatewayApi.fetchAccessLogs',
  dataSource: 'HTTP ingress / gateway request chain',
  request: {
    mapping: undefined,
    sourceIP: undefined,
    method: undefined,
    status: undefined,
    timeRange: undefined,
    slowOnly: false
  },
  response: {
    list: [],
    total: 0,
    page: 1,
    pageSize: 50
  }
}

