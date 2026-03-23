import { defineStore } from 'pinia'
import type { NavigationItem } from '../types'

const navigation: NavigationItem[] = [
  { key: 'dashboard', label: '总览监控', path: '/dashboard' },
  { key: 'nodes-tunnels', label: '节点与级联', path: '/nodes-tunnels' },
  { key: 'local-resources', label: '本地资源', path: '/local-resources' },
  { key: 'tunnel-mappings', label: '隧道映射', path: '/tunnel-mappings' },
  { key: 'link-monitor', label: '链路监控', path: '/link-monitor' },
  { key: 'access-logs', label: '访问日志', path: '/access-logs' },
  { key: 'ops-audits', label: '运维审计', path: '/ops-audits' },
  { key: 'alerts-protection', label: '告警与保护', path: '/alerts-protection' },
  { key: 'system-settings', label: '系统设置', path: '/system-settings' },
  { key: 'diagnostics-loadtest', label: '诊断与压测', path: '/diagnostics-loadtest' },
  { key: 'security-center', label: '授权管理', path: '/security-center' },
  { key: 'security-events', label: '安全事件', path: '/security-events' }
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
