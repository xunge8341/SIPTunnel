<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="通道配置">
      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="通道协议">
              <a-select v-model:value="draft.channel_protocol" :options="channelProtocolOptions" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="网络模式">
              <a-select v-model:value="draft.network_mode" :options="networkModeOptions" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="请求通道">
              <a-select v-model:value="draft.request_channel" :options="channelOptions" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="响应通道">
              <a-select v-model:value="draft.response_channel" :options="channelOptions" />
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
      <a-space>
        <a-button @click="load">重载</a-button>
        <a-button type="primary" :loading="saving" @click="save">保存配置</a-button>
      </a-space>
    </a-card>

    <a-card title="能力矩阵（自动生成）">
      <a-alert type="info" show-icon message="当通道或网络模式变化时，能力矩阵会自动刷新。" style="margin-bottom: 12px" />
      <a-table :data-source="capabilityRows" :pagination="false" row-key="key" size="small">
        <a-table-column title="能力项" data-index="key" key="key" />
        <a-table-column title="支持" key="supported" width="120">
          <template #default="{ record }">
            <a-tag :color="record.supported ? 'green' : 'red'">{{ record.supported ? '支持' : '不支持' }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column title="说明" data-index="description" key="description" />
      </a-table>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { TunnelConfigPayload } from '../types/gateway'
import { buildTunnelCapabilityItems, deriveTunnelCapability } from '../utils/tunnelConfig'

const saving = ref(false)

const channelProtocolOptions = [{ label: 'GB28181', value: 'GB28181' }]
const channelOptions = [
  { label: 'SIP', value: 'SIP' },
  { label: 'RTP', value: 'RTP' },
  { label: 'HTTP', value: 'HTTP' }
]
const networkModeOptions = [
  { label: 'A 到 B 单向 SIP，B 到 A 单向 RTP', value: 'A_TO_B_SIP__B_TO_A_RTP' },
  { label: 'A 与 B 双向 SIP + 双向 RTP', value: 'A_B_BIDIR_SIP__BIDIR_RTP' },
  { label: 'A 与 B 双向 SIP，B 到 A 单向 RTP', value: 'A_B_BIDIR_SIP__B_TO_A_RTP' }
]

const draft = reactive<TunnelConfigPayload>({
  channel_protocol: 'GB28181',
  request_channel: 'SIP',
  response_channel: 'RTP',
  network_mode: 'A_TO_B_SIP__B_TO_A_RTP',
  capability: {
    supports_small_request_body: true,
    supports_large_request_body: false,
    supports_large_response_body: true,
    supports_streaming_response: true,
    supports_bidirectional_http_tunnel: false,
    supports_transparent_http_proxy: false
  },
  capability_items: []
})

watch(
  () => [draft.channel_protocol, draft.request_channel, draft.response_channel, draft.network_mode],
  () => {
    const capability = deriveTunnelCapability(draft)
    draft.capability = capability
    draft.capability_items = buildTunnelCapabilityItems(capability)
  },
  { immediate: true }
)

const capabilityRows = computed(() => draft.capability_items)

const load = async () => {
  const data = await gatewayApi.fetchTunnelConfig()
  Object.assign(draft, data)
}

const save = async () => {
  saving.value = true
  try {
    const capability = deriveTunnelCapability(draft)
    const payload: TunnelConfigPayload = {
      ...JSON.parse(JSON.stringify(draft)),
      capability,
      capability_items: buildTunnelCapabilityItems(capability)
    }
    await gatewayApi.saveTunnelConfig(payload)
    message.success('通道配置保存成功，能力矩阵已更新')
    await load()
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
