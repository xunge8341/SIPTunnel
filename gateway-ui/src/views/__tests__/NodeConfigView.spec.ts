import { mount, flushPromises } from '@vue/test-utils'
import NodesAndTunnelsView from '../NodesAndTunnelsView.vue'
import { gatewayApi } from '../../api/gateway'
vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchNodeTunnelWorkspace: vi.fn(), saveNodeTunnelWorkspace: vi.fn() } }))

describe('NodesAndTunnelsView', () => {
  it('loads workspace', async () => {
    const ws = { localNode: { node_ip: '1.1.1.1', signaling_port: 5060, device_id: 'a', rtp_port_start: 2000, rtp_port_end: 2001 }, peerNode: { node_ip: '2.2.2.2', signaling_port: 5060, device_id: 'b', rtp_port_start: 2000, rtp_port_end: 2001 }, networkMode: 'm', capabilityMatrix: [], sipCapability: {}, rtpCapability: {}, sessionSettings: { channel_protocol: 'SIP', connection_initiator: 'LOCAL', local_device_id: 'a', peer_device_id: 'b', heartbeat_interval_sec: 10, register_retry_count: 1, register_retry_interval_sec: 1, registration_status: '', last_register_time: '', last_heartbeat_time: '', heartbeat_status: '', last_failure_reason: '', next_retry_time: '', consecutive_heartbeat_timeout: 0, supported_capabilities: [], request_channel: '', response_channel: '', network_mode: 'x', capability: { supports_large_request_body: false, supports_large_response_body: false, supports_streaming_response: false, supports_bidirectional_http_tunnel: false, supports_transparent_proxy: false }, capability_items: [] }, securitySettings: { signer: 'HMAC-SHA256', encryption: 'AES', verify_interval_min: 30 }, encryptionSettings: { algorithm: 'AES' } }
    vi.mocked(gatewayApi.fetchNodeTunnelWorkspace).mockResolvedValue(ws as any)
    mount(NodesAndTunnelsView)
    await flushPromises()
    expect(gatewayApi.fetchNodeTunnelWorkspace).toHaveBeenCalled()
  })
})
