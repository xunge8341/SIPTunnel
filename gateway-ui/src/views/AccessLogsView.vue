<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="访问日志" />
    <a-form layout="inline">
      <a-form-item label="映射名称"><a-input v-model:value="query.mapping" /></a-form-item>
      <a-form-item label="来源 IP"><a-input v-model:value="query.sourceIP" /></a-form-item>
      <a-form-item label="请求方法"><a-input v-model:value="query.method" /></a-form-item>
      <a-form-item><a-checkbox v-model:checked="query.slowOnly">仅慢请求</a-checkbox></a-form-item>
      <a-form-item><a-button @click="refresh">查询</a-button></a-form-item>
    </a-form>
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading">
      <a-empty v-if="!loading && list.length === 0" description="暂无日志" />
      <a-table v-else :data-source="list" :columns="columns" row-key="id" />
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import { ACCESS_LOGS_CONTRACT } from '../contracts/accessLogs'
import type { AccessLogEntry, AccessLogQuery } from '../types/gateway'

const loading = ref(false)
const error = ref('')
const list = ref<AccessLogEntry[]>([])
const query = reactive<AccessLogQuery>({ ...ACCESS_LOGS_CONTRACT.request })
const columns = [
  { title: '时间戳', dataIndex: 'occurred_at' },
  { title: '映射名称', dataIndex: 'mapping_name' },
  { title: '来源 IP', dataIndex: 'source_ip' },
  { title: '请求方法', dataIndex: 'method' },
  { title: '请求路径', dataIndex: 'path' },
  { title: '状态码', dataIndex: 'status_code' },
  { title: '时延(ms)', dataIndex: 'duration_ms' },
  { title: '失败原因', dataIndex: 'failure_reason' },
  { title: '请求 ID', dataIndex: 'request_id' },
  { title: '追踪 ID', dataIndex: 'trace_id' }
]

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    const data = await gatewayApi.fetchAccessLogs(query, ACCESS_LOGS_CONTRACT.response.page, ACCESS_LOGS_CONTRACT.response.pageSize)
    list.value = data.list
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

const refresh = async () => {
  await load()
}

onMounted(load)
</script>
