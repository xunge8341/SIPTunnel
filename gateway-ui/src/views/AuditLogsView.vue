<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="审计查询">
      <a-form layout="inline">
        <a-form-item label="request_id">
          <a-input v-model:value="filters.requestId" allow-clear />
        </a-form-item>
        <a-form-item label="trace_id">
          <a-input v-model:value="filters.traceId" allow-clear />
        </a-form-item>
        <a-form-item label="时间范围">
          <a-range-picker v-model:value="timeRange" show-time />
        </a-form-item>
        <a-form-item label="操作类型">
          <a-select v-model:value="filters.action" allow-clear style="width: 160px" :options="actionOptions" />
        </a-form-item>
      </a-form>
    </a-card>

    <a-card title="审计日志">
      <a-table :columns="columns" :data-source="filteredLogs" row-key="id" :pagination="{ pageSize: 8 }">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'action'">
            <a-tag color="processing">{{ record.action }}</a-tag>
          </template>
          <template v-if="column.key === 'actionBtn'">
            <a-button type="link" @click="openDetail(record)">详情</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="drawerVisible" title="审计详情" width="620">
      <a-descriptions bordered :column="1">
        <a-descriptions-item label="request_id">{{ selectedLog.requestId }}</a-descriptions-item>
        <a-descriptions-item label="trace_id">{{ selectedLog.traceId }}</a-descriptions-item>
        <a-descriptions-item label="操作类型">{{ selectedLog.action }}</a-descriptions-item>
        <a-descriptions-item label="操作人">{{ selectedLog.operator }}</a-descriptions-item>
        <a-descriptions-item label="时间">{{ selectedLog.time }}</a-descriptions-item>
        <a-descriptions-item label="详情">{{ selectedLog.detail }}</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'

interface AuditLogItem {
  id: string
  requestId: string
  traceId: string
  action: string
  operator: string
  time: string
  detail: string
}

const filters = reactive({ requestId: '', traceId: '', action: undefined as string | undefined })
const timeRange = ref<any[]>([])
const drawerVisible = ref(false)
const selectedLog = reactive<AuditLogItem>({
  id: '',
  requestId: '',
  traceId: '',
  action: '',
  operator: '',
  time: '',
  detail: ''
})

const logs = ref<AuditLogItem[]>([
  {
    id: 'AUD-001',
    requestId: 'REQ-CMD-1001',
    traceId: 'TRACE-8AF01',
    action: 'UPDATE_RATE_LIMIT',
    operator: 'ops_admin',
    time: '2026-03-12 10:22:11',
    detail: '将 ORDER_SYNC QPS 调整为 300。'
  },
  {
    id: 'AUD-002',
    requestId: 'REQ-CMD-1002',
    traceId: 'TRACE-8AF02',
    action: 'UPDATE_ROUTE',
    operator: 'ops_admin',
    time: '2026-03-12 10:27:31',
    detail: '更新 USER_QUERY 的 timeout 为 2000ms。'
  },
  {
    id: 'AUD-003',
    requestId: 'REQ-FILE-1003',
    traceId: 'TRACE-8AF10',
    action: 'RETRY_TASK',
    operator: 'system',
    time: '2026-03-12 10:31:05',
    detail: '文件任务补片重试，缺片数从 4 降到 1。'
  }
])

const actionOptions = [
  { label: 'UPDATE_RATE_LIMIT', value: 'UPDATE_RATE_LIMIT' },
  { label: 'UPDATE_ROUTE', value: 'UPDATE_ROUTE' },
  { label: 'RETRY_TASK', value: 'RETRY_TASK' }
]

const columns = [
  { title: 'ID', dataIndex: 'id', key: 'id' },
  { title: 'request_id', dataIndex: 'requestId', key: 'requestId' },
  { title: 'trace_id', dataIndex: 'traceId', key: 'traceId' },
  { title: '操作类型', key: 'action' },
  { title: '操作人', dataIndex: 'operator', key: 'operator' },
  { title: '时间', dataIndex: 'time', key: 'time' },
  { title: '操作', key: 'actionBtn' }
]

const filteredLogs = computed(() =>
  logs.value.filter((item) => {
    if (filters.requestId && !item.requestId.includes(filters.requestId.trim())) return false
    if (filters.traceId && !item.traceId.includes(filters.traceId.trim())) return false
    if (filters.action && item.action !== filters.action) return false
    if (timeRange.value.length === 2) {
      const start = new Date(timeRange.value[0]).getTime()
      const end = new Date(timeRange.value[1]).getTime()
      const current = new Date(item.time).getTime()
      if (current < start || current > end) return false
    }
    return true
  })
)

const openDetail = (record: AuditLogItem) => {
  Object.assign(selectedLog, record)
  drawerVisible.value = true
}
</script>
