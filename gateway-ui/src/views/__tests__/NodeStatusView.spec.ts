import { flushPromises, mount } from '@vue/test-utils'
import NodeStatusView from '../NodeStatusView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    createDiagnosticExport: vi.fn(),
    fetchDiagnosticExport: vi.fn(),
    retryDiagnosticExport: vi.fn()
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
  'a-statistic': { template: '<div />' },
  'a-alert': { template: '<div><slot /></div>' },
  'a-select': { template: '<select><slot /></select>' },
  'a-select-option': { template: '<option><slot /></option>' },
  'a-empty': { template: '<div />' },
  'a-list': { template: '<ul><slot name="renderItem" :item="{}" /></ul>' },
  'a-list-item': { template: '<li><slot /></li>' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' },
  StatusPill: { template: '<span />' }
}

describe('NodeStatusView', () => {
  it('can create and render a diagnostic export job', async () => {
    vi.mocked(gatewayApi.createDiagnosticExport).mockResolvedValue({
      jobId: 'diag-001',
      nodeId: 'gateway-a-01',
      status: 'pending',
      progress: 0,
      startedAt: '2026-03-12T08:00:00Z',
      updatedAt: '2026-03-12T08:00:00Z',
      fileName: 'diag_gateway-a-01_20260312T080000_diag-001.zip',
      sections: [
        { key: 'config_snapshot', label: '当前配置快照', done: false },
        { key: 'node_runtime', label: '节点运行状态', done: false },
        { key: 'failed_tasks', label: '最近失败任务摘要', done: false },
        { key: 'log_index', label: '关键日志索引', done: false },
        { key: 'alerts_summary', label: '最近告警摘要', done: false }
      ]
    })
    vi.mocked(gatewayApi.fetchDiagnosticExport).mockResolvedValue({
      jobId: 'diag-001',
      nodeId: 'gateway-a-01',
      status: 'collecting',
      progress: 45,
      startedAt: '2026-03-12T08:00:00Z',
      updatedAt: '2026-03-12T08:00:03Z',
      fileName: 'diag_gateway-a-01_20260312T080000_diag-001.zip',
      sections: [
        { key: 'config_snapshot', label: '当前配置快照', done: true },
        { key: 'node_runtime', label: '节点运行状态', done: true },
        { key: 'failed_tasks', label: '最近失败任务摘要', done: false },
        { key: 'log_index', label: '关键日志索引', done: false },
        { key: 'alerts_summary', label: '最近告警摘要', done: false }
      ]
    })

    const wrapper = mount(NodeStatusView, { global: { stubs } })
    await wrapper.findAll('button')[1].trigger('click')
    await flushPromises()

    expect(gatewayApi.createDiagnosticExport).toHaveBeenCalled()
    expect(gatewayApi.fetchDiagnosticExport).toHaveBeenCalled()
    expect(wrapper.text()).toContain('diag-001')
    expect(wrapper.text()).toContain('正在采集信息')
  })
})
