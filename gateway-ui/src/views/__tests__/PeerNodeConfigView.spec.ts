import { flushPromises, mount } from '@vue/test-utils'
import PeerNodeConfigView from '../PeerNodeConfigView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchPeers: vi.fn(),
    fetchNodeNetworkStatus: vi.fn(),
    createPeer: vi.fn(),
    updatePeer: vi.fn(),
    deletePeer: vi.fn()
  }
}))

vi.mock('ant-design-vue', () => ({ message: { success: vi.fn() } }))

const stubs = {
  'a-space': { template: '<div><slot /></div>' },
  'a-card': { template: '<section><slot /></section>' },
  'a-alert': { template: '<div><slot /></div>' },
  'a-descriptions': { template: '<div><slot /></div>' },
  'a-descriptions-item': { template: '<div><slot /></div>' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' },
  'a-table': { template: '<div><slot /></div>' },
  'a-table-column': { template: '<div></div>' },
  'a-switch': { template: '<input type="checkbox" />' },
  'a-popconfirm': { template: '<div><slot /></div>' },
  'a-drawer': { template: '<div><slot /><slot name="footer" /></div>' },
  'a-form': { template: '<form><slot /></form>' },
  'a-row': { template: '<div class="stub-row"><slot /></div>' },
  'a-col': { template: '<div class="stub-col"><slot /></div>' },
  'a-form-item': { template: '<div><slot /></div>' },
  'a-input': { template: '<input />' },
  'a-input-number': { template: '<input />' }
}

describe('PeerNodeConfigView', () => {
  it('loads peers and capability summary', async () => {
    vi.mocked(gatewayApi.fetchPeers).mockResolvedValue({
      items: [
        {
          peer_node_id: 'peer-1', peer_name: 'Peer 1', peer_signaling_ip: '10.0.0.1', peer_signaling_port: 5060,
          peer_media_ip: '10.0.0.2', peer_media_port_start: 32000, peer_media_port_end: 32100,
          supported_network_mode: 'SENDER_SIP__RECEIVER_RTP', enabled: true
        }
      ]
    })
    vi.mocked(gatewayApi.fetchNodeNetworkStatus).mockResolvedValue({
      network_mode: 'SENDER_SIP__RECEIVER_RTP',
      capability: { supports_large_request_body: false, supports_large_response_body: true, supports_streaming_response: false, supports_bidirectional_http_tunnel: false, supports_transparent_proxy: false },
      current_network_mode: 'SENDER_SIP__RECEIVER_RTP',
      current_capability: { supports_large_request_body: false, supports_large_response_body: true, supports_streaming_response: false, supports_bidirectional_http_tunnel: false, supports_transparent_proxy: false },
      compatibility_status: { level: 'info', message: '', suggestion: '', action_hint: '' },
      capability_summary: { supported: ['small_request'], unsupported: ['large_request'], items: [] }
    })

    const wrapper = mount(PeerNodeConfigView, { global: { stubs } })
    await flushPromises()

    expect(gatewayApi.fetchPeers).toHaveBeenCalled()
    expect(gatewayApi.fetchNodeNetworkStatus).toHaveBeenCalled()
    expect(wrapper.text()).toContain('SENDER_SIP__RECEIVER_RTP')
    expect(wrapper.text()).toContain('small_request')
    expect(wrapper.findAll('.stub-row')).toHaveLength(0)
  })
})
