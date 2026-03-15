<template>
  <a-space direction="vertical" style="width:100%">
    <a-page-header title="授权与安全" />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading">
      <a-empty v-if="!state" description="暂无安全态" />
      <a-descriptions v-else bordered :column="1">
        <a-descriptions-item label="licenseStatus">{{ state.licenseStatus }}</a-descriptions-item>
        <a-descriptions-item label="expiryTime">{{ state.expiryTime }}</a-descriptions-item>
        <a-descriptions-item label="licensedFeatures">{{ state.licensedFeatures.join(', ') }}</a-descriptions-item>
        <a-descriptions-item label="lastValidation">{{ state.lastValidation }}</a-descriptions-item>
        <a-descriptions-item label="managementSecurity">{{ state.managementSecurity }}</a-descriptions-item>
        <a-descriptions-item label="signingAlgorithm">{{ state.signingAlgorithm }}</a-descriptions-item>
      </a-descriptions>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { SecurityCenterState } from '../types/gateway'

const loading = ref(false)
const error = ref('')
const state = ref<SecurityCenterState>()

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    state.value = await gatewayApi.fetchSecurityState()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}
onMounted(load)
</script>
