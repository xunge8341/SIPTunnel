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

export interface PeerBinding {
  peer_node_id: string
  peer_name?: string
  peer_signaling_ip?: string
  peer_signaling_port?: number
}

export interface StartupSummaryPayload {
  node_id: string
  network_mode: string
  bound_peer?: PeerBinding
  peer_binding_error?: string
  capability: Capability
  capability_summary: CapabilitySummary
  config_path: string
  config_source: string
  ui_mode: UiDeployMode
  ui_url: string
  api_url: string
  transport_plan: TunnelTransportPlan
  active_strategy_summary?: {
    entry_selection_policy: string
    udp_control_header_policy: string
    generic_download_rtp_tolerance_profile: string
    generic_download_guard_policy?: string
  }
  ui_delivery_summary?: {
    metadata_present: boolean
    consistency_status: string
    consistency_detail: string
    build_nonce: string
    embedded_at: string
    ui_source_latest_write: string
    embedded_hash_sha256: string
    asset_base_mode: string
    router_base_path_policy: string
    delivery_guard_status: string
    delivery_guard_detail: string
    delivery_guard_removed_count: number
    delivery_guard_remaining_count: number
    delivery_guard_hit_count: number
  }
  business_execution: BusinessExecutionStatus
  self_check_summary: {
    generated_at: string
    overall: 'info' | 'warn' | 'error'
    info: number
    warn: number
    error: number
  }
}

export interface SystemStatusCapability {
  supports_small_request_body: boolean
  supports_large_response_body: boolean
  supports_streaming_response: boolean
  supports_large_file_upload: boolean
  supports_bidirectional_http_tunnel: boolean
}

