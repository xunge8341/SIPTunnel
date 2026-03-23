import { mount, flushPromises } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'
import { gatewayApi } from '../../api/gateway'

const pushMock = vi.fn()

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: pushMock })
}))

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchDashboardSummary: vi.fn(),
    fetchDashboardOpsSummary: vi.fn(),
    fetchDashboardTrends: vi.fn()
  }
}))

describe('DashboardView', () => {
  beforeEach(() => {
    pushMock.mockReset()
  })

  it('loads summary and ops', async () => {
    vi.mocked(gatewayApi.fetchDashboardSummary).mockResolvedValue({ systemHealth: 'healthy', activeConnections: 2, mappingTotal: 3, mappingErrorCount: 1, recentFailureCount: 1, rateLimitState: 'normal', circuitBreakerState: 'closed' } as any)
    vi.mocked(gatewayApi.fetchDashboardOpsSummary).mockResolvedValue({ hotMappings: [{ name: 'm1', count: 2 }], topFailureMappings: [], hotSourceIPs: [], topFailureIPs: [] } as any)
    vi.mocked(gatewayApi.fetchDashboardTrends).mockResolvedValue({ range: '24h', granularity: '1h', points: [] } as any)

    const wrapper = mount(DashboardView)
    await flushPromises()

    expect(gatewayApi.fetchDashboardSummary).toHaveBeenCalled()
    expect(gatewayApi.fetchDashboardOpsSummary).toHaveBeenCalled()
    expect(gatewayApi.fetchDashboardTrends).toHaveBeenCalled()
    expect(wrapper.text()).toContain('系统健康')
    expect(wrapper.text()).toContain('快速操作')
  })
})
