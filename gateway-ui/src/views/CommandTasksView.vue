<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="筛选区">
      <a-form layout="inline">
        <a-form-item label="状态">
          <a-select v-model:value="filters.status" allow-clear style="width: 140px" :options="statusOptions" />
        </a-form-item>
        <a-form-item label="request_id">
          <a-input v-model:value="filters.requestId" placeholder="REQ-CMD-0001" allow-clear />
        </a-form-item>
        <a-form-item label="trace_id">
          <a-input v-model:value="filters.traceId" placeholder="TRACE-xxxx" allow-clear />
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button type="primary" @click="loadData(1)">查询</a-button>
            <a-button @click="reset">重置</a-button>
          </a-space>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card title="命令任务列表">
      <a-table
        :columns="columns"
        :data-source="result.list"
        :pagination="pagination"
        row-key="id"
        @change="onTableChange"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="statusColorMap[record.status]">{{ statusTextMap[record.status] }}</a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button type="link" @click="goDetail(record.id)">详情</a-button>
          </template>
        </template>
      </a-table>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { gatewayApi } from '../api/gateway'
import type { CommandTask, TaskListFilters, TaskListResult } from '../types/gateway'

const router = useRouter()
const filters = reactive<TaskListFilters>({})
const result = ref<TaskListResult<CommandTask>>({ list: [], total: 0, page: 1, pageSize: 10 })

const statusTextMap: Record<string, string> = {
  pending: '待执行',
  accepted: '已受理',
  running: '执行中',
  transferring: '传输中',
  verifying: '校验中',
  retry_wait: '重试等待',
  succeeded: '成功',
  failed: '失败',
  dead_lettered: '死信',
  cancelled: '已取消'
}

const statusColorMap: Record<string, string> = {
  pending: 'default',
  accepted: 'blue',
  running: 'processing',
  transferring: 'cyan',
  verifying: 'purple',
  retry_wait: 'orange',
  succeeded: 'success',
  failed: 'error',
  dead_lettered: 'red',
  cancelled: 'default'
}

const statusOptions = Object.entries(statusTextMap).map(([value, label]) => ({ value, label }))

const columns = [
  { title: '任务ID', dataIndex: 'id', key: 'id' },
  { title: 'request_id', dataIndex: 'requestId', key: 'requestId' },
  { title: 'trace_id', dataIndex: 'traceId', key: 'traceId' },
  { title: 'api_code', dataIndex: 'apiCode', key: 'apiCode' },
  { title: '状态', dataIndex: 'status', key: 'status' },
  { title: '延迟(ms)', dataIndex: 'latencyMs', key: 'latencyMs' },
  { title: '更新时间', dataIndex: 'updatedAt', key: 'updatedAt' },
  { title: '操作', key: 'action' }
]

const pagination = ref({ current: 1, pageSize: 10, total: 0, showSizeChanger: true })

const loadData = async (page = pagination.value.current, pageSize = pagination.value.pageSize) => {
  result.value = await gatewayApi.fetchCommandTasks(filters, page, pageSize)
  pagination.value = { ...pagination.value, current: page, pageSize, total: result.value.total }
}

const onTableChange = (pager: { current?: number; pageSize?: number }) => {
  loadData(pager.current ?? 1, pager.pageSize ?? 10)
}

const reset = () => {
  Object.assign(filters, { status: undefined, requestId: '', traceId: '' })
  loadData(1)
}

const goDetail = (id: string) => {
  router.push({ name: 'task-detail', params: { taskKind: 'command', id } })
}

onMounted(() => loadData(1))
</script>
