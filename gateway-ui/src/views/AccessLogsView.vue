<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="访问日志" />
    <a-form layout="inline">
      <a-form-item label="mapping"><a-input v-model:value="query.mapping" /></a-form-item>
      <a-form-item label="sourceIP"><a-input v-model:value="query.sourceIP" /></a-form-item>
      <a-form-item label="method"><a-input v-model:value="query.method" /></a-form-item>
      <a-form-item><a-checkbox v-model:checked="query.slowOnly">slowOnly</a-checkbox></a-form-item>
      <a-form-item><a-button @click="load">查询</a-button></a-form-item>
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
import type { AccessLogEntry, AccessLogQuery } from '../types/gateway'

const loading = ref(false)
const error = ref('')
const list = ref<AccessLogEntry[]>([])
const query = reactive<AccessLogQuery>({ slowOnly: false })
const columns = [
  { title: 'timestamp', dataIndex: 'occurred_at' },
  { title: 'mappingName', dataIndex: 'mapping_name' },
  { title: 'sourceIP', dataIndex: 'source_ip' },
  { title: 'method', dataIndex: 'method' },
  { title: 'path', dataIndex: 'path' },
  { title: 'status', dataIndex: 'status_code' },
  { title: 'latency', dataIndex: 'duration_ms' },
  { title: 'failureReason', dataIndex: 'failure_reason' },
  { title: 'requestId', dataIndex: 'request_id' },
  { title: 'traceId', dataIndex: 'trace_id' }
]

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    const data = await gatewayApi.fetchAccessLogs(query, 1, 50)
    list.value = data.list
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
