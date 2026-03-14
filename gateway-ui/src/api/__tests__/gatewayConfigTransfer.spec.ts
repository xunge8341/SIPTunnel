import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ConfigTransferPayload } from '../../types/gateway'

const requestMock = vi.fn()

vi.mock('../http', () => ({ request: requestMock }))

describe('gatewayApi config transfer', () => {
  beforeEach(() => {
    vi.resetModules()
    requestMock.mockReset()
  })

  it('calls GET /config/transfer/export in real mode', async () => {
    requestMock.mockResolvedValue({ data: { version: 'v1' } })
    const { gatewayApi } = await import('../gateway')
    await gatewayApi.exportConfigJson()
    expect(requestMock).toHaveBeenCalledWith('/config/transfer/export', { method: 'GET' })
  })

  it('calls POST /config/transfer/import in real mode', async () => {
    requestMock.mockResolvedValue({ data: { imported: true } })
    const { gatewayApi } = await import('../gateway')
    const payload: ConfigTransferPayload = {
      version: 'v1',
      exported_at: new Date().toISOString(),
      network_config: {
        sip: {
          listenIp: '0.0.0.0',
          listenPort: 5060,
          protocol: 'UDP',
          advertisedAddress: '',
          domain: 'local.test',
          tcpKeepaliveEnabled: true,
          tcpKeepaliveIntervalMs: 30000,
          tcpReadBufferBytes: 1048576,
          tcpWriteBufferBytes: 1048576,
          maxConnections: 100
        },
        rtp: {
          listenIp: '0.0.0.0',
          portRangeStart: 20000,
          portRangeEnd: 20999,
          protocol: 'UDP',
          advertisedAddress: '',
          maxConcurrentTransfers: 500
        }
      },
      tunnel_config: {
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
      },
      node_config: {
        local_node: {
          node_ip: '10.0.0.1',
          signaling_port: 5060,
          device_id: 'node-a',
          rtp_port_start: 20000,
          rtp_port_end: 20999
        },
        peer_node: {
          node_ip: '10.0.0.2',
          signaling_port: 5060,
          device_id: 'node-b'
        }
      }
    }
    await gatewayApi.importConfigJson(payload)
    expect(requestMock).toHaveBeenCalledWith('/config/transfer/import', { method: 'POST', body: payload })
  })

  it('calls GET /config/transfer/template in real mode', async () => {
    requestMock.mockResolvedValue({ data: { version: 'template-v1' } })
    const { gatewayApi } = await import('../gateway')
    await gatewayApi.downloadConfigTemplate()
    expect(requestMock).toHaveBeenCalledWith('/config/transfer/template', { method: 'GET' })
  })
})
