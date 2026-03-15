import { mount, flushPromises } from '@vue/test-utils'
import SystemSettingsView from '../SystemSettingsView.vue'
import { gatewayApi } from '../../api/gateway'
vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchSystemSettings: vi.fn(), updateSystemSettings: vi.fn() } }))

describe('SystemSettingsView', () => {
  it('loads settings', async () => {
    const state = { sqlitePath: 'a.db', logPath: '/log', logRetentionDays: 1, auditRetentionDays: 1, accessLogRetentionDays: 1, diagnosticsRetentionDays: 1, loadtestRetentionDays: 1, cleanupCron: '* * * * *', adminCIDR: '127.0.0.1/32', mfaEnabled: false, lastCleanupStatus: 'ok' }
    vi.mocked(gatewayApi.fetchSystemSettings).mockResolvedValue(state)
    mount(SystemSettingsView)
    await flushPromises()
    expect(gatewayApi.fetchSystemSettings).toHaveBeenCalled()
  })
})
