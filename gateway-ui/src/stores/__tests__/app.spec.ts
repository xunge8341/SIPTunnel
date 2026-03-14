import { createPinia, setActivePinia } from 'pinia'
import { useAppStore } from '../app'

describe('app store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('toggles sidebar collapsed state', () => {
    const store = useAppStore()
    expect(store.collapsed).toBe(false)

    store.toggleSidebar()
    expect(store.collapsed).toBe(true)

    store.toggleSidebar()
    expect(store.collapsed).toBe(false)
  })

  it('contains route navigation definitions', () => {
    const store = useAppStore()
    expect(store.navigation.length).toBeGreaterThan(0)
    expect(store.navigation.some((item) => item.path === '/dashboard')).toBe(true)
    expect(store.navigation.some((item) => item.path === '/ops-tools')).toBe(true)
  })
})
