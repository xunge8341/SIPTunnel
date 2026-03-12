import { defineStore } from 'pinia'
import type { NavigationItem } from '../types'

const navigation: NavigationItem[] = [
  { key: 'dashboard', label: 'Dashboard', path: '/dashboard' },
  { key: 'cmd-task', label: '命令任务', path: '/command-tasks' },
  { key: 'file-task', label: '文件任务', path: '/file-tasks' },
  { key: 'network-config', label: '网络配置', path: '/network-config' },
  { key: 'route-config', label: '路由配置', path: '/route-config' },
  { key: 'rate-limit', label: '限流策略', path: '/rate-limits' },
  { key: 'node-status', label: '节点状态', path: '/node-status' },
  { key: 'alerts', label: '告警中心', path: '/alerts' },
  { key: 'audit-log', label: '审计日志', path: '/audit-logs' }
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
