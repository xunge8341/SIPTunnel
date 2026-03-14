import { flushPromises, mount } from '@vue/test-utils'
import TunnelMappingsView from '../TunnelMappingsView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchMappings: vi.fn(),
    createMapping: vi.fn(),
    updateMapping: vi.fn(),
    deleteMapping: vi.fn(),
    fetchStartupSummary: vi.fn(),
    testMapping: vi.fn(),
    fetchSystemStatus: vi.fn()
  }
}))

vi.mock('ant-design-vue', () => ({
  message: { success: vi.fn(), warning: vi.fn(), error: vi.fn() }
}))

const stubs = {
  'a-space': { template: '<div><slot /></div>' },
  'a-card': { template: '<section><slot /></section>' },
  'a-form': { template: '<form><slot /></form>' },
  'a-form-item': { props: ['label', 'extra'], template: '<div class="stub-form-item"><label v-if="label">{{ label }}</label><small v-if="extra">{{ extra }}</small><slot /></div>' },
  'a-input': { template: '<input />' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' },
  'a-descriptions': { template: '<div><slot /></div>' },
  'a-descriptions-item': { template: '<div><slot /></div>' },
  'a-alert': { props: ['message'], template: '<div>{{ message }}<slot /></div>' },
  'a-table': {
    props: ['dataSource', 'columns'],
    template: '<div><span v-for="col in (columns || [])" :key="col.key">{{ col.title }}</span><div v-for="item in (dataSource || [])" :key="item.mapping_id || item.key">{{ item.label || item.mapping_id }}</div></div>'
  },
  'a-switch': { template: '<input type="checkbox" />' },
  'a-tag': { template: '<span><slot /></span>' },
  'a-popconfirm': { template: '<div><slot /></div>' },
  'a-drawer': { template: '<div><slot /><slot name="footer" /></div>' },
  'a-row': { template: '<div class="stub-row"><slot /></div>' },
  'a-col': { template: '<div class="stub-col"><slot /></div>' },
  'a-input-number': { template: '<input type="number" />' },
  'a-textarea': { template: '<textarea />' }
}

const startupSummaryPayload = {
  node_id: 'node', network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
  capability: { supports_large_request_body: false, supports_large_response_body: true, supports_streaming_response: false, supports_bidirectional_http_tunnel: false, supports_transparent_proxy: false },
  capability_summary: { supported: ['small_request'], unsupported: ['large_request'], items: [] },
  config_path: '', config_source: '', ui_mode: 'embedded', ui_url: '', api_url: '',
  transport_plan: { request_meta_transport: 'sip_control', request_body_transport: 'sip_body_only', response_meta_transport: 'sip_control', response_body_transport: 'rtp_stream', request_body_size_limit: 999999999, response_body_size_limit: 999999999, notes: [], warnings: [] },
  business_execution: { state: 'active', route_count: 1, message: '', impact: '' },
  self_check_summary: { generated_at: '', overall: 'info', info: 1, warn: 0, error: 0 }
}

describe('TunnelMappingsView', () => {
  it('renders single-column editor fields in ops-friendly order', async () => {
    vi.mocked(gatewayApi.fetchMappings).mockResolvedValue({
      items: [
        {
          mapping_id: 'map-1', enabled: true,
          local_bind_ip: '10.0.0.1', local_bind_port: 18080, local_base_path: '/a',
          remote_target_ip: '10.0.0.2', remote_target_port: 8080, remote_base_path: '/b',
          allowed_methods: ['*'], connect_timeout_ms: 500, request_timeout_ms: 1000, response_timeout_ms: 2000,
          max_request_body_bytes: 1024, max_response_body_bytes: 2048, require_streaming_response: false, description: '', updated_at: '2026-03-14T09:00:00Z', link_status: 'degraded', status_reason: '心跳超时'
        }
      ],
      warnings: [],
      bound_peer: { peer_node_id: 'peer-b', peer_name: 'Peer B', peer_signaling_ip: '10.20.0.20', peer_signaling_port: 5060 }
    })
    vi.mocked(gatewayApi.fetchStartupSummary).mockResolvedValue(startupSummaryPayload as never)
    vi.mocked(gatewayApi.testMapping).mockResolvedValue({ sip_request: 'success', rtp_channel: 'fail' })
    vi.mocked(gatewayApi.fetchSystemStatus).mockResolvedValue({
      tunnel_status: 'degraded',
      connection_reason: '对端不可达',
      network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
      registration_status: 'registered',
      heartbeat_status: 'timeout',
      capability: {
        supports_small_request_body: true,
        supports_large_response_body: true,
        supports_streaming_response: false,
        supports_large_file_upload: false,
        supports_bidirectional_http_tunnel: false
      }
    })

    const wrapper = mount(TunnelMappingsView, { global: { stubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('序号')
    expect(wrapper.text()).toContain('映射链路状态')
    expect(wrapper.text()).toContain('状态原因')
    expect(wrapper.text()).toContain('更新时间')
    expect(wrapper.text()).not.toContain('名称')
    expect(wrapper.text()).not.toContain('对端节点')
    expect(wrapper.text()).not.toContain('方法白名单')
    expect(wrapper.text()).toContain('peer-b')

    const text = wrapper.text()
    expect(text).toContain('本端入口 IP')
    expect(text).toContain('本端入口端口')
    expect(text).toContain('对端目标 IP')
    expect(text).toContain('对端目标端口')
    expect(text).toContain('系统按动作类型自动选择命令或文件传输链路。')
    expect(text).toContain('备注')

    const indexLocalIp = text.indexOf('本端入口 IP')
    const indexLocalPort = text.indexOf('本端入口端口')
    const indexRemoteIp = text.indexOf('对端目标 IP')
    const indexRemotePort = text.indexOf('对端目标端口')
    const indexReqTimeout = text.indexOf('请求超时（毫秒）')
    const indexRespTimeout = text.indexOf('响应超时（毫秒）')
    const indexReqLimit = text.indexOf('请求体大小上限（字节）')
    const indexRespLimit = text.indexOf('响应体大小上限（字节）')
    const indexEnabled = text.indexOf('启用状态')
    const indexRemark = text.indexOf('备注')

    expect(indexLocalIp).toBeGreaterThan(-1)
    expect(indexLocalPort).toBeGreaterThan(indexLocalIp)
    expect(indexRemoteIp).toBeGreaterThan(indexLocalPort)
    expect(indexRemotePort).toBeGreaterThan(indexRemoteIp)
    expect(indexReqTimeout).toBeGreaterThan(indexRemotePort)
    expect(indexRespTimeout).toBeGreaterThan(indexReqTimeout)
    expect(indexReqLimit).toBeGreaterThan(indexRespTimeout)
    expect(indexRespLimit).toBeGreaterThan(indexReqLimit)
    expect(indexEnabled).toBeGreaterThan(indexRespLimit)
    expect(indexRemark).toBeGreaterThan(indexEnabled)

    expect(wrapper.find('.stub-row').exists()).toBe(false)
  })

  it('uses default allowed_methods on save', async () => {
    vi.mocked(gatewayApi.fetchMappings).mockResolvedValue({ items: [], warnings: [], binding_error: "multiple enabled peer nodes configured" })
    vi.mocked(gatewayApi.fetchStartupSummary).mockResolvedValue(startupSummaryPayload as never)
    vi.mocked(gatewayApi.createMapping).mockResolvedValue({ mapping: {} as never, warnings: [] })
    vi.mocked(gatewayApi.fetchSystemStatus).mockResolvedValue({
      tunnel_status: 'connected',
      connection_reason: '正常',
      network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
      registration_status: 'registered',
      heartbeat_status: 'healthy',
      capability: {
        supports_small_request_body: true,
        supports_large_response_body: true,
        supports_streaming_response: false,
        supports_large_file_upload: false,
        supports_bidirectional_http_tunnel: false
      }
    })

    const wrapper = mount(TunnelMappingsView, { global: { stubs } })
    await flushPromises()

    const saveBtn = wrapper.findAll('button').find((btn) => btn.text() === '保存')
    expect(saveBtn).toBeTruthy()
    await saveBtn!.trigger('click')

    expect(gatewayApi.createMapping).not.toHaveBeenCalled()
  })
})
