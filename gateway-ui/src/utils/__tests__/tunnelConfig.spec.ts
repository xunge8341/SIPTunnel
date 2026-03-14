import { buildTunnelCapabilityItems, deriveTunnelCapability } from '../tunnelConfig'

describe('tunnelConfig utils', () => {
  it('derives capability for classic SIP request + RTP response mode', () => {
    const capability = deriveTunnelCapability({ network_mode: 'SENDER_SIP__RECEIVER_RTP' })

    expect(capability.supports_small_request_body).toBe(true)
    expect(capability.supports_large_request_body).toBe(false)
    expect(capability.supports_large_response_body).toBe(true)
    expect(capability.supports_streaming_response).toBe(true)
  })

  it('derives bidirectional capability for full-duplex mode', () => {
    const capability = deriveTunnelCapability({ network_mode: 'SENDER_SIP_RTP__RECEIVER_SIP_RTP' })

    expect(capability.supports_large_request_body).toBe(true)
    expect(capability.supports_bidirectional_http_tunnel).toBe(true)
    expect(capability.supports_transparent_http_proxy).toBe(true)
  })

  it('builds capability items for display', () => {
    const items = buildTunnelCapabilityItems({
      supports_small_request_body: true,
      supports_large_request_body: false,
      supports_large_response_body: true,
      supports_streaming_response: true,
      supports_bidirectional_http_tunnel: false,
      supports_transparent_http_proxy: false
    })

    expect(items).toHaveLength(6)
    expect(items[0].key).toBe('supports_small_request_body')
  })
})
