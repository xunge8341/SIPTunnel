import { createRouter, createWebHistory } from 'vue-router'
import AdminLayout from '../layouts/AdminLayout.vue'
import DashboardView from '../views/DashboardView.vue'
import PlaceholderView from '../views/PlaceholderView.vue'
import CommandTasksView from '../views/CommandTasksView.vue'
import FileTasksView from '../views/FileTasksView.vue'
import TaskDetailView from '../views/TaskDetailView.vue'

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
          path: 'route-config',
          name: 'route-config',
          component: PlaceholderView,
          meta: { title: '路由配置', description: '维护 api_code 到 HTTP 模板的映射关系。' }
        },
        {
          path: 'rate-limits',
          name: 'rate-limit',
          component: PlaceholderView,
          meta: { title: '限流策略', description: '为各节点与业务配置限流、熔断策略。' }
        },
        {
          path: 'node-status',
          name: 'node-status',
          component: PlaceholderView,
          meta: { title: '节点状态', description: '查看各边界节点连接与健康状态。' }
        },
        {
          path: 'alerts',
          name: 'alerts',
          component: PlaceholderView,
          meta: { title: '告警中心', description: '统一查看并处理系统告警。' }
        },
        {
          path: 'audit-logs',
          name: 'audit-log',
          component: PlaceholderView,
          meta: { title: '审计日志', description: '查询控制面与执行面审计日志。' }
        }
      ]
    }
  ]
})

export default router
