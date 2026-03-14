import { flushPromises, mount } from '@vue/test-utils'
import LocalNodeConfigView from '../LocalNodeConfigView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchNodeDetail: vi.fn(),
    fetchNodeNetworkStatus: vi.fn(),
    updateLocalNode: vi.fn()
  }
}))

vi.mock('ant-design-vue', () => ({ message: { success: vi.fn() } }))

const stubs = {
  'a-space': { template: '<div><slot /></div>' },
  'a-card': { template: '<section><slot /></section>' },
  'a-alert': { template: '<div><slot /></div>' },
  'a-descriptions': { template: '<div><slot /></div>' },
  'a-descriptions-item': { template: '<div><slot /></div>' },
  'a-form': { template: '<form><slot /></form>' },
  'a-row': { template: '<div><slot /></div>' },
  'a-col': { template: '<div><slot /></div>' },
  'a-form-item': { template: '<div><slot /></div>' },
  'a-input': { template: '<input />' },
  'a-input-number': { template: '<input />' },
  'a-radio-group': { template: '<div><slot /></div>' },
  'a-radio-button': { template: '<div><slot /></div>' },
  'a-divider': { template: '<div><slot /></div>' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' }
}

describe('LocalNodeConfigView', () => {
  it('loads node detail and network capability summary', async () => {
    vi.mocked(gatewayApi.fetchNodeDetail).mockResolvedValue({
      local_node: {
        node_id: 'gateway-a-01', node_name: 'Gateway A', node_role: 'gateway', network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
        sip_listen_ip: '0.0.0.0', sip_listen_port: 5060, sip_transport: 'UDP',
        rtp_listen_ip: '0.0.0.0', rtp_port_start: 20000, rtp_port_end: 20999, rtp_transport: 'UDP'
      },
      current_network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
      current_capability: { supports_large_request_body: false, supports_large_response_body: true, supports_streaming_response: false, supports_bidirectional_http_tunnel: false, supports_transparent_proxy: false },
      compatibility_status: { level: 'info', message: '', suggestion: '', action_hint: '' }
    })
    vi.mocked(gatewayApi.fetchNodeNetworkStatus).mockResolvedValue({
      network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
      capability: { supports_large_request_body: false, supports_large_response_body: true, supports_streaming_response: false, supports_bidirectional_http_tunnel: false, supports_transparent_proxy: false },
      current_network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
      current_capability: { supports_large_request_body: false, supports_large_response_body: true, supports_streaming_response: false, supports_bidirectional_http_tunnel: false, supports_transparent_proxy: false },
      compatibility_status: { level: 'info', message: '', suggestion: '', action_hint: '' },
      capability_summary: { supported: ['small_request'], unsupported: ['large_request'], items: [] }
    })

    const wrapper = mount(LocalNodeConfigView, { global: { stubs } })
    await flushPromises()

    expect(gatewayApi.fetchNodeDetail).toHaveBeenCalled()
    expect(gatewayApi.fetchNodeNetworkStatus).toHaveBeenCalled()
    expect(wrapper.text()).toContain('A_TO_B_SIP__B_TO_A_RTP')
    expect(wrapper.text()).toContain('small_request')
  })
})
