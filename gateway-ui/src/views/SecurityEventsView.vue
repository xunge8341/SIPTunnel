
<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="安全事件" sub-title="查看 SIP/RTP 协议拒绝、重放、摘要校验与签名失败事件；该视图与统一审计查询口径联动。">
      <template #extra>
        <a-button @click="load">刷新</a-button>
      </template>
    </a-page-header>
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading">
      <a-table :data-source="rows" :columns="columns" row-key="key" :pagination="false" :locale="{ emptyText: '暂无安全事件' }">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'when'">
            <span>{{ formatDateTimeText(record.when) }}</span>
          </template>
          <template v-else-if="column.key === 'transport'">
            <a-tag :color="record.transport === 'RTP' ? 'orange' : 'blue'">{{ record.transport }}</a-tag>
          </template>
          <template v-else-if="column.key === 'auditLinked'">
            <a-tag :color="record.auditLinked ? 'green' : 'default'">{{ record.auditLinked ? '已纳入审计' : '仅安全事件' }}</a-tag>
          </template>
        </template>
      </a-table>
    </a-spin>
  </a-space>
</template>
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import { formatDateTimeText } from '../utils/date'
import type { SecurityEventRecord } from '../types/gateway'
const loading = ref(false)
const error = ref('')
const rows = ref<(SecurityEventRecord & { key: string })[]>([])
const columns = [
  { title: '时间', dataIndex: 'when', key: 'when' },
  { title: '类别', dataIndex: 'category', key: 'category' },
  { title: '传输', dataIndex: 'transport', key: 'transport' },
  { title: '原因', dataIndex: 'reason', key: 'reason', ellipsis: true },
  { title: 'request_id', dataIndex: 'requestId', key: 'requestId', ellipsis: true },
  { title: 'trace_id', dataIndex: 'traceId', key: 'traceId', ellipsis: true },
  { title: '审计联动', dataIndex: 'auditLinked', key: 'auditLinked' }
]
const load = async () => {
  loading.value = true
  error.value = ''
  try {
    const items = await gatewayApi.fetchSecurityEvents()
    rows.value = items.map((item, index) => ({ ...item, key: `${item.when}-${index}` }))
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载安全事件失败'
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>
