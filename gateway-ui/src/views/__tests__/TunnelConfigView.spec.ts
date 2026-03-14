import { flushPromises, mount } from '@vue/test-utils'
import TunnelConfigView from '../TunnelConfigView.vue'
import { gatewayApi } from '../../api/gateway'

vi.mock('../../api/gateway', () => ({
  gatewayApi: {
    fetchTunnelConfig: vi.fn(),
    saveTunnelConfig: vi.fn(),
    triggerTunnelSessionAction: vi.fn()
  }
}))

vi.mock('ant-design-vue', () => ({ message: { success: vi.fn() } }))

const stubs = {
  'a-card': { template: '<section><slot /></section>' },
  'a-form': { template: '<form><slot /></form>' },
  'a-row': { template: '<div><slot /></div>' },
  'a-col': { template: '<div><slot /></div>' },
  'a-form-item': { template: '<div><slot /></div>' },
  'a-input': { template: '<input />' },
  'a-input-number': { template: '<input />' },
  'a-radio-group': { template: '<div><slot /></div>' },
  'a-radio-button': { template: '<button><slot /></button>' },
  'a-space': { template: '<div><slot /></div>' },
  'a-button': { template: '<button @click="$emit(`click`)"><slot /></button>' },
  'a-statistic': { template: '<div><slot /></div>' },
  'a-tag': { template: '<span><slot /></span>' },
  'a-alert': { template: '<div><slot /></div>' }
}

describe('TunnelConfigView', () => {
  it('shows registration heartbeat info and saves config', async () => {
    vi.mocked(gatewayApi.fetchTunnelConfig).mockResolvedValue({
      channel_protocol: 'GB/T 28181',
      connection_initiator: 'LOCAL',
      local_device_id: '34020000001320000001',
      peer_device_id: '34020000002000000001',
      heartbeat_interval_sec: 60,
      register_retry_count: 3,
      register_retry_interval_sec: 10,
      registration_status: 'registered',
      last_register_time: '2026-03-14T10:00:00Z',
      last_heartbeat_time: '2026-03-14T10:00:30Z',
      heartbeat_status: 'healthy',
      last_failure_reason: '',
      next_retry_time: '',
      consecutive_heartbeat_timeout: 0,
      supported_capabilities: ['支持小请求体（典型 SIP JSON 负载）'],
      request_channel: 'SIP',
      response_channel: 'RTP',
      network_mode: 'SENDER_SIP__RECEIVER_RTP',
      capability: {
        supports_small_request_body: true,
        supports_large_request_body: false,
        supports_large_response_body: true,
        supports_streaming_response: true,
        supports_bidirectional_http_tunnel: false,
        supports_transparent_http_proxy: false
      },
      capability_items: []
    })
    vi.mocked(gatewayApi.saveTunnelConfig).mockResolvedValue({} as any)

    const wrapper = mount(TunnelConfigView, { global: { stubs } })
    await flushPromises()

    expect(gatewayApi.fetchTunnelConfig).toHaveBeenCalled()
    expect(wrapper.text()).toContain('单向请求：发送端 -> 接收端（SIP）')
    expect(wrapper.text()).toContain('单向响应：接收端 -> 发送端（RTP）')
    expect(wrapper.text()).toMatchSnapshot()

    const saveButton = wrapper.findAll('button').find((btn) => btn.text() === '保存配置')
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()
    expect(gatewayApi.saveTunnelConfig).toHaveBeenCalled()
    const calls = vi.mocked(gatewayApi.saveTunnelConfig).mock.calls
    expect(calls[calls.length - 1][0]).toMatchObject({
      channel_protocol: 'GB/T 28181',
      connection_initiator: 'LOCAL',
      heartbeat_interval_sec: 60,
      register_retry_count: 3
    })
    expect(calls[calls.length - 1][0]).not.toHaveProperty('local_device_id')
    expect(calls[calls.length - 1][0]).not.toHaveProperty('peer_device_id')

    vi.mocked(gatewayApi.triggerTunnelSessionAction).mockResolvedValue({} as any)
    const registerNowButton = wrapper.findAll('button').find((btn) => btn.text() === '立即注册')
    expect(registerNowButton).toBeTruthy()
    await registerNowButton!.trigger('click')
    await flushPromises()
    expect(gatewayApi.triggerTunnelSessionAction).toHaveBeenCalledWith({ action: 'register_now' })
  })
})
