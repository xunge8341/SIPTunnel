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
  currentConnections: number
  failedTasks1h: number
  transportErrors1h: number
  rateLimitHits1h: number
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

export type UiDeployMode = 'embedded' | 'external'

export interface DeploymentModePayload {
  uiMode: UiDeployMode
  uiUrl: string
  apiUrl: string
  configPath: string
  configSource: string
}

export interface BusinessExecutionStatus {
  state: 'active' | 'protocol_only'
  route_count: number
  message: string
  impact: string
}

export interface TunnelTransportPlan {
  request_meta_transport: string
  request_body_transport: string
  response_meta_transport: string
  response_body_transport: string
  request_body_size_limit: number
  response_body_size_limit: number
  notes: string[]
  warnings: string[]
}

export interface StartupSummaryPayload {
  node_id: string
  network_mode: string
  capability: Capability
  capability_summary: CapabilitySummary
  config_path: string
  config_source: string
  ui_mode: UiDeployMode
  ui_url: string
  api_url: string
  transport_plan: TunnelTransportPlan
  business_execution: BusinessExecutionStatus
  self_check_summary: {
    generated_at: string
    overall: 'info' | 'warn' | 'error'
    info: number
    warn: number
    error: number
  }
}

export interface Capability {
  supports_large_request_body: boolean
  supports_large_response_body: boolean
  supports_streaming_response: boolean
  supports_bidirectional_http_tunnel: boolean
  supports_transparent_proxy: boolean
}

export interface CapabilitySummary {
  supported: string[]
  unsupported: string[]
  items: CapabilityItem[]
}

export interface CapabilityItem {
  key: string
  label: string
  supported: boolean
  note: string
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

export interface TunnelMapping {
  mapping_id: string
  name: string
  enabled: boolean
  peer_node_id: string
  local_bind_ip: string
  local_bind_port: number
  local_base_path: string
  remote_target_ip: string
  remote_target_port: number
  remote_base_path: string
  allowed_methods: string[]
  connect_timeout_ms: number
  request_timeout_ms: number
  response_timeout_ms: number
  max_request_body_bytes: number
  max_response_body_bytes: number
  require_streaming_response: boolean
  description: string
}

export interface TunnelMappingListPayload {
  items: TunnelMapping[]
  warnings?: string[]
}

export interface TunnelMappingSavePayload {
  mapping: TunnelMapping
  warnings?: string[]
}

export interface OpsNode {
  node_id: string
  role: string
  status: string
  endpoint: string
}

export interface LocalNodeConfig {
  node_id: string
  node_name: string
  node_role: string
  network_mode: string
  sip_listen_ip: string
  sip_listen_port: number
  sip_transport: TransportProtocol
  rtp_listen_ip: string
  rtp_port_start: number
  rtp_port_end: number
  rtp_transport: TransportProtocol
}

export interface PeerNodeConfig {
  peer_node_id: string
  peer_name: string
  peer_signaling_ip: string
  peer_signaling_port: number
  peer_media_ip: string
  peer_media_port_start: number
  peer_media_port_end: number
  supported_network_mode: string
  enabled: boolean
}

export interface NodeConfigCheckResult {
  level: 'info' | 'warn' | 'error'
  message: string
  suggestion: string
  action_hint: string
}

export interface NodeDetailPayload {
  local_node: LocalNodeConfig
  current_network_mode: string
  current_capability: Capability
  compatibility_status: NodeConfigCheckResult
}

export interface NodeNetworkStatusPayload {
  network_mode: string
  capability: Capability
  current_network_mode: string
  current_capability: Capability
  compatibility_status: NodeConfigCheckResult
  capability_summary: CapabilitySummary
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

export interface OpsLinkTestItem {
  name: string
  passed: boolean
  status: 'passed' | 'failed'
  detail: string
  duration_ms: number
}

export interface OpsLinkTestReport {
  passed: boolean
  status: 'passed' | 'failed'
  request_id: string
  trace_id: string
  duration_ms: number
  checked_at: string
  mock_target: string
  items: OpsLinkTestItem[]
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
  usageRate: number
}

export interface ConnectionErrorEvent {
  id: string
  occurredAt: string
  transport: 'SIP' | 'RTP'
  protocol: TransportProtocol
  nodeId: string
  errorCode: string
  reason: string
}

export interface SelfCheckItem {
  key: string
  name: string
  level: 'info' | 'warn' | 'error'
  message: string
  suggestion: string
  action_hint: string
  doc_link?: string
}

export interface LinkTestResult {
  id: string
  scene: string
  status: 'pass' | 'warn' | 'fail'
  avgLatencyMs: number
  packetLossRate: number
  throughputMbps: number
  executedAt: string
}

export interface NetworkConfigPayload {
  sip: SipNetworkConfig
  rtp: RtpNetworkConfig
  portPool: PortPoolStatus
  connectionErrors: ConnectionErrorEvent[]
  selfCheckItems: SelfCheckItem[]
  linkTests: LinkTestResult[]
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
