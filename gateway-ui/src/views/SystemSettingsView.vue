<template>
  <a-space direction="vertical" style="width:100%">
    <a-page-header title="系统设置" />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading || saving">
      <a-empty v-if="!form" description="暂无设置" />
      <a-form v-else layout="vertical">
        <a-form-item label="sqlitePath"><a-input v-model:value="form.sqlitePath" /></a-form-item>
        <a-form-item label="cleanupCron"><a-input v-model:value="form.cleanupCron" /></a-form-item>
        <a-form-item label="adminCIDR"><a-input v-model:value="form.adminCIDR" /></a-form-item>
        <a-form-item label="mfaEnabled"><a-switch v-model:checked="form.mfaEnabled" /></a-form-item>
        <a-space>
          <a-button @click="load">refresh</a-button>
          <a-button type="primary" @click="save">save</a-button>
        </a-space>
      </a-form>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { SystemSettingsState } from '../types/gateway'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const form = ref<SystemSettingsState>()

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    form.value = await gatewayApi.fetchSystemSettings()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

const save = async () => {
  if (!form.value) return
  saving.value = true
  error.value = ''
  try {
    form.value = await gatewayApi.updateSystemSettings(form.value)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
