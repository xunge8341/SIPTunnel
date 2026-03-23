import { mount, flushPromises } from '@vue/test-utils'
import TunnelMappingsView from '../TunnelMappingsView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchTunnelMappingOverview: vi.fn(),
    fetchMappings: vi.fn(),
    fetchNodeTunnelWorkspace: vi.fn(),
    fetchTunnelCatalog: vi.fn(),
    testMapping: vi.fn(),
    createMapping: vi.fn(),
    updateMapping: vi.fn(),
    deleteMapping: vi.fn()
  }
}))

describe('TunnelMappingsView', () => {
  it('loads mappings and catalog exposure', async () => {
    vi.mocked(gatewayApi.fetchTunnelMappingOverview).mockResolvedValue({ items: [] } as any)
    vi.mocked(gatewayApi.fetchMappings).mockResolvedValue({ items: [] } as any)
    vi.mocked(gatewayApi.fetchNodeTunnelWorkspace).mockResolvedValue({ localNode: { rtp_port_start: 2000, rtp_port_end: 2001 }, networkMode: 'SENDER_SIP__RECEIVER_RTP' } as any)
    vi.mocked(gatewayApi.fetchTunnelCatalog).mockResolvedValue({ resources: [], summary: { resource_total: 0, manual_expose_num: 0, unexposed_num: 0 } } as any)
    const wrapper = mount(TunnelMappingsView)
    await flushPromises()
    expect(gatewayApi.fetchTunnelMappingOverview).toHaveBeenCalled()
    expect(gatewayApi.fetchMappings).toHaveBeenCalled()
    expect(gatewayApi.fetchTunnelCatalog).toHaveBeenCalled()
    expect(wrapper.text()).toContain('隧道映射')
  })
})
