<template>
  <a-space direction="vertical" style="width: 100%">
    <a-card title="系统设置">
      <a-typography-paragraph type="secondary">系统级持久化、保留和清理规则统一入口。保存后立即回读。</a-typography-paragraph>
      <a-form layout="vertical">
        <a-divider orientation="left">SQLite 持久化</a-divider>
        <a-form-item label="SQLite 文件路径"><a-input v-model:value="draft.sqlite_path" /></a-form-item>

        <a-divider orientation="left">清理与保留规则</a-divider>
        <a-form-item label="日志清理计划(cron)"><a-input v-model:value="draft.log_cleanup_cron" /></a-form-item>
        <a-row :gutter="12">
          <a-col :span="8"><a-form-item label="数据库保留天数"><a-input-number v-model:value="draft.max_task_age_days" :min="1" style="width:100%" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="数据库保留条数"><a-input-number v-model:value="draft.max_task_records" :min="100" style="width:100%" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="访问日志保留天数"><a-input-number v-model:value="draft.max_access_log_age_days" :min="1" style="width:100%" /></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="8"><a-form-item label="访问日志保留条数"><a-input-number v-model:value="draft.max_access_log_records" :min="100" style="width:100%" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="运维审计保留天数"><a-input-number v-model:value="draft.max_audit_age_days" :min="1" style="width:100%" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="运维审计保留条数"><a-input-number v-model:value="draft.max_audit_records" :min="100" style="width:100%" /></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="8"><a-form-item label="诊断保留天数"><a-input-number v-model:value="draft.max_diagnostic_age_days" :min="1" style="width:100%" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="诊断保留条数"><a-input-number v-model:value="draft.max_diagnostic_records" :min="100" style="width:100%" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="压测保留天数"><a-input-number v-model:value="draft.max_loadtest_age_days" :min="1" style="width:100%" /></a-form-item></a-col>
        </a-row>
        <a-form-item label="压测保留条数"><a-input-number v-model:value="draft.max_loadtest_records" :min="100" style="width:100%" /></a-form-item>

        <a-divider orientation="left">管理面安全</a-divider>
        <a-form-item label="管理面允许网段"><a-input v-model:value="draft.admin_allow_cidr" /></a-form-item>
        <a-form-item label="管理面强制 MFA"><a-switch v-model:checked="draft.admin_require_mfa" /></a-form-item>

        <a-alert type="info" show-icon :message="`最近清理执行：${draft.cleaner_last_run_at || '-'} / ${draft.cleaner_last_result || '-'} / 清理记录 ${draft.cleaner_last_removed_records || 0}`" style="margin-bottom:12px" />
        <a-space>
          <a-button @click="load">刷新回读</a-button>
          <a-button type="primary" :loading="saving" @click="save">保存系统设置</a-button>
        </a-space>
      </a-form>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { SystemSettingsPayload } from '../types/gateway'

const saving = ref(false)
const draft = reactive<SystemSettingsPayload>({
  sqlite_path: '', log_cleanup_cron: '*/30 * * * *', max_task_age_days: 7, max_task_records: 20000,
  max_access_log_age_days: 7, max_access_log_records: 20000, max_audit_age_days: 30, max_audit_records: 50000,
  max_diagnostic_age_days: 15, max_diagnostic_records: 2000, max_loadtest_age_days: 15, max_loadtest_records: 2000,
  admin_allow_cidr: '127.0.0.1/32', admin_require_mfa: false, cleaner_last_run_at: '', cleaner_last_result: '', cleaner_last_removed_records: 0
})

const load = async () => Object.assign(draft, await gatewayApi.fetchSystemSettings())
const save = async () => {
  saving.value = true
  try {
    await gatewayApi.updateSystemSettings({ ...draft })
    message.success('系统设置已保存')
    await load()
  } finally { saving.value = false }
}
onMounted(load)
</script>
