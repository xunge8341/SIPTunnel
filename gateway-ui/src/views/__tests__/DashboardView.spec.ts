import { mount, flushPromises } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchDashboard: vi.fn()
  }
}))

describe('DashboardView', () => {
  it('renders metric cards and trend chart after loading data', async () => {
    vi.mocked(gatewayApi.fetchDashboard).mockResolvedValue({
      metrics: {
        successRate: 99.8,
        failureRate: 0.2,
        concurrency: 8,
        rtpLossRate: 0.01,
        rateLimitHits: 3
      },
      recentTrends: [
        { time: '10:00', total: 10, success: 9, failed: 1 },
        { time: '10:05', total: 12, success: 12, failed: 0 }
      ]
    })

    const wrapper = mount(DashboardView)
    await flushPromises()

    expect(gatewayApi.fetchDashboard).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('成功率')
    expect(wrapper.findAll('circle')).toHaveLength(2)
  })
})
