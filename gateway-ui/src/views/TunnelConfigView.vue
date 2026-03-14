<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="通道配置（GB/T 28181 注册与心跳）">
      <a-form layout="vertical">
        <a-form-item label="通道协议">
          <a-input :value="draft.channel_protocol" disabled />
        </a-form-item>
        <a-form-item label="发送端 / 接收端角色（只读）">
          <a-space direction="vertical" style="width: 100%">
            <a-alert type="info" show-icon :message="networkModeProfile?.senderRole ?? '发送端角色未知'" />
            <a-alert type="info" show-icon :message="networkModeProfile?.receiverRole ?? '接收端角色未知'" />
            <a-typography-text type="secondary">{{ networkModeProfile?.requestDirection ?? '-' }}</a-typography-text>
            <a-typography-text type="secondary">{{ networkModeProfile?.responseDirection ?? '-' }}</a-typography-text>
          </a-space>
        </a-form-item>
        <a-form-item label="网络模式（只读）">
          <a-input :value="networkModeProfile?.flowLabel ?? networkModeLabel" disabled />
        </a-form-item>

        <a-form-item label="本端设备编号（来源：节点配置）">
          <a-input :value="draft.local_device_id || '未配置'" disabled />
        </a-form-item>
        <a-form-item label="对端设备编号（来源：节点配置）">
          <a-input :value="draft.peer_device_id || '未配置'" disabled />
        </a-form-item>
        <a-alert
          type="info"
          show-icon
          style="margin-bottom: 12px"
          message="设备编码由节点配置统一维护，通道配置仅展示，不可编辑。"
        />

        <a-form-item label="心跳间隔（秒）">
          <a-input-number v-model:value="draft.heartbeat_interval_sec" :min="1" style="width: 100%" />
        </a-form-item>
        <a-form-item label="注册重试次数">
          <a-input-number v-model:value="draft.register_retry_count" :min="0" style="width: 100%" />
        </a-form-item>
        <a-form-item label="注册重试间隔（秒）">
          <a-input-number v-model:value="draft.register_retry_interval_sec" :min="1" style="width: 100%" />
        </a-form-item>
      </a-form>
      <a-space>
        <a-button @click="load">重载</a-button>
        <a-button type="primary" :loading="saving" @click="save">保存配置</a-button>
      </a-space>
    </a-card>

    <a-card title="注册与心跳状态（只读）">
      <a-row :gutter="12">
        <a-col :span="8">
          <a-statistic title="当前注册状态" :value="formatRegistrationStatus(draft.registration_status)" />
        </a-col>
        <a-col :span="8">
          <a-statistic title="最近注册时间" :value="formatDateTime(draft.last_register_time)" />
        </a-col>
        <a-col :span="8">
          <a-statistic title="最近心跳时间" :value="formatDateTime(draft.last_heartbeat_time)" />
        </a-col>
      </a-row>
      <a-row :gutter="12" style="margin-top: 12px">
        <a-col :span="8">
          <a-form-item label="心跳状态">
            <a-tag :color="heartbeatTagColor">{{ formatHeartbeatStatus(draft.heartbeat_status) }}</a-tag>
          </a-form-item>
        </a-col>
        <a-col :span="16">
          <a-form-item label="当前支持能力">
            <a-space wrap>
              <a-tag v-for="item in draft.supported_capabilities" :key="item" color="blue">{{ item }}</a-tag>
            </a-space>
          </a-form-item>
        </a-col>
      </a-row>
      <a-alert
        type="info"
        show-icon
        message="SIP 请求通道与 RTP 响应通道已由系统自动推导，无需运维单独编辑。"
      />
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { TunnelConfigPayload, TunnelConfigUpdatePayload } from '../types/gateway'
import { deriveTunnelCapability } from '../utils/tunnelConfig'
import { getNetworkModeProfile } from '../utils/networkMode'

const saving = ref(false)

const networkModeLabels: Record<string, string> = {
  SENDER_SIP__RECEIVER_RTP: '模式1：SIP --> | <-- RTP',
  SENDER_SIP__RECEIVER_SIP_RTP: '模式2：SIP --> | <-- SIP&RTP',
  SENDER_SIP_RTP__RECEIVER_SIP_RTP: '模式3：SIP&RTP --> | <-- SIP&RTP'
}

const draft = reactive<TunnelConfigPayload>({
  channel_protocol: 'GB/T 28181',
  connection_initiator: 'LOCAL',
  local_device_id: '',
  peer_device_id: '',
  heartbeat_interval_sec: 60,
  register_retry_count: 3,
  register_retry_interval_sec: 10,
  registration_status: 'unregistered',
  last_register_time: '',
  last_heartbeat_time: '',
  heartbeat_status: 'unknown',
  supported_capabilities: [],
  request_channel: 'SIP',
  response_channel: 'RTP',
  network_mode: 'SENDER_SIP__RECEIVER_RTP',
  capability: deriveTunnelCapability({ network_mode: 'SENDER_SIP__RECEIVER_RTP' }),
  capability_items: []
})

const heartbeatTagColor = computed(() => {
  if (draft.heartbeat_status === 'healthy') return 'green'
  if (draft.heartbeat_status === 'timeout') return 'red'
  return 'default'
})

const networkModeLabel = computed(() => networkModeLabels[draft.network_mode] ?? draft.network_mode)
const networkModeProfile = computed(() => getNetworkModeProfile(draft.network_mode))

const formatDateTime = (value: string) => {
  if (!value) return '暂无'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.toLocaleDateString()} ${date.toLocaleTimeString()}`
}

const formatRegistrationStatus = (status: string) => {
  if (status === 'registered') return '已注册'
  if (status === 'registering') return '注册中'
  if (status === 'failed') return '注册失败'
  return '未注册'
}

const formatHeartbeatStatus = (status: string) => {
  if (status === 'healthy') return '正常'
  if (status === 'timeout') return '超时'
  if (status === 'lost') return '丢失'
  return '未知'
}

const load = async () => {
  const data = await gatewayApi.fetchTunnelConfig()
  Object.assign(draft, data)
}

const save = async () => {
  saving.value = true
  try {
    const payload: TunnelConfigUpdatePayload = {
      channel_protocol: draft.channel_protocol,
      connection_initiator: draft.connection_initiator,
      heartbeat_interval_sec: draft.heartbeat_interval_sec,
      register_retry_count: draft.register_retry_count,
      register_retry_interval_sec: draft.register_retry_interval_sec,
      registration_status: draft.registration_status,
      last_register_time: draft.last_register_time,
      last_heartbeat_time: draft.last_heartbeat_time,
      heartbeat_status: draft.heartbeat_status,
      network_mode: draft.network_mode
    }
    await gatewayApi.saveTunnelConfig(payload)
    message.success('GB/T 28181 注册与心跳配置保存成功')
    await load()
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
