export type TaskKind = 'command' | 'file'

export type TaskStatus =
  | 'pending'
  | 'accepted'
  | 'running'
  | 'transferring'
  | 'verifying'
  | 'retry_wait'
  | 'succeeded'
  | 'failed'
  | 'dead_lettered'
  | 'cancelled'

export interface DashboardMetrics {
  successRate: number
  failureRate: number
  concurrency: number
  rtpLossRate: number
  rateLimitHits: number
  sipProtocol: TransportProtocol
  sipListenPort: number
  rtpProtocol: TransportProtocol
  rtpPortRange: string
  activeSessions: number
  activeTransfers: number
  failedTasks24h: number
  rateLimitHits24h: number
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

export interface OpsLimits {
  rps: number
  burst: number
  maxConcurrent: number
}

export interface OpsRoute {
  api_code: string
  http_method: string
  http_path: string
  enabled: boolean
}

export interface OpsNode {
  node_id: string
  role: string
  status: string
  endpoint: string
}

export interface OpsAuditEvent {
  who: string
  when: string
  request_type: string
  validation_passed: boolean
  local_service_route: string
  final_result: string
  ops_action: string
  core: {
    request_id: string
    trace_id: string
    session_id: string
    api_code: string
    source_system: string
    source_node: string
    message_type: string
    result_code: string
    span_id: string
  }
}

export type TransportProtocol = 'TCP' | 'UDP'

export interface SipNetworkConfig {
  listenIp: string
  listenPort: number
  protocol: TransportProtocol
  advertisedAddress: string
  domain: string
  tcpKeepaliveEnabled: boolean
  tcpKeepaliveIntervalMs: number
  tcpReadBufferBytes: number
  tcpWriteBufferBytes: number
  maxConnections: number
}

export interface RtpNetworkConfig {
  listenIp: string
  portRangeStart: number
  portRangeEnd: number
  protocol: TransportProtocol
  advertisedAddress: string
  maxConcurrentTransfers: number
}

export interface PortPoolStatus {
  totalAvailablePorts: number
  occupiedPorts: number
  activeTransfers: number
}

export interface NetworkConfigPayload {
  sip: SipNetworkConfig
  rtp: RtpNetworkConfig
  portPool: PortPoolStatus
}

export interface UpdateNetworkConfigPayload {
  sip: SipNetworkConfig
  rtp: RtpNetworkConfig
}

export interface ConfigGovernanceSnapshot {
  version: string
  createdAt: string
  operator: string
  changeSummary: string
  status: 'active' | 'pending' | 'archived'
}

export interface RuntimeGatewayConfig {
  sip: {
    listen_port: number
    transport: TransportProtocol
    listen_ip: string
  }
  rtp: {
    port_start: number
    port_end: number
    transport: TransportProtocol
    listen_ip: string
  }
  max_message_bytes: number
  heartbeat_interval_sec: number
}

export interface ConfigDiffItem {
  path: string
  before: string
  after: string
  riskLevel: 'high' | 'medium' | 'low'
}

export interface ConfigSnapshotFilters {
  startTime?: string
  endTime?: string
  operator?: string
  version?: string
}

export interface ConfigGovernancePayload {
  snapshots: ConfigGovernanceSnapshot[]
  currentConfig: RuntimeGatewayConfig
  pendingConfig: RuntimeGatewayConfig
  diff: ConfigDiffItem[]
}

export type DiagnosticExportStatus = 'pending' | 'collecting' | 'packaging' | 'succeeded' | 'failed'

export interface DiagnosticExportCreatePayload {
  nodeId: string
  requestId?: string
  traceId?: string
}

export interface DiagnosticExportSection {
  key:
    | 'transport_config'
    | 'connection_stats_snapshot'
    | 'port_pool_status'
    | 'transport_error_summary'
    | 'task_failure_summary'
    | 'rate_limit_hit_summary'
    | 'profile_entry'
  label: string
  done: boolean
}

export interface DiagnosticExportJob {
  jobId: string
  nodeId: string
  status: DiagnosticExportStatus
  progress: number
  startedAt: string
  updatedAt: string
  fileName: string
  sections: DiagnosticExportSection[]
  errorMessage?: string
  downloadUrl?: string
}

export interface NodePortBindingStatus {
  service: 'SIP' | 'RTP'
  protocol: TransportProtocol
  bindAddress: string
  status: 'bound' | 'unbound' | 'degraded'
  updatedAt: string
}

export interface PortBindingFailureEvent {
  id: string
  occurredAt: string
  service: 'SIP' | 'RTP'
  reason: string
}

export interface NodeSelfCheckSummary {
  status: 'pass' | 'warn' | 'fail'
  checkedAt: string
  passed: number
  warning: number
  failed: number
  summary: string
}

export interface NodeOpsSnapshot {
  id: string
  status: 'online' | 'offline' | 'degraded'
  cpu: number
  memory: number
  backlog: number
  concurrency: number
  portBindings: NodePortBindingStatus[]
  bindingFailures: PortBindingFailureEvent[]
  selfCheck: NodeSelfCheckSummary
}
