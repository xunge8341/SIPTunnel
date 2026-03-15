import { flushPromises, mount } from '@vue/test-utils'
import SecurityCenterView from '../SecurityCenterView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchLicense: vi.fn(), fetchSecuritySettings: vi.fn(), updateSecuritySettings: vi.fn(), updateLicense: vi.fn() } }))
vi.mock('ant-design-vue', () => ({ message: { success: vi.fn() } }))

const stubs = {
  'a-space': { template: '<div><slot /></div>' },
  'a-card': { template: '<section><slot /></section>' },
  'a-descriptions': { template: '<div><slot /></div>' },
  'a-descriptions-item': { template: '<div><slot /></div>' },
  'a-input': { template: '<input />' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' },
  'a-form': { template: '<form><slot /></form>' },
  'a-form-item': { template: '<div><slot /></div>' },
  'a-select': { template: '<div><slot /></div>' },
  'a-select-option': { template: '<div><slot /></div>' },
  'a-radio-group': { template: '<div><slot /></div>' },
  'a-radio': { template: '<div><slot /></div>' },
  'a-input-number': { template: '<input />' }
}

describe('SecurityCenterView', () => {
  it('reloads settings after save', async () => {
    vi.mocked(gatewayApi.fetchLicense).mockResolvedValue({ status: '-', expire_at: '-', features: [], last_verify_result: '-' })
    vi.mocked(gatewayApi.fetchSecuritySettings).mockResolvedValue({ signer: 'HMAC-SHA256', encryption: 'AES', verify_interval_min: 30 })
    vi.mocked(gatewayApi.updateSecuritySettings).mockResolvedValue({ signer: 'HMAC-SHA256', encryption: 'SM4', verify_interval_min: 30 })
    const wrapper = mount(SecurityCenterView, { global: { stubs } })
    await flushPromises()
    await wrapper.findAll('button')[1].trigger('click')
    await flushPromises()
    expect(gatewayApi.updateSecuritySettings).toHaveBeenCalled()
    expect(gatewayApi.fetchSecuritySettings).toHaveBeenCalledTimes(3)
  })
})
