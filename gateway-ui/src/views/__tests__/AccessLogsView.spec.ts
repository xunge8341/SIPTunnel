import { flushPromises, mount } from '@vue/test-utils'
import AccessLogsView from '../AccessLogsView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchAccessLogs: vi.fn() } }))

const stubs = {
  'a-space': { template: '<div><slot /></div>' },
  'a-card': { template: '<section><slot /></section>' },
  'a-form': { template: '<form><slot /></form>' },
  'a-form-item': { template: '<div><slot /></div>' },
  'a-select': { template: '<div><slot /></div>' },
  'a-select-option': { template: '<div><slot /></div>' },
  'a-input': { template: '<input />' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' },
  'a-table': { template: '<div />' },
  'a-tag': { template: '<span><slot /></span>' },
  'a-drawer': { template: '<div><slot /></div>' },
  'a-descriptions': { template: '<div><slot /></div>' },
  'a-descriptions-item': { template: '<div><slot /></div>' }
}

describe('AccessLogsView', () => {
  it('loads access logs via real access log API', async () => {
    vi.mocked(gatewayApi.fetchAccessLogs).mockResolvedValue({ list: [], total: 0, page: 1, pageSize: 100 })
    mount(AccessLogsView, { global: { stubs } })
    await flushPromises()
    expect(gatewayApi.fetchAccessLogs).toHaveBeenCalledTimes(1)
  })
})
