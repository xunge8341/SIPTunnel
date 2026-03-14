import { mount, flushPromises } from '@vue/test-utils'
import DashboardView from '../DashboardView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchDashboard: vi.fn(),
    fetchDeploymentMode: vi.fn(),
    fetchStartupSummary: vi.fn(),
    fetchSystemStatus: vi.fn()
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
      network_mode: 'SENDER_SIP__RECEIVER_RTP',
      capability: {
        supports_large_request_body: false,
        supports_large_response_body: true,
        supports_streaming_response: false,
        supports_bidirectional_http_tunnel: false,
        supports_transparent_proxy: false
      },
      capability_summary: {
        supported: ['small_request', 'large_response'],
        unsupported: ['large_request'],
        items: []
      },
      config_path: '/etc/siptunnel/config.yaml',
      config_source: 'config-center',
      ui_mode: 'external',
      ui_url: 'https://ops.example.com',
      api_url: 'https://api.example.com',
      transport_plan: {
        request_meta_transport: 'sip_control',
        request_body_transport: 'sip_body_only',
        response_meta_transport: 'sip_control',
        response_body_transport: 'rtp_stream',
        request_body_size_limit: 65535,
        response_body_size_limit: -1,
        notes: ['transport 决策由全局网络模式推导，禁止在单条隧道映射上覆盖。'],
        warnings: ['不支持大请求体上传；超过 SIP 限制的请求体将被拒绝。']
      },
      business_execution: {
        state: 'protocol_only',
        route_count: 0,
        message: '协议层可启动，业务执行层未激活（未加载下游 HTTP 隧道映射）',
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


    vi.mocked(gatewayApi.fetchSystemStatus).mockResolvedValue({
      tunnel_status: 'connected',
      connection_reason: 'SIP 控制面与 RTP 文件面链路正常',
      network_mode: 'SENDER_SIP__RECEIVER_RTP',
      registration_status: 'registered',
      heartbeat_status: 'healthy',
      last_register_time: '2026-03-14T10:00:00Z',
      last_heartbeat_time: '2026-03-14T10:00:30Z',
      mapping_total: 5,
      mapping_abnormal_total: 2,
      latest_mapping_error_reason: 'map-query：对端不可达',
      capability: {
        supports_small_request_body: true,
        supports_large_response_body: true,
        supports_streaming_response: true,
        supports_large_file_upload: false,
        supports_bidirectional_http_tunnel: false
      }
    })

    const wrapper = mount(DashboardView)
    await flushPromises()

    expect(gatewayApi.fetchDashboard).toHaveBeenCalledTimes(1)
    expect(gatewayApi.fetchDeploymentMode).toHaveBeenCalledTimes(1)
    expect(gatewayApi.fetchStartupSummary).toHaveBeenCalledTimes(1)
    expect(gatewayApi.fetchSystemStatus).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('成功率')
    expect(wrapper.text()).toContain('当前 SIP 协议')
    expect(wrapper.text()).toContain('最近 1 小时传输异常')
    expect(wrapper.text()).toContain('独立式')
    expect(wrapper.text()).toContain('config-center')
    expect(wrapper.text()).toContain('当前未加载 HTTP 隧道映射')
    expect(wrapper.text()).toContain('不会执行 A 网 HTTP 落地')
    expect(wrapper.text()).toContain('sip_body_only')
    expect(wrapper.text()).toContain('rtp_stream')
    expect(wrapper.text()).toContain('隧道链路状态')
    expect(wrapper.text()).toContain('注册状态')
    expect(wrapper.text()).toContain('心跳状态')
    expect(wrapper.text()).toContain('最近注册时间')
    expect(wrapper.text()).toContain('隧道映射总数 / 异常数')
    expect(wrapper.text()).toContain('能力矩阵')
    expect(wrapper.findAll('circle')).toHaveLength(2)
  })
})
