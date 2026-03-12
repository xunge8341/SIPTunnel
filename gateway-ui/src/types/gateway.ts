export type TaskKind = 'command' | 'file'

export type TaskStatus = 'pending' | 'running' | 'success' | 'failed' | 'partial_success'

export interface DashboardMetrics {
  successRate: number
  failureRate: number
  concurrency: number
  rtpLossRate: number
  rateLimitHits: number
}

export interface TrendPoint {
  time: string
  total: number
  success: number
  failed: number
}

export interface DashboardPayload {
  metrics: DashboardMetrics
  recentTrends: TrendPoint[]
}

export interface TaskListFilters {
  status?: TaskStatus
  requestId?: string
  traceId?: string
  startAt?: string
  endAt?: string
  nodeId?: string
}

export interface CommandTask {
  id: string
  requestId: string
  traceId: string
  apiCode: string
  nodeId: string
  status: TaskStatus
  createdAt: string
  updatedAt: string
  latencyMs: number
}

export interface FileTask {
  id: string
  requestId: string
  traceId: string
  filename: string
  status: TaskStatus
  totalShards: number
  missingShards: number
  retryRounds: number
  checksumPassed: boolean
  progress: number
  createdAt: string
  updatedAt: string
}

export interface TaskListResult<T> {
  list: T[]
  total: number
  page: number
  pageSize: number
}

export interface TimelineItem {
  stage: string
  status: 'done' | 'processing' | 'wait'
  time: string
  operator: string
  detail: string
}

export interface SipEvent {
  time: string
  method: string
  code: number
  summary: string
}

export interface HttpExecutionResult {
  apiCode: string
  url: string
  method: string
  statusCode: number
  durationMs: number
  responseSnippet: string
}

export interface AuditSnippet {
  id: string
  time: string
  actor: string
  action: string
  summary: string
}

export interface TaskDetail {
  id: string
  taskKind: TaskKind
  requestId: string
  traceId: string
  status: TaskStatus
  nodeId: string
  createdAt: string
  updatedAt: string
  timeline: TimelineItem[]
  sipEvents: SipEvent[]
  rtpStats: {
    totalShards: number
    receivedShards: number
    missingShards: number
    retransmittedShards: number
    bitrateMbps: number
  }
  httpResult: HttpExecutionResult
  auditSnippets: AuditSnippet[]
}
