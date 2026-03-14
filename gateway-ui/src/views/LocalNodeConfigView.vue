<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="本端节点配置">
      <a-alert
        type="info"
        show-icon
        message="全局网络模式决定 transport 承载策略"
        description="映射页只配置本端入口和对端目标，不承载 NetworkMode/Capability 的主配置。"
        style="margin-bottom: 12px"
      />
      <a-descriptions bordered :column="2" size="small" style="margin-bottom: 16px">
        <a-descriptions-item label="当前 NetworkMode">{{ networkStatus?.current_network_mode ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="Capability 摘要">{{ capabilitySummaryText }}</a-descriptions-item>
      </a-descriptions>
      <a-form layout="vertical">
        <a-form-item label="node_id"><a-input v-model:value="draft.node_id" /></a-form-item>
        <a-form-item label="node_name"><a-input v-model:value="draft.node_name" /></a-form-item>
        <a-form-item label="node_role"><a-input v-model:value="draft.node_role" /></a-form-item>
        <a-form-item label="network_mode">
          <a-input v-model:value="draft.network_mode" />
        </a-form-item>

        <a-divider orientation="left">SIP 配置</a-divider>
        <a-form-item label="sip_listen_ip"><a-input v-model:value="draft.sip_listen_ip" /></a-form-item>
        <a-form-item label="sip_listen_port"><a-input-number v-model:value="draft.sip_listen_port" :min="1" :max="65535" style="width: 100%" /></a-form-item>
        <a-form-item label="sip_transport">
          <a-radio-group v-model:value="draft.sip_transport" button-style="solid">
            <a-radio-button value="UDP">UDP</a-radio-button>
            <a-radio-button value="TCP">TCP</a-radio-button>
          </a-radio-group>
        </a-form-item>

        <a-divider orientation="left">RTP 配置</a-divider>
        <a-form-item label="rtp_listen_ip"><a-input v-model:value="draft.rtp_listen_ip" /></a-form-item>
        <a-form-item label="rtp_port_start"><a-input-number v-model:value="draft.rtp_port_start" :min="1" :max="65535" style="width: 100%" /></a-form-item>
        <a-form-item label="rtp_port_end"><a-input-number v-model:value="draft.rtp_port_end" :min="1" :max="65535" style="width: 100%" /></a-form-item>
        <a-form-item label="rtp_transport">
          <a-radio-group v-model:value="draft.rtp_transport" button-style="solid">
            <a-radio-button value="UDP">UDP</a-radio-button>
            <a-radio-button value="TCP">TCP</a-radio-button>
          </a-radio-group>
        </a-form-item>
      </a-form>
      <a-space>
        <a-button @click="load">重载</a-button>
        <a-button type="primary" :loading="saving" @click="save">保存本端节点配置</a-button>
      </a-space>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { LocalNodeConfig, NodeNetworkStatusPayload } from '../types/gateway'

const saving = ref(false)
const networkStatus = ref<NodeNetworkStatusPayload>()

const draft = reactive<LocalNodeConfig>({
  node_id: '',
  node_name: '',
  node_role: '',
  network_mode: '',
  sip_listen_ip: '',
  sip_listen_port: 5060,
  sip_transport: 'UDP',
  rtp_listen_ip: '',
  rtp_port_start: 20000,
  rtp_port_end: 20999,
  rtp_transport: 'UDP'
})

const capabilitySummaryText = computed(() => {
  if (!networkStatus.value) return '-'
  const summary = networkStatus.value.capability_summary
  return `支持: ${summary.supported.join(', ') || '-'}；不支持: ${summary.unsupported.join(', ') || '-'}`
})

const load = async () => {
  const [detail, status] = await Promise.all([gatewayApi.fetchNodeDetail(), gatewayApi.fetchNodeNetworkStatus()])
  Object.assign(draft, detail.local_node)
  networkStatus.value = status
}

const save = async () => {
  saving.value = true
  try {
    await gatewayApi.updateLocalNode(JSON.parse(JSON.stringify(draft)))
    message.success('本端节点配置已保存')
    await load()
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
