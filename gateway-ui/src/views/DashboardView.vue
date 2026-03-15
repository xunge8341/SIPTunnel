<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="总览监控" sub-title="Contract-First：摘要与聚合均来自后端统一接口。" />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading">
      <a-empty v-if="!summary" description="暂无摘要数据" />
      <template v-else>
        <a-row :gutter="[16, 16]">
          <a-col :xs="24" :md="8" v-for="card in cards" :key="card.title">
            <a-card>
              <a-statistic :title="card.title" :value="card.value" />
            </a-card>
          </a-col>
        </a-row>
        <a-card title="热点与风险 TopN" style="margin-top: 12px">
          <a-row :gutter="16">
            <a-col :span="12"><a-list :data-source="ops?.hotMappings || []" size="small"><template #renderItem="{ item }"><a-list-item>{{ item.name }} / {{ item.count }}</a-list-item></template></a-list></a-col>
            <a-col :span="12"><a-list :data-source="ops?.topFailureMappings || []" size="small"><template #renderItem="{ item }"><a-list-item>{{ item.name }} / {{ item.count }}</a-list-item></template></a-list></a-col>
          </a-row>
        </a-card>
      </template>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { DashboardOpsSummary, DashboardSummary } from '../types/gateway'

const loading = ref(false)
const error = ref('')
const summary = ref<DashboardSummary>()
const ops = ref<DashboardOpsSummary>()

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    const [s, o] = await Promise.all([gatewayApi.fetchDashboardSummary(), gatewayApi.fetchDashboardOpsSummary()])
    summary.value = s
    ops.value = o
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

const cards = computed(() => {
  if (!summary.value) return []
  return [
    { title: 'systemHealth', value: summary.value.systemHealth },
    { title: 'activeConnections', value: summary.value.activeConnections },
    { title: 'mappingTotal', value: summary.value.mappingTotal },
    { title: 'mappingErrorCount', value: summary.value.mappingErrorCount },
    { title: 'recentFailureCount', value: summary.value.recentFailureCount },
    { title: 'rateLimitState', value: summary.value.rateLimitState },
    { title: 'circuitBreakerState', value: summary.value.circuitBreakerState }
  ]
})

onMounted(load)
</script>
