import { defineStore } from 'pinia'
import type { NavigationItem } from '../types'

const navigation: NavigationItem[] = [
  { key: 'dashboard', label: 'Dashboard', path: '/dashboard' },
  { key: 'node-config', label: 'Node Config', path: '/node-config' },
  { key: 'tunnel-config', label: 'Tunnel Config', path: '/tunnel-config' },
  { key: 'tunnel-mappings', label: 'Mapping', path: '/tunnel-mappings' },
  { key: 'system-settings', label: 'System Settings', path: '/system-settings' }
]

export const useAppStore = defineStore('app', {
  state: () => ({
    collapsed: false,
    navigation
  }),
  actions: {
    toggleSidebar() {
      this.collapsed = !this.collapsed
    }
  }
})
