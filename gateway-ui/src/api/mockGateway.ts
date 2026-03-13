import type {
  CommandTask,
  DashboardPayload,
  FileTask,
  TaskDetail,
  TaskKind,
  TaskListFilters,
  TaskListResult,
  TaskStatus,
  NetworkConfigPayload,
  UpdateNetworkConfigPayload,
  ConfigGovernancePayload,
  ConfigSnapshotFilters,
  RuntimeGatewayConfig,
  DiagnosticExportJob,
  DiagnosticExportCreatePayload
} from '../types/gateway'

const wait = (ms = 200) => new Promise((resolve) => setTimeout(resolve, ms))

const statuses: TaskStatus[] = ['pending', 'running', 'succeeded', 'failed', 'retry_wait']

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
      rateLimitHits: 31,
      sipProtocol: 'UDP',
      sipListenPort: 5060,
      rtpProtocol: 'UDP',
      rtpPortRange: '20000-20999',
      activeSessions: 128,
      activeTransfers: 42,
      failedTasks24h: 19,
      rateLimitHits24h: 31
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


let networkConfigState: NetworkConfigPayload = {
  sip: {
    listenIp: '0.0.0.0',
    listenPort: 5060,
    protocol: 'UDP',
    advertisedAddress: 'sip.siptunnel.local:5060',
    domain: 'siptunnel.local',
    tcpKeepaliveEnabled: true,
    tcpKeepaliveIntervalMs: 30000,
    tcpReadBufferBytes: 65536,
    tcpWriteBufferBytes: 65536,
    maxConnections: 2048
  },
  rtp: {
    listenIp: '0.0.0.0',
    portRangeStart: 20000,
    portRangeEnd: 20999,
    protocol: 'UDP',
    advertisedAddress: 'rtp.siptunnel.local',
    maxConcurrentTransfers: 240
  },
  portPool: {
    totalAvailablePorts: 1000,
    occupiedPorts: 126,
    activeTransfers: 58
  }
}

export async function fetchNetworkConfigMock(): Promise<NetworkConfigPayload> {
  await wait()
  return JSON.parse(JSON.stringify(networkConfigState))
}

export async function updateNetworkConfigMock(payload: UpdateNetworkConfigPayload): Promise<NetworkConfigPayload> {
  await wait()
  networkConfigState = {
    ...networkConfigState,
    sip: JSON.parse(JSON.stringify(payload.sip)),
    rtp: JSON.parse(JSON.stringify(payload.rtp))
  }
  const span = Math.max(0, networkConfigState.rtp.portRangeEnd - networkConfigState.rtp.portRangeStart + 1)
  networkConfigState.portPool.totalAvailablePorts = span
  networkConfigState.portPool.occupiedPorts = Math.min(span, Math.round(payload.rtp.maxConcurrentTransfers * 0.6))
  networkConfigState.portPool.activeTransfers = Math.min(
    networkConfigState.portPool.occupiedPorts,
    Math.round(payload.rtp.maxConcurrentTransfers * 0.35)
  )
  return JSON.parse(JSON.stringify(networkConfigState))
}


const configSnapshotsSeed = [
  { version: 'v2026.03.12.1', createdAt: '2026-03-12 09:10:00', operator: 'ops_admin', changeSummary: '调整 RTP 端口池容量', status: 'active' as const },
  { version: 'v2026.03.12.2', createdAt: '2026-03-12 11:32:00', operator: 'secops', changeSummary: '切换 SIP 传输协议到 TCP', status: 'archived' as const },
  { version: 'v2026.03.12.3', createdAt: '2026-03-12 13:45:00', operator: 'ops_admin', changeSummary: '提升 max_message_bytes 以支持大报文', status: 'pending' as const },
  { version: 'v2026.03.12.4', createdAt: '2026-03-12 15:21:00', operator: 'release_bot', changeSummary: '校准 RTP 端口区间', status: 'archived' as const }
]

let snapshotState = [...configSnapshotsSeed]

