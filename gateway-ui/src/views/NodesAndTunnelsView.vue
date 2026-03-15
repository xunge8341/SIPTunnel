<template>
  <a-space direction="vertical" style="width:100%">
    <a-page-header title="节点与隧道" />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-spin :spinning="loading || saving">
      <a-empty v-if="!workspace" description="暂无工作区" />
      <template v-else>
        <a-descriptions bordered :column="1">
          <a-descriptions-item label="networkMode">{{ workspace.networkMode }}</a-descriptions-item>
          <a-descriptions-item label="localNode">{{ workspace.localNode.device_id }} / {{ workspace.localNode.node_ip }}</a-descriptions-item>
          <a-descriptions-item label="peerNode">{{ workspace.peerNode.device_id }} / {{ workspace.peerNode.node_ip }}</a-descriptions-item>
        </a-descriptions>
        <a-form layout="vertical" style="margin-top: 12px">
          <a-form-item label="signingAlgorithm"><a-input v-model:value="workspace.securitySettings.signer" /></a-form-item>
          <a-form-item label="encryption"><a-input v-model:value="workspace.securitySettings.encryption" /></a-form-item>
          <a-space><a-button @click="load">refresh</a-button><a-button type="primary" @click="save">save</a-button></a-space>
        </a-form>
      </template>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { NodeTunnelWorkspace } from '../types/gateway'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const workspace = ref<NodeTunnelWorkspace>()

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    workspace.value = await gatewayApi.fetchNodeTunnelWorkspace()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载失败'
  } finally {
    loading.value = false
  }
}

const save = async () => {
  if (!workspace.value) return
  saving.value = true
  error.value = ''
  try {
    await gatewayApi.saveNodeTunnelWorkspace(workspace.value)
    workspace.value = await gatewayApi.fetchNodeTunnelWorkspace()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存失败'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
