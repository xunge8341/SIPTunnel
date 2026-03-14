import { mount } from '@vue/test-utils'
import OpsToolsView from '../OpsToolsView.vue'

describe('OpsToolsView', () => {
  it('renders all operation tool tabs', () => {
    const wrapper = mount(OpsToolsView)

    expect(wrapper.text()).toContain('运维工具')
    expect(wrapper.text()).toContain('网络诊断')
    expect(wrapper.text()).toContain('端口检测')
    expect(wrapper.text()).toContain('通道测试')
    expect(wrapper.text()).toContain('配置校验')
  })

  it('renders tool actions', () => {
    const wrapper = mount(OpsToolsView)

    expect(wrapper.text()).toContain('开始诊断')
    expect(wrapper.text()).toContain('执行检测')
    expect(wrapper.text()).toContain('执行通道测试')
    expect(wrapper.text()).toContain('执行配置校验')
  })
})
