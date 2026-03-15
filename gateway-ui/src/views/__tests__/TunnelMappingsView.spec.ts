import { mount, flushPromises } from '@vue/test-utils'
import TunnelMappingsView from '../TunnelMappingsView.vue'
import { gatewayApi } from '../../api/gateway'
vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchMappingWorkspaceList: vi.fn() } }))

describe('TunnelMappingsView', () => {
  it('loads mapping list', async () => {
    vi.mocked(gatewayApi.fetchMappingWorkspaceList).mockResolvedValue({ items: [] })
    mount(TunnelMappingsView)
    await flushPromises()
    expect(gatewayApi.fetchMappingWorkspaceList).toHaveBeenCalled()
  })
})
