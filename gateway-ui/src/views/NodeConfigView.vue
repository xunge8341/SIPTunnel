<template>
  <a-card title="节点配置">
    <a-form layout="vertical">
      <a-alert type="info" show-icon message="节点配置按角色拆分：本端维护接收端监听参数，对端维护发送端接入参数。" style="margin-bottom: 12px" />
      <a-divider orientation="left">接收端（SIP下级域 / 本端节点）</a-divider>
      <a-form-item label="节点IP"><a-input v-model:value="draft.local_node.node_ip" /></a-form-item>
      <a-form-item label="信令端口"><a-input-number v-model:value="draft.local_node.signaling_port" :min="1" :max="65535" style="width: 100%" /></a-form-item>
      <a-form-item label="设备编号"><a-input v-model:value="draft.local_node.device_id" /></a-form-item>
      <a-form-item label="RTP起始端口"><a-input-number v-model:value="draft.local_node.rtp_port_start" :min="1" :max="65535" style="width: 100%" /></a-form-item>
      <a-form-item label="RTP结束端口"><a-input-number v-model:value="draft.local_node.rtp_port_end" :min="1" :max="65535" style="width: 100%" /></a-form-item>

      <a-divider orientation="left">发送端（SIP上级域 / 对端节点）</a-divider>
      <a-form-item label="节点IP"><a-input v-model:value="draft.peer_node.node_ip" /></a-form-item>
      <a-form-item label="信令端口"><a-input-number v-model:value="draft.peer_node.signaling_port" :min="1" :max="65535" style="width: 100%" /></a-form-item>
      <a-form-item label="设备编号"><a-input v-model:value="draft.peer_node.device_id" /></a-form-item>
    </a-form>
    <a-space>
      <a-button @click="load">重载</a-button>
      <a-button type="primary" :loading="saving" @click="save">保存并重启通道</a-button>
    </a-space>
  </a-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { NodeConfigPayload } from '../types/gateway'

const saving = ref(false)
const draft = reactive<NodeConfigPayload>({
  local_node: { node_ip: '', signaling_port: 5060, device_id: '', rtp_port_start: 20000, rtp_port_end: 20999 },
  peer_node: { node_ip: '', signaling_port: 5060, device_id: '' }
})

const load = async () => {
  const data = await gatewayApi.fetchNodeConfig()
  Object.assign(draft.local_node, data.local_node)
  Object.assign(draft.peer_node, data.peer_node)
}

const save = async () => {
  saving.value = true
  try {
    const result = await gatewayApi.saveNodeConfig(JSON.parse(JSON.stringify(draft)))
    if (result.tunnel_restarted) {
      message.success('节点配置已保存并重启通道')
    }
    await load()
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
