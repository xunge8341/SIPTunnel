import { createRouter, createWebHistory } from 'vue-router'
import { resolveUIRouterBase } from '../utils/uiBase'

const AdminLayout = () => import('../layouts/AdminLayout.vue')
const DashboardView = () => import('../views/DashboardView.vue')
const TunnelMappingsView = () => import('../views/TunnelMappingsView.vue')
const LocalResourcesView = () => import('../views/LocalResourcesView.vue')
const LinkMonitorView = () => import('../views/LinkMonitorView.vue')
const SystemSettingsView = () => import('../views/SystemSettingsView.vue')
const NodesAndTunnelsView = () => import('../views/NodesAndTunnelsView.vue')
const AccessLogsView = () => import('../views/AccessLogsView.vue')
const OpsAuditsView = () => import('../views/OpsAuditsView.vue')
const AlertsAndRateLimitView = () => import('../views/AlertsAndRateLimitView.vue')
const DiagnosticsLoadtestView = () => import('../views/DiagnosticsLoadtestView.vue')
const SecurityCenterView = () => import('../views/SecurityCenterView.vue')
const SecurityEventsView = () => import('../views/SecurityEventsView.vue')

const router = createRouter({
  history: createWebHistory(resolveUIRouterBase()),
  routes: [
    {
      path: '/',
      component: AdminLayout,
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', name: 'dashboard', component: DashboardView, meta: { title: '总览监控' } },
        { path: 'nodes-tunnels', name: 'nodes-tunnels', component: NodesAndTunnelsView, meta: { title: '节点与级联' } },
        { path: 'local-resources', name: 'local-resources', component: LocalResourcesView, meta: { title: '本地资源' } },
        { path: 'tunnel-mappings', name: 'tunnel-mappings', component: TunnelMappingsView, meta: { title: '隧道映射' } },
        { path: 'link-monitor', name: 'link-monitor', component: LinkMonitorView, meta: { title: '链路监控' } },
        { path: 'access-logs', name: 'access-logs', component: AccessLogsView, meta: { title: '访问日志' } },
        { path: 'ops-audits', name: 'ops-audits', component: OpsAuditsView, meta: { title: '运维审计' } },
        { path: 'alerts-protection', name: 'alerts-protection', component: AlertsAndRateLimitView, meta: { title: '告警与保护' } },
        { path: 'system-settings', name: 'system-settings', component: SystemSettingsView, meta: { title: '系统设置' } },
        { path: 'diagnostics-loadtest', name: 'diagnostics-loadtest', component: DiagnosticsLoadtestView, meta: { title: '诊断与压测' } },
        { path: 'security-center', name: 'security-center', component: SecurityCenterView, meta: { title: '授权管理' } },
        { path: 'security-events', name: 'security-events', component: SecurityEventsView, meta: { title: '安全事件' } }
      ]
    }
  ]
})

export default router
