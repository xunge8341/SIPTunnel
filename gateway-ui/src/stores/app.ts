import { defineStore } from 'pinia'
import type { NavigationItem } from '../types'

const navigation: NavigationItem[] = [
  { key: 'dashboard', label: '首页', path: '/dashboard' },
  { key: 'node-config', label: '节点配置', path: '/node-config' },
  { key: 'tunnel-config', label: '通道配置', path: '/tunnel-config' },
  { key: 'tunnel-mappings', label: '映射规则', path: '/tunnel-mappings' },
  { key: 'audit-log', label: '日志', path: '/audit-logs' },
  { key: 'ops-tools', label: '运维工具', path: '/ops-tools' }
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
