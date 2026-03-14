<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card>
      <a-space style="width: 100%; justify-content: space-between" align="start">
        <a-space direction="vertical" size="small">
          <a-typography-title :level="5" style="margin: 0">配置导入导出</a-typography-title>
          <a-typography-text type="secondary">支持配置导出 JSON、导入 JSON 与模板下载。</a-typography-text>
        </a-space>
      </a-space>
    </a-card>

    <a-card title="导出配置 JSON">
      <a-space>
        <a-button type="primary" :loading="exporting" @click="handleExport">导出 JSON</a-button>
      </a-space>
      <a-typography-paragraph v-if="lastExportAt" type="secondary" style="margin-top: 12px">
        最近导出时间：{{ lastExportAt }}
      </a-typography-paragraph>
    </a-card>

    <a-card title="导入配置 JSON">
      <a-space direction="vertical" style="width: 100%" size="middle">
        <a-textarea v-model:value="importContent" :rows="10" placeholder="请粘贴配置 JSON 内容" />
        <a-space>
          <a-button type="primary" :loading="importing" @click="handleImport">导入 JSON</a-button>
          <a-button @click="importContent = ''">清空</a-button>
        </a-space>
      </a-space>
    </a-card>

    <a-card title="模板下载">
      <a-button :loading="downloadingTemplate" @click="handleDownloadTemplate">下载模板 JSON</a-button>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { ConfigTransferPayload } from '../types/gateway'

const exporting = ref(false)
const importing = ref(false)
const downloadingTemplate = ref(false)
const importContent = ref('')
const lastExportAt = ref('')

const downloadJson = (fileName: string, payload: ConfigTransferPayload) => {
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = fileName
  link.click()
  URL.revokeObjectURL(url)
}

const handleExport = async () => {
  exporting.value = true
  try {
    const payload = await gatewayApi.exportConfigJson()
    downloadJson(`siptunnel-config-${payload.version}.json`, payload)
    lastExportAt.value = payload.exported_at
    message.success('配置导出成功')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '配置导出失败')
  } finally {
    exporting.value = false
  }
}

const handleImport = async () => {
  importing.value = true
  try {
    const payload = JSON.parse(importContent.value) as ConfigTransferPayload
    const result = await gatewayApi.importConfigJson(payload)
    message.success(result.message)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '配置导入失败，请检查 JSON 格式')
  } finally {
    importing.value = false
  }
}

const handleDownloadTemplate = async () => {
  downloadingTemplate.value = true
  try {
    const payload = await gatewayApi.downloadConfigTemplate()
    downloadJson('siptunnel-config-template.json', payload)
    message.success('模板下载成功')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '模板下载失败')
  } finally {
    downloadingTemplate.value = false
  }
}
</script>
