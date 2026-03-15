import { mount, flushPromises } from '@vue/test-utils'
import SecurityCenterView from '../SecurityCenterView.vue'
import { gatewayApi } from '../../api/gateway'
vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchSecurityState: vi.fn() } }))

describe('SecurityCenterView', () => {
  it('loads security state', async () => {
    vi.mocked(gatewayApi.fetchSecurityState).mockResolvedValue({ licenseStatus: 'ok', expiryTime: 't', licensedFeatures: ['a'], lastValidation: 'ok', managementSecurity: 'cidr', signingAlgorithm: 'HMAC-SHA256' })
    const wrapper = mount(SecurityCenterView)
    await flushPromises()
    expect(wrapper.text()).toContain('HMAC-SHA256')
  })
})
