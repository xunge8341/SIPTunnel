<template>
  <a-space direction="vertical" style="width:100%">
    <a-page-header title="告警与保护" />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading">
      <a-empty v-if="!state" description="暂无保护状态" />
      <a-descriptions v-else bordered :column="1">
        <a-descriptions-item label="alertRules">{{ state.alertRules.join(' | ') }}</a-descriptions-item>
        <a-descriptions-item label="rateLimitRules">{{ state.rateLimitRules.join(' | ') }}</a-descriptions-item>
        <a-descriptions-item label="circuitBreakerRules">{{ state.circuitBreakerRules.join(' | ') }}</a-descriptions-item>
        <a-descriptions-item label="currentTriggered">{{ state.currentTriggered.join(' | ') || '-' }}</a-descriptions-item>
        <a-descriptions-item label="lastTriggeredTime">{{ state.lastTriggeredTime || '-' }}</a-descriptions-item>
        <a-descriptions-item label="lastTriggeredTarget">{{ state.lastTriggeredTarget || '-' }}</a-descriptions-item>
      </a-descriptions>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { AlertProtectionState } from '../types/gateway'

const loading = ref(false)
const error = ref('')
const state = ref<AlertProtectionState>()

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    state.value = await gatewayApi.fetchProtectionState()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>
