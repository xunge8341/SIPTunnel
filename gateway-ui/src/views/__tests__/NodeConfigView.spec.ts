import { flushPromises, mount } from '@vue/test-utils'
import NodeConfigView from '../NodeConfigView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchNodeConfig: vi.fn(),
    saveNodeConfig: vi.fn()
  }
}))

vi.mock('ant-design-vue', () => ({ message: { success: vi.fn() } }))

const stubs = {
  'a-card': { template: '<section><slot /></section>' },
  'a-form': { template: '<form><slot /></form>' },
  'a-divider': { template: '<div><slot /></div>' },
  'a-row': { template: '<div class="stub-row"><slot /></div>' },
  'a-col': { template: '<div class="stub-col"><slot /></div>' },
  'a-form-item': { template: '<div><slot /></div>' },
  'a-input': { template: '<input />' },
  'a-input-number': { template: '<input />' },
  'a-space': { template: '<div><slot /></div>' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' }
}

describe('NodeConfigView', () => {
  it('loads and saves node config', async () => {
    vi.mocked(gatewayApi.fetchNodeConfig).mockResolvedValue({
      local_node: { node_ip: '10.0.0.1', signaling_port: 5060, device_id: 'gw-a', rtp_port_start: 20000, rtp_port_end: 20999 },
      peer_node: { node_ip: '10.0.0.2', signaling_port: 5060, device_id: 'gw-b' }
    })
    vi.mocked(gatewayApi.saveNodeConfig).mockResolvedValue({
      config: {
        local_node: { node_ip: '10.0.0.1', signaling_port: 5060, device_id: 'gw-a', rtp_port_start: 20000, rtp_port_end: 20999 },
        peer_node: { node_ip: '10.0.0.2', signaling_port: 5060, device_id: 'gw-b' }
      },
      tunnel_restarted: true
    })

    const wrapper = mount(NodeConfigView, { global: { stubs } })
    await flushPromises()

    expect(gatewayApi.fetchNodeConfig).toHaveBeenCalledTimes(1)
    await wrapper.findAll('button')[1].trigger('click')
    await flushPromises()
    expect(gatewayApi.saveNodeConfig).toHaveBeenCalled()
    expect(wrapper.findAll('.stub-row')).toHaveLength(0)
  })
})
