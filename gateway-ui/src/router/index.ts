import { createRouter, createWebHistory } from 'vue-router'
import AdminLayout from '../layouts/AdminLayout.vue'
import DashboardView from '../views/DashboardView.vue'
import RateLimitPoliciesView from '../views/RateLimitPoliciesView.vue'
import TunnelMappingsView from '../views/TunnelMappingsView.vue'
import NodeStatusView from '../views/NodeStatusView.vue'
import AlertsCenterView from '../views/AlertsCenterView.vue'
import AuditLogsView from '../views/AuditLogsView.vue'
import CommandTasksView from '../views/CommandTasksView.vue'
import FileTasksView from '../views/FileTasksView.vue'
import TaskDetailView from '../views/TaskDetailView.vue'
import NetworkConfigView from '../views/NetworkConfigView.vue'
import ConfigGovernanceView from '../views/ConfigGovernanceView.vue'
import LocalNodeConfigView from '../views/LocalNodeConfigView.vue'
import PeerNodeConfigView from '../views/PeerNodeConfigView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: AdminLayout,
      redirect: '/dashboard',
      children: [
        {
          path: 'dashboard',
          name: 'dashboard',
          component: DashboardView,
          meta: { title: 'Dashboard', description: '系统关键指标与运行状态总览。' }
        },
        {
          path: 'command-tasks',
          name: 'cmd-task',
          component: CommandTasksView,
          meta: { title: '命令任务', description: '管理 SIP 控制面下发的命令任务。' }
        },
        {
          path: 'file-tasks',
          name: 'file-task',
          component: FileTasksView,
          meta: { title: '文件任务', description: '管理 RTP 文件面传输任务。' }
        },
        {
          path: 'tasks/:taskKind/:id',
          name: 'task-detail',
          component: TaskDetailView,
          meta: { title: '任务详情', description: '查看任务全链路执行详情。' }
        },
        {
          path: 'network-config',
          name: 'network-config',
          component: NetworkConfigView,
          meta: { title: '网络配置', description: '统一管理 SIP/RTP 网络参数与端口池状态。' }
        },
        {
          path: 'local-node-config',
          name: 'local-node-config',
          component: LocalNodeConfigView,
          meta: { title: '本端节点配置', description: '管理本端 node_id / network_mode / SIP / RTP 参数。' }
        },
        {
          path: 'peer-node-config',
          name: 'peer-node-config',
          component: PeerNodeConfigView,
          meta: { title: '对端节点配置', description: '管理 peer signaling/media 与模式兼容性。' }
        },
        {
          path: 'config-governance',
          name: 'config-governance',
          component: ConfigGovernanceView,
          meta: { title: '配置治理', description: '快照、对比、回滚与 YAML 导出。' }
        },
        {
          path: 'tunnel-mappings',
          name: 'tunnel-mappings',
          component: TunnelMappingsView,
          meta: { title: '隧道映射', description: '维护隧道映射（本端入口 -> 对端目标）核心字段，展示网络模式能力矩阵，并对超能力配置进行告警/拦截。' }
        },
        {
          path: 'rate-limits',
          name: 'rate-limit',
          component: RateLimitPoliciesView,
          meta: { title: '限流策略', description: '为各节点与业务配置限流、熔断策略。' }
        },
        {
          path: 'node-status',
          name: 'node-status',
          component: NodeStatusView,
          meta: { title: '节点状态', description: '查看各边界节点连接与健康状态。' }
        },
        {
          path: 'alerts',
          name: 'alerts',
          component: AlertsCenterView,
          meta: { title: '告警中心', description: '统一查看并处理系统告警。' }
        },
        {
          path: 'audit-logs',
          name: 'audit-log',
          component: AuditLogsView,
          meta: { title: '审计日志', description: '查询控制面与执行面审计日志。' }
        }
      ]
    }
  ]
})

export default router
