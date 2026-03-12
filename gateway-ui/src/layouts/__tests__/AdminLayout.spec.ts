import { createPinia, setActivePinia } from 'pinia'
import { mount } from '@vue/test-utils'
import AdminLayout from '../AdminLayout.vue'
import { useAppStore } from '../../stores/app'

const pushMock = vi.fn()

vi.mock('vue-router', () => ({
  useRoute: () => ({ name: 'dashboard', meta: { title: 'Dashboard' } }),
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

  it('navigates to target path on menu click event', async () => {
    const wrapper = mount(AdminLayout, {
      global: {
        stubs: {
          'router-view': true,
          'global-message-host': true,
          'a-menu': {
            emits: ['click'],
            template: '<button class="menu-trigger" @click="$emit(\'click\', { key: \'file-task\' })">menu</button>'
          }
        }
      }
    })

    await wrapper.find('.menu-trigger').trigger('click')
    expect(pushMock).toHaveBeenCalledWith('/file-tasks')
  })
})
