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
    testMapping: vi.fn()
  }
}))

vi.mock('ant-design-vue', () => ({
  message: { success: vi.fn(), warning: vi.fn(), error: vi.fn() }
}))

const stubs = {
  'a-space': { template: '<div><slot /></div>' },
  'a-card': { template: '<section><slot /></section>' },
  'a-form': { template: '<form><slot /></form>' },
  'a-form-item': { template: '<div><slot /></div>' },
  'a-input': { template: '<input />' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' },
  'a-descriptions': { template: '<div><slot /></div>' },
  'a-descriptions-item': { template: '<div><slot /></div>' },
  'a-alert': { props: ['message'], template: '<div>{{ message }}<slot /></div>' },
  'a-table': {
    props: ['dataSource'],
    template: '<div><div v-for="item in (dataSource || [])" :key="item.mapping_id || item.key">{{ item.label || item.mapping_id }}</div><slot name="bodyCell" :column="{key: `action`}" :record="record" /></div>',
    data: () => ({ record: { mapping_id: 'map-1' } })
  },
  'a-switch': { template: '<input type="checkbox" />' },
  'a-tag': { template: '<span><slot /></span>' },
  'a-popconfirm': { template: '<div><slot /></div>' },
  'a-drawer': { template: '<div><slot /><slot name="footer" /></div>' },
  'a-row': { template: '<div><slot /></div>' },
  'a-col': { template: '<div><slot /></div>' },
  'a-input-number': { template: '<input type="number" />' },
  'a-textarea': { template: '<textarea />' }
}

describe('TunnelMappingsView', () => {
  it('renders mappings and readonly transport context', async () => {
    vi.mocked(gatewayApi.fetchMappings).mockResolvedValue({
      items: [
        {
          mapping_id: 'map-1', name: '订单', enabled: true, peer_node_id: 'peer-1',
          local_bind_ip: '10.0.0.1', local_bind_port: 18080, local_base_path: '/a',
          remote_target_ip: '10.0.0.2', remote_target_port: 8080, remote_base_path: '/b',
          allowed_methods: ['POST'], connect_timeout_ms: 500, request_timeout_ms: 1000, response_timeout_ms: 2000,
          max_request_body_bytes: 1024, max_response_body_bytes: 2048, require_streaming_response: false, description: ''
        }
      ],
      warnings: ['warning-1']
    })
    vi.mocked(gatewayApi.testMapping).mockResolvedValue({ sip_request: 'success', rtp_channel: 'fail' })
    vi.mocked(gatewayApi.fetchStartupSummary).mockResolvedValue({
      node_id: 'node', network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
      capability: { supports_large_request_body: false, supports_large_response_body: true, supports_streaming_response: false, supports_bidirectional_http_tunnel: false, supports_transparent_proxy: false },
      capability_summary: { supported: ['small_request'], unsupported: ['large_request'], items: [] },
      config_path: '', config_source: '', ui_mode: 'embedded', ui_url: '', api_url: '',
      transport_plan: { request_meta_transport: 'sip_control', request_body_transport: 'sip_body_only', response_meta_transport: 'sip_control', response_body_transport: 'rtp_stream', request_body_size_limit: 1, response_body_size_limit: 2, notes: [], warnings: [] },
      business_execution: { state: 'active', route_count: 1, message: '', impact: '' },
      self_check_summary: { generated_at: '', overall: 'info', info: 1, warn: 0, error: 0 }
    })

    const wrapper = mount(TunnelMappingsView, { global: { stubs } })
    await flushPromises()

    expect(gatewayApi.fetchMappings).toHaveBeenCalled()
    expect(gatewayApi.fetchStartupSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('A_TO_B_SIP__B_TO_A_RTP')
    expect(wrapper.text()).toContain('warning-1')
    expect(wrapper.text()).toContain('小请求体')
    expect(wrapper.text()).toContain('测试映射规则')
  })
})
