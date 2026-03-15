import { defineStore } from 'pinia'
import type { NavigationItem } from '../types'

const navigation: NavigationItem[] = [
  { key: 'dashboard', label: '总览监控', path: '/dashboard' },
  { key: 'nodes-tunnels', label: '节点与隧道', path: '/nodes-tunnels' },
  { key: 'tunnel-mappings', label: '隧道映射', path: '/tunnel-mappings' },
  { key: 'access-logs', label: '访问日志', path: '/access-logs' },
  { key: 'ops-audits', label: '运维审计', path: '/ops-audits' },
  { key: 'alerts-rate-limit', label: '告警与限流', path: '/alerts-rate-limit' },
  { key: 'system-settings', label: '系统设置', path: '/system-settings' },
  { key: 'diagnostics-loadtest', label: '诊断与压测', path: '/diagnostics-loadtest' },
  { key: 'security-center', label: '授权与安全', path: '/security-center' }
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
