<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="访问日志" sub-title="基于真实请求链路查询访问记录，并与总览监控和告警保护共用同一分析口径。">
      <template #extra>
        <a-space>
          <a-tag color="blue">统计口径：{{ summaryWindow }}</a-tag>
          <a-button @click="load">刷新</a-button>
        </a-space>
      </template>
    </a-page-header>

    <a-row :gutter="[16, 16]">
      <a-col v-for="card in summaryCards" :key="card.title" :xs="24" :sm="12" :xl="6">
        <a-card :bordered="false" class="metric-card"><a-statistic :title="card.title" :value="card.value" /></a-card>
      </a-col>
    </a-row>

    <a-card :bordered="false">
      <a-form layout="inline" class="filter-form">
        <a-form-item label="映射名称"><a-input v-model:value="query.mapping" allow-clear /></a-form-item>
        <a-form-item label="来源 IP"><a-input v-model:value="query.sourceIP" allow-clear /></a-form-item>
        <a-form-item label="请求方法"><a-input v-model:value="query.method" allow-clear /></a-form-item>
        <a-form-item label="状态码"><a-input-number v-model:value="query.status" :min="100" :max="599" style="width: 140px" /></a-form-item>
        <a-form-item label="时间范围"><a-range-picker v-model:value="timeRange" show-time style="width: 320px" /></a-form-item>
        <a-form-item><a-checkbox v-model:checked="query.slowOnly">仅慢请求</a-checkbox></a-form-item>
        <a-form-item><a-checkbox v-model:checked="query.failedOnly">仅失败请求</a-checkbox></a-form-item>
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
      <a-table
        :data-source="list"
        :columns="columns"
        row-key="id"
        :pagination="false"
        :locale="{ emptyText: '暂无访问日志' }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status_code'">
            <a-tag :color="record.status_code >= 500 ? 'red' : record.status_code >= 400 ? 'orange' : 'green'">{{ record.status_code }}</a-tag>
          </template>
          <template v-else-if="column.key === 'occurred_at'">
            <span>{{ formatDateTimeText(record.occurred_at) }}</span>
          </template>
          <template v-else-if="column.key === 'duration_ms'">
            <span>{{ record.duration_ms }} ms</span>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button type="link" @click="openDetail(record)">查看详情</a-button>
          </template>
        </template>
      </a-table>
    </a-spin>

    <a-drawer v-model:open="detailOpen" title="访问日志详情" :width="720">
      <a-descriptions v-if="current" :column="1" bordered size="small">
        <a-descriptions-item label="时间">{{ formatDateTimeText(current.occurred_at) }}</a-descriptions-item>
        <a-descriptions-item label="映射名称">{{ current.mapping_name }}</a-descriptions-item>
        <a-descriptions-item label="来源 IP">{{ current.source_ip }}</a-descriptions-item>
        <a-descriptions-item label="请求方法">{{ current.method }}</a-descriptions-item>
        <a-descriptions-item label="请求路径">{{ current.path }}</a-descriptions-item>
        <a-descriptions-item label="状态码">{{ current.status_code }}</a-descriptions-item>
        <a-descriptions-item label="耗时">{{ current.duration_ms }} ms</a-descriptions-item>
        <a-descriptions-item label="失败原因">{{ current.failure_reason || '无' }}</a-descriptions-item>
        <a-descriptions-item label="request_id">{{ current.request_id }}</a-descriptions-item>
        <a-descriptions-item label="trace_id">{{ current.trace_id }}</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import dayjs, { type Dayjs } from 'dayjs'
import { gatewayApi } from '../api/gateway'
import { formatDateTimeText } from '../utils/date'
import type { AccessLogEntry, AccessLogQuery, AccessLogSummary } from '../types/gateway'

const loading = ref(false)
const error = ref('')
const list = ref<AccessLogEntry[]>([])
const summary = ref<AccessLogSummary>({ total: 0, failed: 0, slow: 0, error_types: {}, window: '当前筛选条件' })
const current = ref<AccessLogEntry>()
const detailOpen = ref(false)
const timeRange = ref<[Dayjs, Dayjs] | null>(null)

const defaultQuery: AccessLogQuery = {
  mapping: undefined,
  sourceIP: undefined,
  method: undefined,
  status: undefined,
  startTime: undefined,
  endTime: undefined,
  slowOnly: false,
  failedOnly: false
}
const query = reactive<AccessLogQuery>({ ...defaultQuery })

watch(timeRange, (range) => {
  query.startTime = range?.[0] ? range[0].toISOString() : undefined
  query.endTime = range?.[1] ? range[1].toISOString() : undefined
})

const columns = [
  { title: '时间', dataIndex: 'occurred_at', key: 'occurred_at' },
  { title: '映射名称', dataIndex: 'mapping_name', key: 'mapping_name' },
  { title: '来源 IP', dataIndex: 'source_ip', key: 'source_ip' },
  { title: '请求方法', dataIndex: 'method', key: 'method' },
  { title: '请求路径', dataIndex: 'path', key: 'path' },
  { title: '状态码', dataIndex: 'status_code', key: 'status_code' },
  { title: '耗时', dataIndex: 'duration_ms', key: 'duration_ms' },
  { title: '失败原因', dataIndex: 'failure_reason', key: 'failure_reason', ellipsis: true },
  { title: '操作', key: 'action', width: 100 }
]

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    const data = await gatewayApi.fetchAccessLogs(query, 1, 100)
    list.value = data.list
    summary.value = data.summary
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载访问日志失败'
  } finally {
    loading.value = false
  }
}

const reset = () => {
  Object.assign(query, defaultQuery)
  timeRange.value = [dayjs().subtract(1, 'hour'), dayjs()]
  void load()
}

const openDetail = (record: AccessLogEntry) => {
  current.value = record
  detailOpen.value = true
}

const summaryCards = computed(() => {
  const errorKinds = Object.keys(summary.value.error_types || {}).length
  return [
    { title: '命中记录数', value: summary.value.total },
    { title: '失败请求', value: summary.value.failed },
    { title: '慢请求', value: summary.value.slow },
    { title: '异常类型数', value: errorKinds }
  ]
})

const summaryWindow = computed(() => summary.value.window || '当前筛选条件')

onMounted(() => {
  timeRange.value = [dayjs().subtract(1, 'hour'), dayjs()]
  void load()
})
</script>

<style scoped>
.filter-form {
  row-gap: 8px;
}
.metric-card {
  height: 100%;
}
</style>

