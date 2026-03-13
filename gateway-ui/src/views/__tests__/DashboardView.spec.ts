import { mount, flushPromises } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchDashboard: vi.fn(),
    fetchDeploymentMode: vi.fn(),
    fetchStartupSummary: vi.fn()
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
        rateLimitHits: 3,
        sipProtocol: 'UDP',
        sipListenPort: 5060,
        rtpProtocol: 'UDP',
        rtpPortRange: '20000-20999',
        activeSessions: 20,
        activeTransfers: 8,
        currentConnections: 42,
        failedTasks1h: 2,
        transportErrors1h: 1,
        rateLimitHits1h: 3
      },
      recentTrends: [
        { time: '10:00', total: 10, success: 9, failed: 1 },
        { time: '10:05', total: 12, success: 12, failed: 0 }
      ]
    })
    vi.mocked(gatewayApi.fetchDeploymentMode).mockResolvedValue({
      uiMode: 'external',
      uiUrl: 'https://ops.example.com',
      apiUrl: 'https://api.example.com',
      configPath: '/etc/siptunnel/config.yaml',
      configSource: 'config-center'
    })

    vi.mocked(gatewayApi.fetchStartupSummary).mockResolvedValue({
      node_id: 'gateway-a-01',
      config_path: '/etc/siptunnel/config.yaml',
      config_source: 'config-center',
      ui_mode: 'external',
      ui_url: 'https://ops.example.com',
      api_url: 'https://api.example.com',
      business_execution: {
        state: 'protocol_only',
        route_count: 0,
        message: '协议层可启动，业务执行层未激活（未加载下游 HTTP 路由）',
        impact: '仅完成 SIP/RTP 协议交互，不会执行 A 网 HTTP 落地'
      },
      self_check_summary: {
        generated_at: '2026-03-12T10:00:00Z',
        overall: 'warn',
        info: 8,
        warn: 1,
        error: 0
      }
    })

    const wrapper = mount(DashboardView)
    await flushPromises()

    expect(gatewayApi.fetchDashboard).toHaveBeenCalledTimes(1)
    expect(gatewayApi.fetchDeploymentMode).toHaveBeenCalledTimes(1)
    expect(gatewayApi.fetchStartupSummary).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('成功率')
    expect(wrapper.text()).toContain('当前 SIP transport')
    expect(wrapper.text()).toContain('最近 1h transport error')
    expect(wrapper.text()).toContain('external mode')
    expect(wrapper.text()).toContain('config-center')
    expect(wrapper.text()).toContain('当前未加载业务路由')
    expect(wrapper.text()).toContain('不会执行 A 网 HTTP 落地')
    expect(wrapper.findAll('circle')).toHaveLength(2)
  })
})