const runtimeCurrentConfig: RuntimeGatewayConfig = {
  sip: {
    listen_port: 5060,
    transport: 'UDP',
    listen_ip: '0.0.0.0'
  },
  rtp: {
    port_start: 20000,
    port_end: 20999,
    transport: 'UDP',
    listen_ip: '0.0.0.0'
  },
  max_message_bytes: 1048576,
  heartbeat_interval_sec: 15
}

let runtimePendingConfig: RuntimeGatewayConfig = {
  sip: {
    listen_port: 5061,
    transport: 'TCP',
    listen_ip: '0.0.0.0'
  },
  rtp: {
    port_start: 21000,
    port_end: 21999,
    transport: 'UDP',
    listen_ip: '0.0.0.0'
  },
  max_message_bytes: 2097152,
  heartbeat_interval_sec: 15
}

const buildDiff = (before: RuntimeGatewayConfig, after: RuntimeGatewayConfig) => [
  { path: 'sip.listen_port', before: String(before.sip.listen_port), after: String(after.sip.listen_port), riskLevel: 'high' as const },
  { path: 'sip.transport', before: before.sip.transport, after: after.sip.transport, riskLevel: 'high' as const },
  { path: 'rtp.port_start', before: String(before.rtp.port_start), after: String(after.rtp.port_start), riskLevel: 'high' as const },
  { path: 'rtp.port_end', before: String(before.rtp.port_end), after: String(after.rtp.port_end), riskLevel: 'high' as const },
  { path: 'rtp.transport', before: before.rtp.transport, after: after.rtp.transport, riskLevel: 'high' as const },
  { path: 'max_message_bytes', before: String(before.max_message_bytes), after: String(after.max_message_bytes), riskLevel: 'high' as const },
  { path: 'heartbeat_interval_sec', before: String(before.heartbeat_interval_sec), after: String(after.heartbeat_interval_sec), riskLevel: 'low' as const }
]

const toYaml = (config: RuntimeGatewayConfig) => `sip:
  listen_port: ${config.sip.listen_port}
  transport: ${config.sip.transport}
  listen_ip: ${config.sip.listen_ip}
rtp:
  port_start: ${config.rtp.port_start}
  port_end: ${config.rtp.port_end}
  transport: ${config.rtp.transport}
  listen_ip: ${config.rtp.listen_ip}
max_message_bytes: ${config.max_message_bytes}
heartbeat_interval_sec: ${config.heartbeat_interval_sec}
`

export async function fetchConfigGovernanceMock(filters: ConfigSnapshotFilters): Promise<ConfigGovernancePayload> {
  await wait()
  const snapshots = snapshotState.filter((item) => {
    if (filters.operator && !item.operator.includes(filters.operator.trim())) return false
    if (filters.version && !item.version.includes(filters.version.trim())) return false
    if (filters.startTime && item.createdAt < filters.startTime) return false
    if (filters.endTime && item.createdAt > filters.endTime) return false
    return true
  })

  return {
    snapshots,
    currentConfig: JSON.parse(JSON.stringify(runtimeCurrentConfig)),
    pendingConfig: JSON.parse(JSON.stringify(runtimePendingConfig)),
    diff: buildDiff(runtimeCurrentConfig, runtimePendingConfig)
  }
}

export async function rollbackConfigMock(version: string): Promise<ConfigGovernancePayload> {
  await wait()
  snapshotState = snapshotState.map((item) => ({
    ...item,
    status: item.version === version ? 'active' : item.status === 'active' ? 'archived' : item.status
  }))
  runtimePendingConfig = JSON.parse(JSON.stringify(runtimeCurrentConfig))

  return {
    snapshots: snapshotState,
    currentConfig: JSON.parse(JSON.stringify(runtimeCurrentConfig)),
    pendingConfig: JSON.parse(JSON.stringify(runtimePendingConfig)),
    diff: buildDiff(runtimeCurrentConfig, runtimePendingConfig)
  }
}

export async function exportConfigYamlMock(target: 'current' | 'pending'): Promise<string> {
  await wait(120)
  return toYaml(target === 'current' ? runtimeCurrentConfig : runtimePendingConfig)
}


