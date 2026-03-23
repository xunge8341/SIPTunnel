import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import AdminLayout from '../AdminLayout.vue'
import { useAppStore } from '../../stores/app'

const pushMock = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'dashboard', meta: { title: '首页' } }),
  useRouter: () => ({ push: pushMock })
}))

describe('AdminLayout', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    pushMock.mockReset()
  })

  it('toggles sidebar when header button is clicked', async () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-view': true,
          'global-message-host': true
        }
      }
    })

    const appStore = useAppStore()
    expect(appStore.collapsed).toBe(false)

    await wrapper.find('button').trigger('click')
    expect(appStore.collapsed).toBe(true)
  })

  it('renders brand and keeps delivery navigation config', () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-view': true,
          'global-message-host': true
        }
      }
    })

    const appStore = useAppStore()
    expect(wrapper.text()).toContain('隧道控制台')
    expect(appStore.navigation.map((item) => item.label)).toEqual(['总览监控', '节点与级联', '本地资源', '隧道映射', '链路监控', '访问日志', '运维审计', '告警与保护', '系统设置', '诊断与压测', '授权管理', '安全事件'])
  })
})
