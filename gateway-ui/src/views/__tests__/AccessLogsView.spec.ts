import { mount, flushPromises } from '@vue/test-utils'
import AccessLogsView from '../AccessLogsView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchAccessLogs: vi.fn() } }))

describe('AccessLogsView', () => {
  it('loads logs on mount and supports refresh action', async () => {
    vi.mocked(gatewayApi.fetchAccessLogs).mockResolvedValue({ list: [], total: 0, page: 1, pageSize: 50 })
    const wrapper = mount(AccessLogsView)
    await flushPromises()
    expect(gatewayApi.fetchAccessLogs).toHaveBeenCalledTimes(1)

    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(gatewayApi.fetchAccessLogs).toHaveBeenCalledTimes(2)
  })})
