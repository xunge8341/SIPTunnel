<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-card :bordered="false">
      <a-page-header title="访问日志" sub-title="日志工作台：先筛选定位，再下钻查看请求上下文。" />
      <a-row :gutter="[12, 12]">
        <a-col v-for="item in summary" :key="item.title" :xs="24" :md="12" :xl="6">
          <a-card size="small">
            <a-statistic :title="item.title" :value="item.value" />
            <a-typography-text type="secondary">{{ item.hint }}</a-typography-text>
          </a-card>
        </a-col>
      </a-row>
      <a-form layout="inline" style="margin-top: 16px">
        <a-form-item label="映射"><a-select v-model:value="filters.mapping" :options="mappingOptions" style="width: 180px" /></a-form-item>
        <a-form-item label="来源 IP"><a-input v-model:value="filters.ip" allow-clear /></a-form-item>
        <a-form-item label="状态"><a-select v-model:value="filters.status" :options="statusOptions" style="width: 120px" /></a-form-item>
        <a-form-item label="方法"><a-select v-model:value="filters.method" :options="methodOptions" style="width: 120px" /></a-form-item>
        <a-form-item label="时间范围"><a-range-picker v-model:value="filters.range" /></a-form-item>
        <a-form-item label="失败原因"><a-input v-model:value="filters.reason" allow-clear /></a-form-item>
        <a-form-item><a-checkbox v-model:checked="filters.onlySlow">仅慢请求</a-checkbox></a-form-item>
      </a-form>
    </a-card>

    <a-card :bordered="false">
      <a-table :columns="columns" :data-source="filteredRows" row-key="time">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="record.status >= 400 ? 'red' : 'green'">{{ record.status }}</a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button type="link" @click="openDetail(record)">查看详情</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="detailOpen" title="日志详情" :width="680">
      <a-descriptions :column="1" bordered>
        <a-descriptions-item label="时间">{{ current?.time }}</a-descriptions-item>
        <a-descriptions-item label="映射名称">{{ current?.mapping }}</a-descriptions-item>
        <a-descriptions-item label="request_id">req-20260315-{{ current?.id }}</a-descriptions-item>
        <a-descriptions-item label="trace_id">trace-4f2e-{{ current?.id }}</a-descriptions-item>
        <a-descriptions-item label="本端入口">{{ current?.path }}</a-descriptions-item>
        <a-descriptions-item label="对端目标">{{ current?.target }}</a-descriptions-item>
        <a-descriptions-item label="原始请求/响应摘要">请求体 12KB，响应体 18KB，头字段 14 个。</a-descriptions-item>
        <a-descriptions-item label="关联诊断信息">对应 10:32 节点连通性测试通过，10:35 发生瞬时超时。</a-descriptions-item>
        <a-descriptions-item label="关联调试字段">重试 1 次；降级策略未触发；限流未命中。</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'

type LogRow = {
  id: number
  time: string
  mapping: string
  ip: string
  method: string
  path: string
  target: string
  status: number
  latency: string
  reason: string
}

const summary = [
  { title: '近 1 小时访问量', value: 4280, hint: '较上一小时 +4.5%' },
  { title: '近 1 小时失败量', value: 96, hint: '主要集中在支付回调' },
  { title: '慢请求数', value: 43, hint: '阈值：>500ms' },
  { title: '主要异常类型', value: '目标超时', hint: '占失败 62%' }
]

const columns = [
  { title: '时间', dataIndex: 'time', key: 'time' },
  { title: '映射名称', dataIndex: 'mapping', key: 'mapping' },
  { title: '来源 IP', dataIndex: 'ip', key: 'ip' },
  { title: '方法', dataIndex: 'method', key: 'method' },
  { title: '路径', dataIndex: 'path', key: 'path' },
  { title: '状态', dataIndex: 'status', key: 'status' },
  { title: '耗时', dataIndex: 'latency', key: 'latency' },
  { title: '失败原因摘要', dataIndex: 'reason', key: 'reason' },
  { title: '操作', key: 'action' }
]

const rows = ref<LogRow[]>([
  { id: 101, time: '2026-03-15 11:32:11', mapping: '支付回调', ip: '10.2.8.9', method: 'POST', path: '/api/payment/callback', target: 'http://10.9.2.31/callback', status: 504, latency: '812ms', reason: '对端响应超时' },
  { id: 102, time: '2026-03-15 11:31:24', mapping: '订单同步', ip: '10.2.8.21', method: 'POST', path: '/api/order/sync', target: 'http://10.9.2.20/sync', status: 200, latency: '146ms', reason: '-' },
  { id: 103, time: '2026-03-15 11:31:05', mapping: '账单归档', ip: '10.2.8.44', method: 'PUT', path: '/api/bill/archive', target: 'http://10.9.2.45/archive', status: 500, latency: '521ms', reason: '对端返回 500' }
])

const filters = reactive({ mapping: 'all', ip: '', status: 'all', method: 'all', range: [] as string[], reason: '', onlySlow: false })
const mappingOptions = [{ label: '全部', value: 'all' }, { label: '订单同步', value: '订单同步' }, { label: '支付回调', value: '支付回调' }, { label: '账单归档', value: '账单归档' }]
const statusOptions = [{ label: '全部', value: 'all' }, { label: '成功(2xx)', value: 'success' }, { label: '失败(4xx/5xx)', value: 'fail' }]
const methodOptions = [{ label: '全部', value: 'all' }, { label: 'GET', value: 'GET' }, { label: 'POST', value: 'POST' }, { label: 'PUT', value: 'PUT' }]

const filteredRows = computed(() => rows.value.filter((row) => {
  const byMap = filters.mapping === 'all' || row.mapping === filters.mapping
  const byIp = !filters.ip || row.ip.includes(filters.ip)
  const byStatus = filters.status === 'all' || (filters.status === 'success' ? row.status < 400 : row.status >= 400)
  const byMethod = filters.method === 'all' || row.method === filters.method
  const byReason = !filters.reason || row.reason.includes(filters.reason)
  const bySlow = !filters.onlySlow || Number.parseInt(row.latency, 10) > 500
  return byMap && byIp && byStatus && byMethod && byReason && bySlow
}))

const detailOpen = ref(false)
const current = ref<LogRow>()
const openDetail = (row: LogRow) => {
  current.value = row
  detailOpen.value = true
}
</script>
