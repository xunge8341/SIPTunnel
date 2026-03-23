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
        <a-form-item label="规则关键词">
          <a-input v-model:value="filters.rule" placeholder="api_code / ops_action / request_type" allow-clear />
        </a-form-item>
        <a-form-item label="时间范围">
          <a-range-picker
            v-model:value="selectedRange"
            show-time
            value-format="YYYY-MM-DDTHH:mm:ss[Z]"
            @change="onRangeChange"
          />
        </a-form-item>
        <a-form-item>
          <a-checkbox v-model:checked="filters.errorOnly">仅错误</a-checkbox>
        </a-form-item>
        <a-form-item>
          <a-button type="primary" @click="loadLogs(1)">查询</a-button>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card title="审计日志">
      <a-table :columns="columns" :data-source="logs" row-key="when" :pagination="pagination" @change="onTableChange">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'when'">
            <span>{{ formatDateTimeText(record.when) }}</span>
          </template>
          <template v-else-if="column.key === 'ops_action'">
            <a-tag color="processing">{{ record.ops_action || 'NONE' }}</a-tag>
          </template>
          <template v-if="column.key === 'final_result'">
            <a-tag :color="record.final_result === 'OK' ? 'success' : 'error'">{{ record.final_result || 'UNKNOWN' }}</a-tag>
          </template>
          <template v-if="column.key === 'actionBtn'">
            <a-button type="link" @click="openDetail(record)">详情</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="drawerVisible" title="审计详情" width="620">
      <a-descriptions bordered :column="1">
        <a-descriptions-item label="request_id">{{ selectedLog.core?.request_id }}</a-descriptions-item>
        <a-descriptions-item label="trace_id">{{ selectedLog.core?.trace_id }}</a-descriptions-item>
        <a-descriptions-item label="操作类型">{{ selectedLog.ops_action }}</a-descriptions-item>
        <a-descriptions-item label="操作人">{{ selectedLog.who }}</a-descriptions-item>
        <a-descriptions-item label="时间">{{ formatDateTimeText(selectedLog.when as string) }}</a-descriptions-item>
        <a-descriptions-item label="结果">{{ selectedLog.final_result }}</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import { formatDateTimeText } from '../utils/date'
import type { OpsAuditEvent, OpsAuditFilters } from '../types/gateway'

const filters = reactive<Required<Pick<OpsAuditFilters, 'requestId' | 'traceId' | 'rule'>> & Pick<OpsAuditFilters, 'errorOnly' | 'startTime' | 'endTime'>>({
  requestId: '',
  traceId: '',
  rule: '',
  errorOnly: false,
  startTime: undefined,
  endTime: undefined
})
const selectedRange = ref<[string, string] | undefined>()
const drawerVisible = ref(false)
const selectedLog = ref<Partial<OpsAuditEvent>>({})
const logs = ref<OpsAuditEvent[]>([])

const pagination = ref({ current: 1, pageSize: 8, total: 0, showSizeChanger: true })

const columns = [
  { title: 'request_id', dataIndex: ['core', 'request_id'], key: 'request_id' },
  { title: 'trace_id', dataIndex: ['core', 'trace_id'], key: 'trace_id' },
  { title: '操作类型', key: 'ops_action' },
  { title: '操作人', dataIndex: 'who', key: 'who' },
  { title: '时间', dataIndex: 'when', key: 'when' },
  { title: '结果', key: 'final_result' },
  { title: '操作', key: 'actionBtn' }
]

const buildQuery = (): OpsAuditFilters => ({
  requestId: filters.requestId.trim() || undefined,
  traceId: filters.traceId.trim() || undefined,
  rule: filters.rule.trim() || undefined,
  errorOnly: filters.errorOnly || undefined,
  startTime: filters.startTime,
  endTime: filters.endTime
})

const loadLogs = async (page = pagination.value.current, pageSize = pagination.value.pageSize) => {
  const result = await gatewayApi.fetchAudits(page, pageSize, buildQuery())
  logs.value = result.list
  pagination.value = { ...pagination.value, current: page, pageSize, total: result.total }
}

const onTableChange = (pager: { current?: number; pageSize?: number }) => {
  loadLogs(pager.current ?? 1, pager.pageSize ?? 8)
}

const onRangeChange = (value: [string, string] | null) => {
  filters.startTime = value?.[0]
  filters.endTime = value?.[1]
}

const openDetail = (record: OpsAuditEvent) => {
  selectedLog.value = record
  drawerVisible.value = true
}

loadLogs(1)
</script>
