import { beforeEach, describe, expect, it, vi } from 'vitest'

const requestMock = vi.fn()

vi.mock('../http', () => ({ request: requestMock }))

describe('gatewayApi tunnel config', () => {
  beforeEach(() => {
    vi.resetModules()
    requestMock.mockReset()
  })

  it('calls GET /tunnel/config in real mode', async () => {
    requestMock.mockResolvedValue({ data: { channel_protocol: 'GB/T 28181' } })
    const { gatewayApi } = await import('../gateway')
    await gatewayApi.fetchTunnelConfig()
    expect(requestMock).toHaveBeenCalledWith('/tunnel/config', { method: 'GET' })
  })

  it('calls POST /tunnel/config in real mode', async () => {
    requestMock.mockResolvedValue({ data: { channel_protocol: 'GB/T 28181' } })
    const { gatewayApi } = await import('../gateway')
    const payload = {
      channel_protocol: 'GB/T 28181',
      connection_initiator: 'LOCAL',
      local_device_id: '34020000001320000001',
      peer_device_id: '34020000002000000001',
      heartbeat_interval_sec: 60,
      register_retry_count: 3,
      register_retry_interval_sec: 10,
      registration_status: 'registered',
      last_register_time: '2026-03-14T10:00:00Z',
      last_heartbeat_time: '2026-03-14T10:00:30Z',
      heartbeat_status: 'healthy',
      supported_capabilities: ['支持小请求体（典型 SIP JSON 负载）'],
      request_channel: 'SIP',
      response_channel: 'RTP',
      network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
      capability: {
        supports_small_request_body: true,
        supports_large_request_body: false,
        supports_large_response_body: true,
        supports_streaming_response: true,
        supports_bidirectional_http_tunnel: false,
        supports_transparent_http_proxy: false
      },
      capability_items: []
    }
    await gatewayApi.saveTunnelConfig(payload)
    expect(requestMock).toHaveBeenCalledWith('/tunnel/config', { method: 'POST', body: payload })
  })
})
