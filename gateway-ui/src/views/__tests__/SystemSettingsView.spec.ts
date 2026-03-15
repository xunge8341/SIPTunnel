import { mount, flushPromises } from '@vue/test-utils'
import SystemSettingsView from '../SystemSettingsView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchSystemSettings: vi.fn(), updateSystemSettings: vi.fn() } }))

describe('SystemSettingsView', () => {
  const state = {
    sqlitePath: 'a.db',
    logPath: '/log',
    logRetentionDays: 1,
    auditRetentionDays: 1,
    accessLogRetentionDays: 1,
    diagnosticsRetentionDays: 1,
    loadtestRetentionDays: 1,
    cleanupCron: '* * * * *',
    adminCIDR: '127.0.0.1/32',
    mfaEnabled: false,
    lastCleanupStatus: 'ok'
  }

  it('loads settings', async () => {
    vi.mocked(gatewayApi.fetchSystemSettings).mockResolvedValue(state)
    mount(SystemSettingsView)
    await flushPromises()
    expect(gatewayApi.fetchSystemSettings).toHaveBeenCalled()
  })

  it('saves and reloads settings', async () => {
    vi.mocked(gatewayApi.fetchSystemSettings).mockResolvedValue(state)
    vi.mocked(gatewayApi.updateSystemSettings).mockResolvedValue(state)
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    const buttons = wrapper.findAll('button')
    await buttons[1].trigger('click')
    await flushPromises()

    expect(gatewayApi.updateSystemSettings).toHaveBeenCalledTimes(1)
    expect(gatewayApi.fetchSystemSettings.mock.calls.length).toBeGreaterThanOrEqual(2)
  })
})
