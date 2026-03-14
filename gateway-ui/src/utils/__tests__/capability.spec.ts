import { buildCapabilityMatrix, evaluateMappingCapability } from '../capability'
import type { StartupSummaryPayload, TunnelMapping } from '../../types/gateway'

const startupSummary: StartupSummaryPayload = {
  node_id: 'node-1',
  network_mode: 'SENDER_SIP__RECEIVER_RTP',
  capability: {
    supports_large_request_body: false,
    supports_large_response_body: false,
    supports_streaming_response: false,
    supports_bidirectional_http_tunnel: false,
    supports_transparent_proxy: false
  },
  capability_summary: {
    supported: ['small_request'],
    unsupported: ['large_request', 'large_response', 'streaming_response', 'bidirectional_http_tunnel'],
    items: []
  },
  config_path: '',
  config_source: '',
  ui_mode: 'embedded',
  ui_url: '',
  api_url: '',
  transport_plan: {
    request_meta_transport: 'sip_control',
    request_body_transport: 'sip_body_only',
    response_meta_transport: 'sip_control',
    response_body_transport: 'sip_body_only',
    request_body_size_limit: 1024,
    response_body_size_limit: 1024,
    notes: [],
    warnings: []
  },
  business_execution: {
    state: 'active',
    route_count: 1,
    message: '',
    impact: ''
  },
  self_check_summary: {
    generated_at: '',
    overall: 'info',
    info: 1,
    warn: 0,
    error: 0
  }
}

const mapping: TunnelMapping = {
  mapping_id: 'm1',
  name: 'm1',
  enabled: true,
  peer_node_id: 'peer',
  local_bind_ip: '127.0.0.1',
  local_bind_port: 8080,
  local_base_path: '/',
  remote_target_ip: '127.0.0.1',
  remote_target_port: 9000,
  remote_base_path: '/',
  allowed_methods: ['POST'],
  connect_timeout_ms: 500,
  request_timeout_ms: 1000,
  response_timeout_ms: 1000,
  max_request_body_bytes: 1024,
  max_response_body_bytes: 1024,
  require_streaming_response: false,
  description: ''
}

describe('capability utils', () => {
  it('builds default matrix with operation-friendly labels', () => {
    const matrix = buildCapabilityMatrix(startupSummary.capability)
    expect(matrix.find((item) => item.key === 'small_request')?.label).toContain('小请求体')
    expect(matrix.find((item) => item.key === 'bidirectional_http_tunnel')?.label).toContain('透明 HTTP tunnel')
  })

  it('returns blocking issues and warnings for unsupported mapping config', () => {
    const result = evaluateMappingCapability(
      {
        ...mapping,
        max_request_body_bytes: 2048,
        max_response_body_bytes: 4096,
        require_streaming_response: true,
        allowed_methods: ['CONNECT', 'DELETE']
      },
      startupSummary
    )

    expect(result.blockingIssues).toHaveLength(4)
    expect(result.blockingIssues.join(' ')).toContain('max_request_body_bytes')
    expect(result.blockingIssues.join(' ')).toContain('流式响应')
    expect(result.advisoryWarnings.join(' ')).toContain('PUT/PATCH/DELETE')
  })
})