const diagnosticSectionsTemplate = [
  { key: 'transport_config' as const, label: '当前 transport 配置' },
  { key: 'connection_stats_snapshot' as const, label: '连接统计快照' },
  { key: 'port_pool_status' as const, label: '端口池状态' },
  { key: 'transport_error_summary' as const, label: '最近 transport 错误摘要' },
  { key: 'task_failure_summary' as const, label: '最近 task failure 摘要' },
  { key: 'rate_limit_hit_summary' as const, label: '最近 rate limit 命中摘要' },
  { key: 'profile_entry' as const, label: 'profile 采集入口信息（如果启用）' }
]

const diagnosticJobs = new Map<string, {
  job: DiagnosticExportJob
  polls: number
  failOnce: boolean
}>()

const makeDiagnosticFileName = (nodeId: string, jobId: string, requestId?: string, traceId?: string) => {
  const stamp = new Date().toISOString().replace(/[-:]/g, '').replace(/\..+/, '')
  const normalizedNodeId = nodeId.replace(/-/g, '_')
  const parts = [`diag_${normalizedNodeId}_${stamp}`]
  if (requestId) parts.push(`req_${requestId}`)
  if (traceId) parts.push(`trace_${traceId}`)
  parts.push(jobId)
  return `${parts.join('_')}.zip`
}

const cloneJob = (job: DiagnosticExportJob): DiagnosticExportJob => JSON.parse(JSON.stringify(job))

export async function createDiagnosticExportMock(payload: DiagnosticExportCreatePayload): Promise<DiagnosticExportJob> {
  await wait(150)
  const jobId = `diag-${Math.random().toString(36).slice(2, 8)}`
  const now = new Date().toISOString()
  const job: DiagnosticExportJob = {
    jobId,
    nodeId: payload.nodeId,
    status: 'pending',
    progress: 0,
    startedAt: now,
    updatedAt: now,
    fileName: makeDiagnosticFileName(payload.nodeId, jobId, payload.requestId, payload.traceId),
    sections: diagnosticSectionsTemplate.map((item) => ({ ...item, done: false }))
  }
  diagnosticJobs.set(jobId, {
    job,
    polls: 0,
    failOnce: payload.nodeId.endsWith('02')
  })
  return cloneJob(job)
}

export async function getDiagnosticExportMock(jobId: string): Promise<DiagnosticExportJob> {
  await wait(180)
  const record = diagnosticJobs.get(jobId)
  if (!record) {
    throw new Error('诊断任务不存在，请重新发起导出')
  }

  if (record.job.status === 'succeeded' || record.job.status === 'failed') {
    return cloneJob(record.job)
  }

  record.polls += 1
  const progress = Math.min(100, record.polls * 25)
  record.job.progress = progress
  record.job.updatedAt = new Date().toISOString()
  record.job.status = progress < 40 ? 'collecting' : 'packaging'

  const completedCount = Math.floor((progress / 100) * record.job.sections.length)
  record.job.sections = record.job.sections.map((item, index) => ({ ...item, done: index < completedCount }))

  if (progress >= 100) {
    if (record.failOnce) {
      record.job.status = 'failed'
      record.job.errorMessage = '导出包生成失败：日志索引服务暂时不可用。'
      record.failOnce = false
    } else {
      record.job.status = 'succeeded'
      record.job.errorMessage = undefined
      record.job.sections = record.job.sections.map((item) => ({ ...item, done: true }))
      record.job.downloadUrl = `data:application/zip;base64,UEsFBgAAAAAAAAAAAAAAAAAAAAAAAA==`
    }
  }

  return cloneJob(record.job)
}

export async function retryDiagnosticExportMock(jobId: string): Promise<DiagnosticExportJob> {
  await wait(160)
  const record = diagnosticJobs.get(jobId)
  if (!record) {
    throw new Error('诊断任务不存在，请重新发起导出')
  }
  const now = new Date().toISOString()
  record.polls = 0
  record.job.status = 'pending'
  record.job.progress = 0
  record.job.updatedAt = now
  record.job.errorMessage = undefined
  record.job.downloadUrl = undefined
  record.job.sections = diagnosticSectionsTemplate.map((item) => ({ ...item, done: false }))
  return cloneJob(record.job)
}
