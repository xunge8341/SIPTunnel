<template>
  <a-space direction="vertical" style="width:100%">
    <a-page-header title="隧道映射" />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading">
      <a-empty v-if="!loading && rows.length === 0" description="暂无映射" />
      <a-table v-else :data-source="rows" :columns="columns" row-key="mappingId" />
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { MappingWorkspaceItem } from '../types/gateway'

const loading = ref(false)
const error = ref('')
const rows = ref<MappingWorkspaceItem[]>([])

const columns = [
  { title: 'mappingName', dataIndex: 'mappingName' },
  { title: 'localEntry', dataIndex: 'localEntry' },
  { title: 'peerTarget', dataIndex: 'peerTarget' },
  { title: 'status', dataIndex: 'status' },
  { title: 'lastTestResult', dataIndex: 'lastTestResult' },
  { title: 'requestCount', dataIndex: 'requestCount' },
  { title: 'failureCount', dataIndex: 'failureCount' },
  { title: 'avgLatency', dataIndex: 'avgLatency' },
  { title: 'riskLevel', dataIndex: 'riskLevel' }
]

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    rows.value = (await gatewayApi.fetchMappingWorkspaceList()).items
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
