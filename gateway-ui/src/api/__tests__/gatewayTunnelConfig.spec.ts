import { beforeEach, describe, expect, it, vi } from 'vitest'

const requestMock = vi.fn()

vi.mock('../http', () => ({ request: requestMock }))

describe('gatewayApi tunnel config', () => {
  beforeEach(() => {
    vi.resetModules()
    requestMock.mockReset()
  })

  it('calls GET /tunnel/config in real mode', async () => {
    requestMock.mockResolvedValue({ data: { channel_protocol: 'GB28181' } })
    const { gatewayApi } = await import('../gateway')
    await gatewayApi.fetchTunnelConfig()
    expect(requestMock).toHaveBeenCalledWith('/tunnel/config', { method: 'GET' })
  })

  it('calls POST /tunnel/config in real mode', async () => {
    requestMock.mockResolvedValue({ data: { channel_protocol: 'GB28181' } })
    const { gatewayApi } = await import('../gateway')
    const payload = {
      channel_protocol: 'GB28181',
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
