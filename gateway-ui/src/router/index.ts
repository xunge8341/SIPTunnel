import { createRouter, createWebHistory } from 'vue-router'
import AdminLayout from '../layouts/AdminLayout.vue'
import DashboardView from '../views/DashboardView.vue'
import TunnelMappingsView from '../views/TunnelMappingsView.vue'
import SystemSettingsView from '../views/SystemSettingsView.vue'
import NodesAndTunnelsView from '../views/NodesAndTunnelsView.vue'
import AccessLogsView from '../views/AccessLogsView.vue'
import OpsAuditsView from '../views/OpsAuditsView.vue'
import AlertsAndRateLimitView from '../views/AlertsAndRateLimitView.vue'
import DiagnosticsLoadtestView from '../views/DiagnosticsLoadtestView.vue'
import SecurityCenterView from '../views/SecurityCenterView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: AdminLayout,
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', name: 'dashboard', component: DashboardView, meta: { title: '总览监控' } },
        { path: 'nodes-tunnels', name: 'nodes-tunnels', component: NodesAndTunnelsView, meta: { title: '节点与隧道' } },
        { path: 'tunnel-mappings', name: 'tunnel-mappings', component: TunnelMappingsView, meta: { title: '隧道映射' } },
        { path: 'access-logs', name: 'access-logs', component: AccessLogsView, meta: { title: '访问日志' } },
        { path: 'ops-audits', name: 'ops-audits', component: OpsAuditsView, meta: { title: '运维审计' } },
        { path: 'alerts-protection', name: 'alerts-protection', component: AlertsAndRateLimitView, meta: { title: '告警与保护' } },
        { path: 'system-settings', name: 'system-settings', component: SystemSettingsView, meta: { title: '系统设置' } },
        { path: 'diagnostics-loadtest', name: 'diagnostics-loadtest', component: DiagnosticsLoadtestView, meta: { title: '诊断与压测' } },
        { path: 'security-center', name: 'security-center', component: SecurityCenterView, meta: { title: '授权与安全' } }
      ]
    }
  ]
})

export default router
