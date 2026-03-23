import { mount, flushPromises } from '@vue/test-utils'
import NodesAndTunnelsView from '../NodesAndTunnelsView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchNodeTunnelWorkspace: vi.fn(),
    saveNodeTunnelWorkspace: vi.fn(),
    fetchSystemSettings: vi.fn(),
    updateSystemSettings: vi.fn(),
    triggerTunnelSessionAction: vi.fn()
  }
}))

describe('NodesAndTunnelsView', () => {
  it('loads workspace and renders node/tunnel basics', async () => {
    const ws = {
      localNode: { node_ip: '1.1.1.1', signaling_port: 5060, device_id: 'a', rtp_port_start: 2000, rtp_port_end: 2001, mapping_port_start: 18100, mapping_port_end: 18110 },
      peerNode: { node_ip: '2.2.2.2', signaling_port: 5061, device_id: 'b', rtp_port_start: 3000, rtp_port_end: 3001 },
      networkMode: 'SENDER_SIP__RECEIVER_RTP',
      capabilityMatrix: [],
      sipCapability: { transport: 'TCP' },
      rtpCapability: { transport: 'UDP' },
      sessionSettings: {
        channel_protocol: 'GB/T 28181', connection_initiator: 'LOCAL', mapping_relay_mode: 'AUTO', local_device_id: 'a', peer_device_id: 'b',
        heartbeat_interval_sec: 10, register_retry_count: 1, register_retry_interval_sec: 1, registration_status: '', last_register_time: '', last_heartbeat_time: '', heartbeat_status: '', last_failure_reason: '', next_retry_time: '', consecutive_heartbeat_timeout: 0, supported_capabilities: [], request_channel: 'SIP', response_channel: 'RTP', network_mode: 'SENDER_SIP__RECEIVER_RTP', capability: { supports_small_request_body: true, supports_large_request_body: false, supports_large_response_body: true, supports_streaming_response: true, supports_bidirectional_http_tunnel: false, supports_transparent_http_proxy: false }, capability_items: [], register_auth_enabled: false, register_auth_username: '', register_auth_password: '', register_auth_password_configured: false, register_auth_realm: '', register_auth_algorithm: 'MD5', catalog_subscribe_expires_sec: 3600
      },
      securitySettings: { signer: 'HMAC-SHA256', encryption: 'AES', verify_interval_min: 30 },
      encryptionSettings: { algorithm: 'AES' }
    }
    vi.mocked(gatewayApi.fetchNodeTunnelWorkspace).mockResolvedValue(ws as any)
    vi.mocked(gatewayApi.fetchSystemSettings).mockResolvedValue({ adminCIDR: '', mfaEnabled: false } as any)
    const wrapper = mount(NodesAndTunnelsView)
    await flushPromises()
    expect(gatewayApi.fetchNodeTunnelWorkspace).toHaveBeenCalled()
    expect(wrapper.text()).toContain('节点与级联')
    expect(wrapper.text()).toContain('本级域编码（20 位国标编码）')
    expect(wrapper.text()).toContain('级联对端编码（20 位国标编码）')
  })
})
