<template>
  <a-space direction="vertical" style="width: 100%" size="large">
    <a-page-header title="运维审计" sub-title="聚焦谁在什么时候改了什么，以及执行结果。" />

    <a-card :bordered="false">
      <a-form layout="inline">
        <a-form-item label="请求 ID"><a-input v-model:value="filters.requestId" allow-clear /></a-form-item>
        <a-form-item label="追踪 ID"><a-input v-model:value="filters.traceId" allow-clear /></a-form-item>
        <a-form-item label="仅失败"><a-switch v-model:checked="filters.errorOnly" /></a-form-item>
        <a-form-item>
          <a-space>
            <a-button type="primary" @click="load">查询</a-button>
            <a-button @click="reset">重置</a-button>
          </a-space>
        </a-form-item>
      </a-form>
    </a-card>

    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading">
      <a-empty v-if="!loading && rows.length === 0" description="暂无审计记录" />
      <a-table v-else :columns="columns" :data-source="rows" row-key="id" :pagination="false">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'time'">
            <span>{{ formatDateTimeText(record.time) }}</span>
          </template>
          <template v-else-if="column.key === 'result'">
            <a-tag :color="record.result === '成功' ? 'green' : 'red'">{{ record.result }}</a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button type="link" @click="openDetail(record.raw)">查看详情</a-button>
          </template>
        </template>
      </a-table>
    </a-spin>

    <a-drawer v-model:open="detailOpen" title="审计详情" :width="720">
      <a-descriptions v-if="current" bordered :column="1" size="small">
        <a-descriptions-item label="时间">{{ formatDateTimeText(current.when) }}</a-descriptions-item>
        <a-descriptions-item label="操作类型">{{ current.request_type }}</a-descriptions-item>
        <a-descriptions-item label="操作对象">{{ current.local_service_route || '系统配置' }}</a-descriptions-item>
        <a-descriptions-item label="操作人">{{ current.who }}</a-descriptions-item>
        <a-descriptions-item label="结果">{{ current.final_result }}</a-descriptions-item>
        <a-descriptions-item label="request_id">{{ current.core.request_id }}</a-descriptions-item>
        <a-descriptions-item label="trace_id">{{ current.core.trace_id }}</a-descriptions-item>
        <a-descriptions-item label="来源系统">{{ current.core.source_system }}</a-descriptions-item>
        <a-descriptions-item label="来源节点">{{ current.core.source_node }}</a-descriptions-item>
        <a-descriptions-item label="结果码">{{ current.core.result_code }}</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import { formatDateTimeText } from '../utils/date'
import type { OpsAuditEvent, OpsAuditFilters } from '../types/gateway'

interface AuditRow {
  id: string
  time: string
  type: string
  target: string
  operator: string
  result: '成功' | '失败'
  summary: string
  raw: OpsAuditEvent
}

const loading = ref(false)
const error = ref('')
const rows = ref<AuditRow[]>([])
const detailOpen = ref(false)
const current = ref<OpsAuditEvent>()
const filters = reactive<OpsAuditFilters>({ requestId: '', traceId: '', errorOnly: false })

const columns = [
  { title: '时间', dataIndex: 'time', key: 'time' },
  { title: '操作类型', dataIndex: 'type', key: 'type' },
  { title: '操作对象', dataIndex: 'target', key: 'target' },
  { title: '操作人', dataIndex: 'operator', key: 'operator' },
  { title: '结果', dataIndex: 'result', key: 'result' },
  { title: '摘要', dataIndex: 'summary', key: 'summary' },
  { title: '操作', key: 'action', width: 100 }
]

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    const data = await gatewayApi.fetchAudits(1, 100, filters)
    rows.value = data.list.map((item, index) => ({
      id: `${item.when}-${index}`,
      time: item.when,
      type: item.request_type,
      target: item.local_service_route || item.core.api_code || '系统配置',
      operator: item.who,
      result: item.validation_passed ? '成功' : '失败',
      summary: item.ops_action || item.final_result,
      raw: item
    }))
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载运维审计失败'
  } finally {
    loading.value = false
  }
}

const reset = () => {
  filters.requestId = ''
  filters.traceId = ''
  filters.errorOnly = false
  void load()
}

const openDetail = (row: OpsAuditEvent) => {
  current.value = row
  detailOpen.value = true
}

onMounted(load)
</script>
