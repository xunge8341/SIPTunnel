import { flushPromises, mount } from '@vue/test-utils'
import NodeStatusView from '../NodeStatusView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    createDiagnosticExport: vi.fn(),
    fetchDiagnosticExport: vi.fn(),
    retryDiagnosticExport: vi.fn(),
    runLinkTest: vi.fn(),
    fetchLatestLinkTest: vi.fn(),
    fetchSystemStatus: vi.fn(),
    fetchMappings: vi.fn()
  }
}))

const stubs = {
  'a-space': { template: '<div><slot /></div>' },
  'a-card': { template: '<section><slot /></section>' },
  'a-row': { template: '<div><slot /></div>' },
  'a-col': { template: '<div><slot /></div>' },
  'a-descriptions': { template: '<div><slot /></div>' },
  'a-descriptions-item': { template: '<div><slot /></div>' },
  'a-typography-text': { template: '<span><slot /></span>' },
  'a-progress': { template: '<div />' },
  'a-statistic': { props: ['title', 'value'], template: '<div>{{ title }}{{ value }}</div>' },
  'a-alert': { props: ['message', 'description'], template: '<div>{{ message }}{{ description }}<slot /></div>' },
  'a-select': { template: '<select><slot /></select>' },
  'a-select-option': { template: '<option><slot /></option>' },
  'a-empty': { template: '<div />' },
  'a-list': { template: '<ul><slot name="renderItem" :item="{}" /></ul>' },
  'a-list-item': { template: '<li><slot /></li>' },
  'a-table': { template: '<div />' },
  'a-table-column': { template: '<div />' },
  'a-tag': { template: '<span><slot /></span>' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' },
  'a-input': { template: '<input />' },
  StatusPill: { template: '<span />' }
}

describe('NodeStatusView', () => {
  it('can create and render a diagnostic export job', async () => {
    vi.mocked(gatewayApi.fetchLatestLinkTest).mockRejectedValue(new Error('no report'))
    vi.mocked(gatewayApi.fetchSystemStatus).mockResolvedValue({
      tunnel_status: 'connected',
      connection_reason: '链路稳定',
      network_mode: 'SENDER_SIP__RECEIVER_RTP',
      registration_status: 'registered',
      heartbeat_status: 'healthy',
      last_register_time: '2026-03-12T08:00:00Z',
      last_heartbeat_time: '2026-03-12T08:00:30Z',
      mapping_total: 2,
      mapping_abnormal_total: 1,
      latest_mapping_error_reason: 'map-2：心跳超时',
      capability: {
        supports_small_request_body: true,
        supports_large_response_body: true,
        supports_streaming_response: false,
        supports_large_file_upload: false,
        supports_bidirectional_http_tunnel: false
      }
    })
    vi.mocked(gatewayApi.fetchMappings).mockResolvedValue({
      items: [
        { mapping_id: 'map-1', enabled: true, local_bind_ip: '1.1.1.1', local_bind_port: 80, local_base_path: '/', remote_target_ip: '2.2.2.2', remote_target_port: 80, remote_base_path: '/', connect_timeout_ms: 100, request_timeout_ms: 100, response_timeout_ms: 100, max_request_body_bytes: 1024, max_response_body_bytes: 1024, require_streaming_response: false, description: '', link_status: 'connected', status_reason: '正常' },
        { mapping_id: 'map-2', enabled: true, local_bind_ip: '1.1.1.2', local_bind_port: 81, local_base_path: '/', remote_target_ip: '2.2.2.3', remote_target_port: 81, remote_base_path: '/', connect_timeout_ms: 100, request_timeout_ms: 100, response_timeout_ms: 100, max_request_body_bytes: 1024, max_response_body_bytes: 1024, require_streaming_response: false, description: '', link_status: 'abnormal', status_reason: '心跳超时' }
      ]
    })
    vi.mocked(gatewayApi.createDiagnosticExport).mockResolvedValue({
      jobId: 'diag-001',
      nodeId: 'gateway-a-01',
      status: 'pending',
      progress: 0,
      startedAt: '2026-03-12T08:00:00Z',
      updatedAt: '2026-03-12T08:00:00Z',
      fileName: 'diag_gateway_a_01_20260312T080000Z_req_req-1_trace_trace-1_diag-001.zip',
      sections: [
        { key: 'transport_config', label: '当前 transport 配置', done: false },
        { key: 'connection_stats_snapshot', label: '连接统计快照', done: false },
        { key: 'port_pool_status', label: '端口池状态', done: false },
        { key: 'transport_error_summary', label: '最近 transport 错误摘要', done: false },
        { key: 'task_failure_summary', label: '最近 task failure 摘要', done: false },
        { key: 'rate_limit_hit_summary', label: '最近 rate limit 命中摘要', done: false },
        { key: 'profile_entry', label: 'profile 采集入口信息（如果启用）', done: false }
      ]
    })
    vi.mocked(gatewayApi.fetchDiagnosticExport).mockResolvedValue({
      jobId: 'diag-001',
      nodeId: 'gateway-a-01',
      status: 'collecting',
      progress: 45,
      startedAt: '2026-03-12T08:00:00Z',
      updatedAt: '2026-03-12T08:00:03Z',
      fileName: 'diag_gateway_a_01_20260312T080000Z_req_req-1_trace_trace-1_diag-001.zip',
      sections: [
        { key: 'transport_config', label: '当前 transport 配置', done: true },
        { key: 'connection_stats_snapshot', label: '连接统计快照', done: true },
        { key: 'port_pool_status', label: '端口池状态', done: true },
        { key: 'transport_error_summary', label: '最近 transport 错误摘要', done: false },
        { key: 'task_failure_summary', label: '最近 task failure 摘要', done: false },
        { key: 'rate_limit_hit_summary', label: '最近 rate limit 命中摘要', done: false },
        { key: 'profile_entry', label: 'profile 采集入口信息（如果启用）', done: false }
      ]
    })

    const wrapper = mount(NodeStatusView, { global: { stubs } })
    const exportButton = wrapper.findAll('button').find((btn) => btn.text().includes('导出诊断包'))
    expect(exportButton).toBeTruthy()
    await exportButton!.trigger('click')
    await flushPromises()

    expect(gatewayApi.createDiagnosticExport).toHaveBeenCalledWith({ nodeId: "gateway-a-01", requestId: undefined, traceId: undefined })
    expect(gatewayApi.fetchDiagnosticExport).toHaveBeenCalled()
    expect(wrapper.text()).toContain('diag-001')
    expect(wrapper.text()).toContain('正在采集信息')
    expect(wrapper.text()).toContain('自检通过')
    expect(wrapper.text()).toContain('注册状态')
    expect(wrapper.text()).toContain('map-2：心跳超时')
  })
})
