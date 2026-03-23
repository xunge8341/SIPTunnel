import { mount, flushPromises } from '@vue/test-utils'
import SecurityCenterView from '../SecurityCenterView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchSecurityState: vi.fn(),
    fetchMachineCode: vi.fn(),
    updateLicense: vi.fn()
  }
}))

describe('SecurityCenterView', () => {
  it('loads security state', async () => {
    vi.mocked(gatewayApi.fetchSecurityState).mockResolvedValue({
      licenseStatus: '已授权',
      expiryTime: '2026-09-13',
      activeTime: '2026-03-13',
      maintenanceExpireTime: '2026-09-12',
      licenseTime: '2026-03-13',
      productType: '6',
      productTypeName: 'SIP隧道网关',
      licenseType: '1',
      licenseCounter: '1',
      machineCode: '184641235',
      projectCode: '项目A',
      licensedFeatures: ['AES', 'SM4'],
      lastValidation: '校验通过'
    } as any)
    vi.mocked(gatewayApi.fetchMachineCode).mockResolvedValue({
      machine_code: '184641235',
      node_id: 'NODE-001',
      hostname: 'demo-host',
      cpu_id: 'CPU-DEFAULT',
      board_serial: 'BOARD-DEFAULT',
      mac_address: 'MAC-DEFAULT',
      request_file: '[License Info]\nMachine Code=184641235\n'
    })
    const wrapper = mount(SecurityCenterView)
    await flushPromises()
    expect(wrapper.text()).toContain('授权管理')
    expect(wrapper.text()).toContain('项目编码')
    expect(wrapper.text()).toContain('184641235')
  })
})