export interface SystemStatusPayload {
  tunnel_status: 'connected' | 'disconnected' | 'degraded'
  connection_reason: string
  network_mode: string
  registration_status?: 'registered' | 'unregistered' | 'degraded'
  heartbeat_status?: 'healthy' | 'timeout' | 'unknown'
  last_register_time?: string
  last_heartbeat_time?: string
  mapping_total?: number
  mapping_abnormal_total?: number
  latest_mapping_error_reason?: string
  bound_peer?: PeerBinding
  peer_binding_error?: string
  capability: SystemStatusCapability
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

// 兼容 API（历史模型 route/api_code）返回结构，非主线术语。
export interface OpsRoute {
  api_code: string
  http_method: string
  http_path: string
  enabled: boolean
}

export interface TunnelMapping {
  mapping_id: string
  device_id?: string
  resource_code?: string
  resource_type?: string
  name?: string
  enabled: boolean
  peer_node_id?: string
  local_bind_ip: string
  local_bind_port: number
  local_base_path: string
  remote_target_ip: string
  remote_target_port: number
  remote_base_path: string
  allowed_methods?: string[]
  response_mode?: 'AUTO' | 'INLINE' | 'RTP'
  connect_timeout_ms: number
  request_timeout_ms: number
  response_timeout_ms: number
  max_inline_response_body?: number
  max_request_body_bytes: number
  max_response_body_bytes: number
  require_streaming_response: boolean
  description: string
  updated_at?: string
  link_status?:
    | 'connected'
    | 'disabled'
    | 'listening'
    | 'start_failed'
    | 'interrupted'
    | 'abnormal'
  link_status_text?: '未启用' | '监听中' | '已连接' | '异常' | '启动失败'
  status_reason?: string
  failure_reason?: string
  suggested_action?: string
  request_count?: number
  failure_count?: number
  avg_latency_ms?: number
}

export interface TunnelMappingListPayload {
  items: TunnelMapping[]
  bound_peer?: PeerBinding
  binding_error?: string
  warnings?: string[]
}

export interface TunnelMappingSavePayload {
  mapping: TunnelMapping
  warnings?: string[]
}

export interface LocalResourceItem {
  resource_id: string
  resource_code: string
  device_id: string
  resource_type: string
  name: string
  enabled: boolean
  target_url: string
  methods: string[]
  response_mode: 'AUTO' | 'INLINE' | 'RTP'
  max_inline_response_body: number
  max_request_body: number
  max_response_body: number
  body_limit_policy?: string
  description: string
  updated_at?: string
  entry_hint?: string
  local_bind_port?: number
  mapping_id?: string
}

export interface LocalResourceListPayload {
  items: LocalResourceItem[]
}

export interface LocalResourceSavePayload {
  resource_code: string
  name: string
  enabled: boolean
  target_url: string
  methods: string[]
  response_mode: 'AUTO' | 'INLINE' | 'RTP'
  description: string
}

export interface TunnelMappingOverviewItem {
  resource_code: string
  device_id: string
  mapping_id?: string
  local_bind_port?: number
  resource_type?: string
  name: string
  source_node?: string
  methods: string[]
  response_mode: 'AUTO' | 'INLINE' | 'RTP'
  resource_status: string
  mapping_status: 'UNMAPPED' | 'MANUAL'
  mapping_ids?: string[]
  listen_ip?: string
  listen_ports: number[]
  path_prefix?: string
  enabled: boolean
}

export interface TunnelMappingOverviewSummary {
  resource_total: number
  mapped_total: number
  manual_total: number
  unmapped_total: number
}

export interface TunnelMappingOverviewPayload {
  items: TunnelMappingOverviewItem[]
  summary: TunnelMappingOverviewSummary
}

export interface MappingTestStage {
  key: string
  name: string
  status: 'passed' | 'failed' | 'blocked'
  passed: boolean
  detail: string
  blocking_reason?: string
  suggested_action?: string
}

export interface MappingTestPayload {
  passed: boolean
  status: 'passed' | 'failed'
  stages: MappingTestStage[]
  failure_stage?: string
  signaling_request: '成功' | '失败'
  response_channel: '正常' | '异常'
  registration_status: '正常' | '未注册'
  failure_reason?: string
  suggested_action?: string
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
  mapping_port_start: number
  mapping_port_end: number
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

export interface TunnelConfigCapabilityItem {
  key: string
  supported: boolean
  description: string
}

export interface TunnelConfigCapability {
  supports_small_request_body: boolean
  supports_large_request_body: boolean
  supports_large_response_body: boolean
  supports_streaming_response: boolean
  supports_bidirectional_http_tunnel: boolean
  supports_transparent_http_proxy: boolean
}

export interface TunnelConfigPayload {
  channel_protocol: string
  connection_initiator: 'LOCAL' | 'PEER'
  mapping_relay_mode?: 'AUTO' | 'SIP_ONLY'
  local_device_id: string
  peer_device_id: string
  heartbeat_interval_sec: number
  register_retry_count: number
  register_retry_interval_sec: number
  registration_status: string
  last_register_time: string
  last_heartbeat_time: string
  heartbeat_status: string
  last_failure_reason: string
  next_retry_time: string
  consecutive_heartbeat_timeout: number
  supported_capabilities: string[]
  request_channel: string
  response_channel: string
  network_mode: string
  capability: TunnelConfigCapability
  capability_items: TunnelConfigCapabilityItem[]
  register_auth_enabled?: boolean
  register_auth_username?: string
  register_auth_password?: string
  register_auth_password_configured?: boolean
  register_auth_realm?: string
  register_auth_algorithm?: string
  catalog_subscribe_expires_sec?: number
}

export interface TunnelConfigUpdatePayload {
  channel_protocol: string
  connection_initiator: 'LOCAL' | 'PEER'
  mapping_relay_mode?: 'AUTO' | 'SIP_ONLY'
  heartbeat_interval_sec: number
  register_retry_count: number
  register_retry_interval_sec: number
  network_mode: string
  register_auth_enabled?: boolean
  register_auth_username?: string
  register_auth_password?: string
  register_auth_password_configured?: boolean
  register_auth_realm?: string
  register_auth_algorithm?: string
  catalog_subscribe_expires_sec?: number
}

export interface TunnelCatalogResource {
  resource_code?: string
  device_id: string
  resource_type?: string
  name: string
  local_port?: number
  local_ports?: number[]
  exposure_mode: 'MANUAL' | 'UNEXPOSED'
  methods: string[]
  method_list?: string[]
  response_mode: 'AUTO' | 'INLINE' | 'RTP'
  source?: 'REMOTE' | 'LOCAL'
  status?: string
  max_inline_response_body?: number
  max_request_body?: number
  mapping_ids?: string[]
}

export interface TunnelCatalogSummary {
  resource_total: number
  manual_expose_num: number
  unexposed_num: number
}

export interface TunnelCatalogPayload {
  resources: TunnelCatalogResource[]
  summary: TunnelCatalogSummary
}

export interface GB28181PeerState {
  device_id: string
  node_type?: string
  remote_addr: string
  callback_addr: string
  transport: string
  last_register_at?: string
  register_expires_at?: string
  last_keepalive_at?: string
  subscribed_at?: string
  subscription_expires_at?: string
  last_catalog_notify_at?: string
  auth_required: boolean
  last_error?: string
}

export interface GB28181PendingSession {
  call_id: string
  device_id: string
  mapping_id: string
  response_mode: string
  stage?: string
  last_stage_at?: string
  last_error?: string
  started_at?: string
}

export interface GB28181InboundSession {
  call_id: string
  device_id: string
  mapping_id: string
  callback_addr: string
  transport: string
  remote_rtp_ip: string
  remote_rtp_port: number
  stage?: string
  last_stage_at?: string
  last_error?: string
  started_at?: string
  last_invoke_at?: string
}

export interface GB28181CatalogState {
  resource_total: number
  exposed_total: number
}

export interface GB28181RuntimeSnapshot {
  peers: GB28181PeerState[]
  pending_sessions: GB28181PendingSession[]
  inbound_sessions: GB28181InboundSession[]
  catalog: GB28181CatalogState
  updated_at: string
}

export interface GB28181StatePayload {
  session: TunnelSessionRuntimeState
  config: TunnelConfigPayload
  gb28181?: GB28181RuntimeSnapshot
}

export interface LinkMonitorPayload {
  session: TunnelSessionRuntimeState
  config: TunnelConfigPayload
  gb28181?: GB28181RuntimeSnapshot
  mapping_summary?: TunnelMappingOverviewSummary
  live_status: string
  ready_status: string
  readiness_reasons?: string[]
  updated_at: string
}

export interface TunnelSessionActionPayload {
  action: 'register_now' | 'reregister' | 'heartbeat_once'
}

export interface TunnelCatalogActionPayload {
  action: 'pull_remote' | 'push_local' | 'refresh_all'
}

export interface TunnelCatalogActionResponse {
  action: string
  subscribe_triggered?: number
  notify_triggered?: number
  gb28181?: GB28181RuntimeSnapshot
}

export interface TunnelSessionRuntimeState {
  registration_status: string
  heartbeat_status: string
  phase?: string
  phase_updated_at?: string
  last_register_time: string
  last_heartbeat_time: string
  last_failure_reason: string
  next_retry_time: string
  consecutive_heartbeat_timeout: number
}

export interface TunnelSessionActionResponse {
  action: string
  state: TunnelSessionRuntimeState
}

export interface NodeConfigEndpoint {
  node_ip: string
  signaling_port: number
  device_id: string
  node_type?: string
  rtp_port_start?: number
  rtp_port_end?: number
  mapping_port_start?: number
  mapping_port_end?: number
}

export interface NodeConfigPayload {
  local_node: NodeConfigEndpoint
  peer_node: NodeConfigEndpoint
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
  bound_peer?: PeerBinding
  peer_binding_error?: string
}

export interface OpsAuditFilters {
  requestId?: string
  traceId?: string
  rule?: string
  errorOnly?: boolean
  startTime?: string
  endTime?: string
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
  name?: string
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
  name?: string
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

export interface ConfigTransferPayload {
  version: string
  exported_at: string
  network_config: UpdateNetworkConfigPayload
  tunnel_config: TunnelConfigPayload
  node_config: NodeConfigPayload
}

export interface ConfigTransferImportResult {
  imported: boolean
  tunnel_restarted: boolean
  version: string
  message: string
}

export type DiagnosticExportStatus =
  | 'pending'
  | 'collecting'
  | 'packaging'
  | 'succeeded'
  | 'failed'

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

export interface AccessLogSummary {
  total: number
  failed: number
  slow: number
  error_types: Record<string, number>
  window: string
}

export interface AccessLogEntry {
  id: string
  occurred_at: string
  mapping_name: string
  source_ip: string
  method: string
  path: string
  status_code: number
  duration_ms: number
  failure_reason: string
  request_id: string
  trace_id: string
}

export interface AccessLogFilters {
  status?: TaskStatus
  requestId?: string
  traceId?: string
}

export interface SystemSettingsPayload {
  sqlite_path: string
  log_cleanup_cron: string
  max_task_age_days: number
  max_task_records: number
  max_access_log_age_days: number
  max_access_log_records: number
  max_audit_age_days: number
  max_audit_records: number
  max_diagnostic_age_days: number
  max_diagnostic_records: number
  max_loadtest_age_days: number
  max_loadtest_records: number
  admin_allow_cidr: string
  admin_require_mfa: boolean
  generic_download_total_mbps?: number
  generic_download_per_transfer_mbps?: number
  generic_download_window_mb?: number
  adaptive_hot_cache_mb?: number
  adaptive_hot_window_mb?: number
  generic_download_segment_concurrency?: number
  generic_download_rtp_reorder_window_packets?: number
  generic_download_rtp_loss_tolerance_packets?: number
  generic_download_rtp_gap_timeout_ms?: number
  generic_download_rtp_fec_enabled?: boolean
  generic_download_rtp_fec_group_packets?: number
  admin_token_configured?: boolean
  admin_mfa_configured?: boolean
  config_encryption_enabled?: boolean
  tunnel_signer_externalized?: boolean
  cleaner_last_run_at: string
  cleaner_last_result: string
  cleaner_last_removed_records: number
}

export interface DashboardOpsSummaryItem {
  name: string
  count: number
  avg_latency_ms?: number
}
export interface DashboardOpsSummaryPayload {
  top_mappings: DashboardOpsSummaryItem[]
  top_source_ips: DashboardOpsSummaryItem[]
  top_failed_mappings: DashboardOpsSummaryItem[]
  top_failed_source_ips: DashboardOpsSummaryItem[]
  rate_limit_status: string
  circuit_breaker_state: string
  protection_status: string
}

export interface DashboardSummary {
  systemHealth: string
  activeConnections: number
  mappingTotal: number
  mappingErrorCount: number
  recentFailureCount: number
  rateLimitState: string
  circuitBreakerState: string
}

export interface DashboardOpsSummary {
  hotMappings: DashboardOpsSummaryItem[]
  topFailureMappings: DashboardOpsSummaryItem[]
  hotSourceIPs: DashboardOpsSummaryItem[]
  topFailureIPs: DashboardOpsSummaryItem[]
}

export interface DashboardTrendPoint {
  bucket: string
  label: string
  total: number
  failed: number
  slow: number
}

export interface DashboardTrendSeries {
  range: string
  granularity: string
  points: DashboardTrendPoint[]
}

export interface NodeTunnelWorkspace {
  localNode: NodeConfigEndpoint
  peerNode: NodeConfigEndpoint
  networkMode: string
  capabilityMatrix: { key: string; supported: boolean }[]
  sipCapability: Record<string, unknown>
  rtpCapability: Record<string, unknown>
  sessionSettings: TunnelConfigPayload
  securitySettings: {
    signer: string
    encryption: string
    verify_interval_min: number
    admin_allow_cidr?: string
    admin_require_mfa?: boolean
  }
  encryptionSettings: { algorithm: string }
}

export interface MappingWorkspaceItem {
  mappingName: string
  localEntry: string
  peerTarget: string
  status: string
  lastTestResult: string
  requestCount: number
  failureCount: number
  avgLatency: number
  riskLevel: string
  mappingId: string
}

export interface MappingWorkspaceList {
  items: MappingWorkspaceItem[]
}

export interface AccessLogQuery {
  mapping?: string
  sourceIP?: string
  method?: string
  status?: number
  startTime?: string
  endTime?: string
  slowOnly?: boolean
  failedOnly?: boolean
}

export interface ProtectionTargetStat {
  target: string
  count: number
}

export interface ProtectionScopeSnapshot {
  scope: string
  label: string
  rps: number
  burst: number
  max_concurrent: number
  active_requests: number
  rate_limit_hits_total: number
  concurrent_rejects_total: number
  allowed_total: number
  top_rate_limit_targets?: ProtectionTargetStat[]
  top_concurrent_targets?: ProtectionTargetStat[]
  top_allowed_targets?: ProtectionTargetStat[]
}

export interface CircuitEntrySnapshot {
  key: string
  state: string
  open_until?: string
  last_cause?: string
  consecutive_failures?: number
}

export interface ProtectionRestrictionSnapshot {
  scope: 'source' | 'mapping' | string
  target: string
  reason?: string
  created_at?: string
  expires_at?: string
  minutes?: number
  active: boolean
}

export interface SystemResourceUsage {
  captured_at: string
  cpu_cores: number
  gomaxprocs: number
  goroutines: number
  heap_alloc_bytes: number
  heap_sys_bytes: number
  heap_idle_bytes: number
  stack_inuse_bytes: number
  last_gc_time?: string
  sip_connections: number
  rtp_active_transfers: number
  rtp_port_pool_used: number
  rtp_port_pool_total: number
  active_requests: number
  configured_generic_download_mbps: number
  configured_generic_per_transfer_mbps: number
  configured_adaptive_hot_cache_mb: number
  configured_adaptive_hot_window_mb: number
  configured_generic_download_window_mb: number
  configured_generic_segment_concurrency: number
  configured_generic_rtp_reorder_window_packets: number
  configured_generic_rtp_loss_tolerance_packets: number
  configured_generic_rtp_gap_timeout_ms: number
  configured_generic_rtp_fec_enabled: boolean
  configured_generic_rtp_fec_group_packets: number
  status_color?: string
  status_summary?: string
  status_reasons?: string[]
  recommended_summary?: string
  recommended_profile?: string
  suggested_actions?: string[]
  suitable_scenarios?: string[]
  selfcheck_overall?: string
  rtp_port_pool_usage_percent?: number
  active_request_usage_percent?: number
  heap_usage_percent?: number
  theoretical_rtp_transfer_limit?: number
  recommended_file_transfer_max_concurrent?: number
  recommended_max_concurrent?: number
  recommended_rate_limit_rps?: number
  recommended_rate_limit_burst?: number
  observed_jitter_loss_events?: number
  observed_gap_timeouts?: number
  observed_fec_recovered?: number
  observed_peak_pending?: number
  observed_max_gap_hold_ms?: number
  observed_writer_block_ms?: number
  observed_max_writer_block_ms?: number
  observed_context_canceled?: number
  observed_circuit_open_count?: number
  observed_circuit_half_open_count?: number
  runtime_profile_applied?: string
  runtime_profile_applied_at?: string
  runtime_profile_reason?: string
  runtime_profile_changed?: boolean
}

export interface AlertProtectionState {
  alertRules: string[]
  rateLimitRules: string[]
  circuitBreakerRules: string[]
  currentTriggered: string[]
  lastTriggeredTime: string
  lastTriggeredTarget: string
  rps?: number
  burst?: number
  maxConcurrent?: number
  failureThreshold?: number
  recoveryWindowSec?: number
  rateLimitStatus?: string
  circuitBreakerStatus?: string
  protectionStatus?: string
  analysisWindow?: string
  recentFailureCount?: number
  recentSlowRequestCount?: number
  currentActiveRequests?: number
  rateLimitHitsTotal?: number
  concurrentRejectsTotal?: number
  allowedRequestsTotal?: number
  lastTriggeredType?: string
  circuitOpenCount?: number
  circuitHalfOpenCount?: number
  circuitActiveState?: string
  circuitLastOpenUntil?: string
  circuitLastOpenReason?: string
  circuitEntries?: CircuitEntrySnapshot[]
  topRateLimitTargets?: ProtectionTargetStat[]
  topConcurrentTargets?: ProtectionTargetStat[]
  topAllowedTargets?: ProtectionTargetStat[]
  scopes?: ProtectionScopeSnapshot[]
  restrictions?: ProtectionRestrictionSnapshot[]
}

export interface GatewayRestartResponse {
  accepted: boolean
  command: string
  scheduled_at: string
}

export interface LoadtestJob {
  job_id: string
  status: string
  created_at: string
  updated_at: string
  targets: string[]
  http_url: string
  sip_address: string
  rtp_address: string
  gateway_base_url: string
  concurrency: number
  qps: number
  duration_sec: number
  output_dir: string
  summary_file?: string
  report_file?: string
  error_message?: string
  capacity_suggestion?: Record<string, unknown>
}

export interface DiagnosticExportData {
  generated_at: string
  job_id: string
  node_id: string
  request_id?: string
  trace_id?: string
  file_name: string
  output_dir: string
  files: Array<{ name: string; description: string }>
}

export interface SystemSettingsState {
  sqlitePath: string
  logPath: string
  uiMode: UiDeployMode
  apiBaseUrl: string
  metricsEndpoint: string
  readyEndpoint: string
  selfCheckEndpoint: string
  startupSummaryEndpoint: string
  uiConsistencyStatus?: string
  uiConsistencyDetail?: string
  uiEmbedBuildNonce?: string
  uiEmbeddedAt?: string
  uiSourceLatestWrite?: string
  uiEmbeddedHash?: string
  uiAssetBaseMode?: string
  uiRouterBasePathPolicy?: string
  uiDeliveryGuardStatus?: string
  uiDeliveryGuardDetail?: string
  uiDeliveryGuardRemovedCount?: number
  uiDeliveryGuardRemainingCount?: number
  uiDeliveryGuardHitCount?: number
  entrySelectionPolicy?: string
  udpControlHeaderPolicy?: string
  genericDownloadRTPToleranceProfile?: string
  genericDownloadGuardPolicy?: string
  logRetentionDays: number
  logRetentionRecords: number
  auditRetentionDays: number
  auditRetentionRecords: number
  accessLogRetentionDays: number
  accessLogRetentionRecords: number
  diagnosticsRetentionDays: number
  diagnosticsRetentionRecords: number
  loadtestRetentionDays: number
  loadtestRetentionRecords: number
  cleanupCron: string
  adminCIDR: string
  mfaEnabled: boolean
  genericDownloadTotalMbps: number
  genericDownloadPerTransferMbps: number
  genericDownloadWindowMB: number
  adaptiveHotCacheMB: number
  adaptiveHotWindowMB: number
  genericDownloadSegmentConcurrency: number
  genericDownloadRTPReorderWindowPackets: number
  genericDownloadRTPLossTolerancePackets: number
  genericDownloadRTPGapTimeoutMS: number
  genericDownloadRTPFECEnabled: boolean
  genericDownloadRTPFECGroupPackets: number
  adminTokenConfigured?: boolean
  adminMfaConfigured?: boolean
  configEncryptionEnabled?: boolean
  tunnelSignerExternalized?: boolean
  lastCleanupStatus: string
  lastCleanupRemovedRecords: number
}

export interface SecurityStatePayload {
  license_status: string
  expiry_time: string
  active_time: string
  maintenance_expire_time: string
  license_time: string
  product_type: string
  product_type_name?: string
  license_type: string
  license_counter: string
  machine_code: string
  project_code: string
  licensed_features: string[]
  last_validation: string
  management_security: string
  signing_algorithm: string
  admin_token_configured?: boolean
  admin_mfa_required?: boolean
  admin_mfa_configured?: boolean
  config_encryption?: boolean
  signer_externalized?: boolean
  admin_token_fingerprint?: string
}

export interface LicensePayload {
  status: string
  expire_at: string
  active_at: string
  maintenance_expire_at: string
  license_time: string
  product_type: string
  product_type_name?: string
  license_type: string
  license_counter: string
  machine_code: string
  project_code: string
  region_info: string
  industry_info: string
  customer_info: string
  user_info: string
  server_info: string
  features: string[]
  last_verify_result: string
  summary1: string
  summary2: string
  raw_license_content?: string
}

export interface SecurityCenterState {
  licenseStatus: string
  expiryTime: string
  activeTime: string
  maintenanceExpireTime: string
  licenseTime: string
  productType: string
  productTypeName?: string
  licenseType: string
  licenseCounter: string
  machineCode: string
  projectCode: string
  licensedFeatures: string[]
  lastValidation: string
  managementSecurity: string
  signingAlgorithm: string
  adminTokenConfigured?: boolean
  adminMFARequired?: boolean
  adminMFAConfigured?: boolean
  configEncryption?: boolean
  signerExternalized?: boolean
  adminTokenFingerprint?: string
}

export interface MachineCodePayload {
  machine_code: string
  node_id: string
  hostname: string
  cpu_id: string
  board_serial: string
  mac_address: string
  request_file: string
}

export interface SecurityEventRecord {
  when: string
  category: string
  transport: string
  requestId: string
  traceId: string
  sessionId: string
  reason: string
  auditLinked?: boolean
}

export interface ProtectionCircuitRecoverResponse {
  removed: number
  target?: string
  state?: AlertProtectionState
}

export interface ProtectionRestrictionActionResponse {
  removed?: boolean
  item?: ProtectionRestrictionSnapshot
  state?: AlertProtectionState
}
