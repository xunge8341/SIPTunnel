<template>
  <a-space direction="vertical" style="width: 100%">
    <a-card title="访问日志">
      <a-form layout="inline">
        <a-form-item label="状态">
          <a-select v-model:value="filters.status" style="width: 150px" allow-clear>
            <a-select-option value="running">进行中</a-select-option>
            <a-select-option value="succeeded">成功</a-select-option>
            <a-select-option value="failed">失败</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="请求ID">
          <a-input v-model:value="filters.requestId" allow-clear placeholder="定位单次访问" />
        </a-form-item>
        <a-form-item><a-button type="primary" @click="load">查询</a-button></a-form-item>
      </a-form>
    </a-card>
    <a-card>
      <a-table :columns="columns" :data-source="rows" row-key="id" :pagination="false">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status_code'">
            <a-tag :color="record.status_code >= 400 ? 'error' : 'success'">{{ record.status_code }}</a-tag>
          </template>
          <template v-else-if="column.key === 'failure_reason'">
            {{ record.failure_reason || '-' }}
          </template>
          <template v-else-if="column.key === 'detail'">
            <a-button type="link" size="small" @click="openDetail(record)">详情</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="detailOpen" title="访问详情" width="480">
      <a-descriptions :column="1" bordered size="small" v-if="activeRow">
        <a-descriptions-item label="请求ID">{{ activeRow.request_id }}</a-descriptions-item>
        <a-descriptions-item label="TraceID">{{ activeRow.trace_id }}</a-descriptions-item>
        <a-descriptions-item label="来源IP">{{ activeRow.source_ip }}</a-descriptions-item>
        <a-descriptions-item label="失败原因">{{ activeRow.failure_reason || '-' }}</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { AccessLogEntry, AccessLogFilters, TaskStatus } from '../types/gateway'

const filters = reactive<AccessLogFilters>({ status: undefined as TaskStatus | undefined, requestId: '' })
const rows = ref<AccessLogEntry[]>([])
const detailOpen = ref(false)
const activeRow = ref<AccessLogEntry>()

const columns = [
  { title: '时间', dataIndex: 'occurred_at', key: 'occurred_at' },
  { title: '映射名称', dataIndex: 'mapping_name', key: 'mapping_name' },
  { title: '来源IP', dataIndex: 'source_ip', key: 'source_ip' },
  { title: '方法', dataIndex: 'method', key: 'method' },
  { title: '路径', dataIndex: 'path', key: 'path' },
  { title: '状态码', dataIndex: 'status_code', key: 'status_code' },
  { title: '耗时(ms)', dataIndex: 'duration_ms', key: 'duration_ms' },
  { title: '失败原因摘要', dataIndex: 'failure_reason', key: 'failure_reason' },
  { title: '详情', key: 'detail' }
]

const openDetail = (row: AccessLogEntry) => {
  activeRow.value = row
  detailOpen.value = true
}

const load = async () => {
  const result = await gatewayApi.fetchAccessLogs(filters, 1, 100)
  rows.value = result.list
}

onMounted(load)
</script>
