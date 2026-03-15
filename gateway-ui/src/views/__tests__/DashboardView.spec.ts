import { mount, flushPromises } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchDashboardSummary: vi.fn(), fetchDashboardOpsSummary: vi.fn() } }))

describe('DashboardView', () => {
  it('loads summary and ops', async () => {
    vi.mocked(gatewayApi.fetchDashboardSummary).mockResolvedValue({ systemHealth: 'healthy', activeConnections: 2, mappingTotal: 3, mappingErrorCount: 1, recentFailureCount: 1, rateLimitState: 'normal', circuitBreakerState: 'closed' })
    vi.mocked(gatewayApi.fetchDashboardOpsSummary).mockResolvedValue({ hotMappings: [{ name: 'm1', count: 2 }], topFailureMappings: [], hotSourceIPs: [], topFailureIPs: [] })
    const wrapper = mount(DashboardView)
    await flushPromises()
    expect(wrapper.text()).toContain('systemHealth')
    expect(gatewayApi.fetchDashboardSummary).toHaveBeenCalled()
  })
})
