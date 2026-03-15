import { flushPromises, mount } from '@vue/test-utils'
import SystemSettingsView from '../SystemSettingsView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchSystemSettings: vi.fn(), updateSystemSettings: vi.fn() } }))
vi.mock('ant-design-vue', () => ({ message: { success: vi.fn() } }))

const stubs = {
  'a-space': { template: '<div><slot /></div>' },
  'a-card': { template: '<section><slot /></section>' },
  'a-typography-paragraph': { template: '<p><slot /></p>' },
  'a-form': { template: '<form><slot /></form>' },
  'a-divider': { template: '<div><slot /></div>' },
  'a-form-item': { template: '<div><slot /></div>' },
  'a-input': { template: '<input />' },
  'a-row': { template: '<div><slot /></div>' },
  'a-col': { template: '<div><slot /></div>' },
  'a-input-number': { template: '<input />' },
  'a-switch': { template: '<input type="checkbox" />' },
  'a-alert': { template: '<div><slot /></div>' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' }
}

describe('SystemSettingsView', () => {
  it('saves and reloads settings', async () => {
    const payload = { sqlite_path: 'a', log_cleanup_cron: '*', max_task_age_days: 1, max_task_records: 1, max_access_log_age_days: 1, max_access_log_records: 1, max_audit_age_days: 1, max_audit_records: 1, max_diagnostic_age_days: 1, max_diagnostic_records: 1, max_loadtest_age_days: 1, max_loadtest_records: 1, admin_allow_cidr: '127', admin_require_mfa: false, cleaner_last_run_at: '', cleaner_last_result: '', cleaner_last_removed_records: 0 }
    vi.mocked(gatewayApi.fetchSystemSettings).mockResolvedValue(payload)
    vi.mocked(gatewayApi.updateSystemSettings).mockResolvedValue(payload)
    const wrapper = mount(SystemSettingsView, { global: { stubs } })
    await flushPromises()
    await wrapper.findAll('button')[1].trigger('click')
    await flushPromises()
    expect(gatewayApi.updateSystemSettings).toHaveBeenCalled()
    expect(gatewayApi.fetchSystemSettings).toHaveBeenCalledTimes(3)
  })
})
