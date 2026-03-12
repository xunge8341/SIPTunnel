import type {
  CommandTask,
  DashboardPayload,
  FileTask,
  TaskDetail,
  TaskKind,
  TaskListFilters,
  TaskListResult,
  TaskStatus
} from '../types/gateway'

const wait = (ms = 200) => new Promise((resolve) => setTimeout(resolve, ms))

const statuses: TaskStatus[] = ['pending', 'running', 'success', 'failed', 'partial_success']

const makeCommandTasks = (): CommandTask[] =>
  Array.from({ length: 24 }).map((_, index) => ({
    id: `cmd-${index + 1}`,
    requestId: `REQ-CMD-${String(index + 1).padStart(4, '0')}`,
    traceId: `TRACE-${(100000 + index).toString(16).toUpperCase()}`,
    apiCode: ['ORDER_SYNC', 'USER_QUERY', 'POLICY_PUSH'][index % 3],
    nodeId: `node-${(index % 4) + 1}`,
    status: statuses[index % statuses.length],
    createdAt: `2026-03-1${index % 8} 09:${String(index % 6)}0:00`,
    updatedAt: `2026-03-1${index % 8} 09:${String((index % 6) + 2)}2:00`,
    latencyMs: 80 + index * 7
  }))

const makeFileTasks = (): FileTask[] =>
  Array.from({ length: 24 }).map((_, index) => {
    const totalShards = 180 + index * 2
    const missingShards = index % 4
    const progress = Math.min(100, 72 + index)
    return {
      id: `file-${index + 1}`,
      requestId: `REQ-FILE-${String(index + 1).padStart(4, '0')}`,
      traceId: `TRACE-F-${(110000 + index).toString(16).toUpperCase()}`,
      filename: `payload_${index + 1}.dat`,
      status: statuses[(index + 1) % statuses.length],
      totalShards,
      missingShards,
      retryRounds: index % 3,
      checksumPassed: index % 6 !== 0,
      progress,
      createdAt: `2026-03-0${(index % 9) + 1} 11:${String(index % 6)}0:00`,
      updatedAt: `2026-03-0${(index % 9) + 1} 11:${String((index % 6) + 3)}2:00`
    }
  })

const commandTasks = makeCommandTasks()
const fileTasks = makeFileTasks()

const filterByCommon = <T extends { requestId: string; traceId: string; status: TaskStatus }>(
  list: T[],
  filters: TaskListFilters
) =>
  list.filter((item) => {
    if (filters.status && item.status !== filters.status) return false
    if (filters.requestId && !item.requestId.includes(filters.requestId.trim())) return false
    if (filters.traceId && !item.traceId.includes(filters.traceId.trim())) return false
    return true
  })

const paginate = <T>(list: T[], page: number, pageSize: number): TaskListResult<T> => {
  const start = (page - 1) * pageSize
  return {
    list: list.slice(start, start + pageSize),
    total: list.length,
    page,
    pageSize
  }
}

export async function fetchDashboardMock(): Promise<DashboardPayload> {
  await wait()
  return {
    metrics: {
      successRate: 99.2,
      failureRate: 0.8,
      concurrency: 186,
      rtpLossRate: 0.17,
      rateLimitHits: 31
    },
    recentTrends: [
      { time: '09:00', total: 120, success: 118, failed: 2 },
      { time: '10:00', total: 132, success: 129, failed: 3 },
      { time: '11:00', total: 150, success: 148, failed: 2 },
      { time: '12:00', total: 142, success: 139, failed: 3 },
      { time: '13:00', total: 166, success: 164, failed: 2 },
      { time: '14:00', total: 158, success: 155, failed: 3 },
      { time: '15:00', total: 171, success: 169, failed: 2 }
    ]
  }
}

export async function fetchCommandTasksMock(
  filters: TaskListFilters,
  page: number,
  pageSize: number
): Promise<TaskListResult<CommandTask>> {
  await wait()
  return paginate(filterByCommon(commandTasks, filters), page, pageSize)
}

export async function fetchFileTasksMock(
  filters: TaskListFilters,
  page: number,
  pageSize: number
): Promise<TaskListResult<FileTask>> {
  await wait()
  return paginate(filterByCommon(fileTasks, filters), page, pageSize)
}

export async function fetchTaskDetailMock(id: string, taskKind: TaskKind): Promise<TaskDetail> {
  await wait()
  return {
    id,
    taskKind,
    requestId: `${taskKind === 'command' ? 'REQ-CMD' : 'REQ-FILE'}-0088`,
    traceId: 'TRACE-DET-75FA',
    status: 'running',
    nodeId: 'node-2',
    createdAt: '2026-03-12 08:40:00',
    updatedAt: '2026-03-12 08:46:31',
    timeline: [
      { stage: '任务创建', status: 'done', time: '08:40:00', operator: 'scheduler', detail: '接收到 SIP INVITE' },
      { stage: 'SIP 协商', status: 'done', time: '08:40:08', operator: 'gateway-a', detail: '200 OK，建立会话' },
      { stage: 'RTP 传输', status: 'processing', time: '08:44:21', operator: 'gateway-b', detail: '进行中，触发过 1 次重传' },
      { stage: 'HTTP 执行', status: 'wait', time: '-', operator: '-', detail: '等待文件完整后触发' }
    ],
    sipEvents: [
      { time: '08:40:00', method: 'INVITE', code: 100, summary: 'Trying' },
      { time: '08:40:05', method: 'INVITE', code: 180, summary: 'Ringing' },
      { time: '08:40:08', method: 'INVITE', code: 200, summary: 'OK' },
      { time: '08:45:52', method: 'INFO', code: 200, summary: 'State sync success' }
    ],
    rtpStats: {
      totalShards: 210,
      receivedShards: 206,
      missingShards: 4,
      retransmittedShards: 4,
      bitrateMbps: 46.8
    },
    httpResult: {
      apiCode: 'ORDER_SYNC',
      url: '/internal/api/order/sync',
      method: 'POST',
      statusCode: 202,
      durationMs: 132,
      responseSnippet: '{"message":"accepted","job_id":"JOB-8871"}'
    },
    auditSnippets: [
      {
        id: 'AUD-901',
        time: '08:40:00',
        actor: 'system',
        action: 'CREATE_TASK',
        summary: '从 SIP 控制面创建任务，初始化幂等键。'
      },
      {
        id: 'AUD-902',
        time: '08:41:10',
        actor: 'ops_admin',
        action: 'UPDATE_RATE_LIMIT',
        summary: '调高 node-2 对 ORDER_SYNC 的并发阈值。'
      }
    ]
  }
}
