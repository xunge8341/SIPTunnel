<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="HTTP 映射隧道配置（GB/T 28181 注册与心跳）">
      <a-alert type="info" show-icon style="margin-bottom: 12px" message="当前仅保留严格模式：控制面统一使用 MESSAGE + Application/MANSCDP+xml；读接口不回显密码原文。" />
      <a-form layout="vertical">
        <a-form-item label="通道协议（控制面）">
          <a-input :value="draft.channel_protocol" disabled />
        </a-form-item>
        <a-form-item>
          <template #label>
            <a-space size="small">
              当前节点角色（发送端 / 接收端，只读）
              <a-tooltip>
                <template #title>角色由网络模式决定：本端与对端在请求/响应方向上各自承担发送端或接收端职责。</template>
                <a-typography-text type="secondary">ⓘ</a-typography-text>
              </a-tooltip>
            </a-space>
          </template>
          <a-space direction="vertical" style="width: 100%">
            <a-alert type="info" show-icon :message="networkModeProfile?.senderRole ?? '发送端角色未知'" />
            <a-alert type="info" show-icon :message="networkModeProfile?.receiverRole ?? '接收端角色未知'" />
            <a-typography-text type="secondary">{{ networkModeProfile?.requestDirection ?? '-' }}</a-typography-text>
            <a-typography-text type="secondary">{{ networkModeProfile?.responseDirection ?? '-' }}</a-typography-text>
          </a-space>
        </a-form-item>
        <a-form-item>
          <template #label>
            <a-space size="small">
              网络模式（只读）
              <a-tooltip>
                <template #title>网络模式是全局约束，决定能力矩阵与 transport 推导，不能按单条映射覆盖。</template>
                <a-typography-text type="secondary">ⓘ</a-typography-text>
              </a-tooltip>
            </a-space>
          </template>
          <a-input :value="networkModeProfile?.flowLabel ?? networkModeLabel" disabled />
        </a-form-item>

        <a-form-item label="本端编码（来源：节点配置）">
          <a-input :value="draft.local_device_id || '未配置'" disabled />
        </a-form-item>
        <a-form-item label="对端编码（来源：节点配置）">
          <a-input :value="draft.peer_device_id || '未配置'" disabled />
        </a-form-item>
        <a-alert
          type="info"
          show-icon
          style="margin-bottom: 12px"
          message="编码由节点配置统一维护，通道配置仅展示，不可编辑。"
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
        <a-divider orientation="left">注册鉴权与目录订阅</a-divider>
        <a-row :gutter="12">
          <a-col :span="8"><a-form-item label="启用 REGISTER Digest 鉴权"><a-switch v-model:checked="draft.register_auth_enabled" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="鉴权算法"><a-select v-model:value="draft.register_auth_algorithm" :options="registerAuthAlgorithmOptions" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="Catalog 续订周期（秒）"><a-input-number v-model:value="draft.catalog_subscribe_expires_sec" :min="60" style="width: 100%" /></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="8"><a-form-item label="鉴权用户名"><a-input v-model:value="draft.register_auth_username" :disabled="!draft.register_auth_enabled" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="鉴权密码" extra="留空表示保持不变；读接口不回显密码原文。"><a-input-password v-model:value="registerAuthPasswordInput" :disabled="!draft.register_auth_enabled" /><a-typography-text type="secondary">当前状态：{{ draft.register_auth_password_configured ? "已配置" : "未配置" }}</a-typography-text></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="鉴权域 Realm"><a-input v-model:value="draft.register_auth_realm" :disabled="!draft.register_auth_enabled" /></a-form-item></a-col>
        </a-row>
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
      <a-row :gutter="12">
        <a-col :span="12">
          <a-alert type="error" show-icon :message="`最近失败原因：${draft.last_failure_reason || '暂无'}`" />
        </a-col>
        <a-col :span="12">
          <a-alert type="warning" show-icon :message="`下次重试时间：${formatDateTime(draft.next_retry_time)}`" />
        </a-col>
      </a-row>
      <a-row :gutter="12" style="margin-top: 12px">
        <a-col :span="24">
          <a-space>
            <a-button :loading="actionLoading" @click="runAction('register_now')">立即注册</a-button>
            <a-button :loading="actionLoading" @click="runAction('reregister')">重新注册</a-button>
            <a-button :loading="actionLoading" @click="runAction('heartbeat_once')">发送一次心跳</a-button>
          </a-space>
        </a-col>
      </a-row>
      <a-alert
        type="info"
        show-icon
        style="margin-top: 12px"
        message="transport 来源：由网络模式与映射承载模式共同推导（SIP 请求通道 + SIP/RTP 响应通道），无需逐条映射配置。"
      />
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { TunnelConfigPayload, TunnelConfigUpdatePayload, TunnelSessionActionPayload } from '../types/gateway'
import { deriveTunnelCapability } from '../utils/tunnelConfig'
import { formatDateTimeText } from '../utils/date'
import { getNetworkModeProfile } from '../utils/networkMode'

const saving = ref(false)
const actionLoading = ref(false)
const registerAuthAlgorithmOptions = [{ label: 'MD5', value: 'MD5' }]
const registerAuthPasswordInput = ref('')

const networkModeLabels: Record<string, string> = {
  SENDER_SIP__RECEIVER_SIP: '模式0：SIP --> | <-- SIP',
  SENDER_SIP__RECEIVER_RTP: '模式1：SIP --> | <-- RTP',
  SENDER_SIP__RECEIVER_SIP_RTP: '模式2：SIP --> | <-- SIP&RTP',
  SENDER_SIP_RTP__RECEIVER_SIP_RTP: '模式3：SIP&RTP --> | <-- SIP&RTP'
}

const draft = reactive<TunnelConfigPayload>({
  channel_protocol: 'GB/T 28181',
  connection_initiator: 'LOCAL',
  mapping_relay_mode: 'AUTO',
  local_device_id: '',
  peer_device_id: '',
  heartbeat_interval_sec: 60,
  register_retry_count: 3,
  register_retry_interval_sec: 10,
  registration_status: 'unregistered',
  last_register_time: '',
  last_heartbeat_time: '',
  heartbeat_status: 'unknown',
  last_failure_reason: '',
  next_retry_time: '',
  consecutive_heartbeat_timeout: 0,
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

const formatDateTime = (value: string) => formatDateTimeText(value, '暂无')

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
  registerAuthPasswordInput.value = ""
}

const save = async () => {
  saving.value = true
  try {
    const payload: TunnelConfigUpdatePayload = {
      channel_protocol: draft.channel_protocol,
      connection_initiator: draft.connection_initiator,
      mapping_relay_mode: draft.mapping_relay_mode ?? 'AUTO',
      heartbeat_interval_sec: draft.heartbeat_interval_sec,
      register_retry_count: draft.register_retry_count,
      register_retry_interval_sec: draft.register_retry_interval_sec,
      network_mode: draft.network_mode,
      register_auth_enabled: draft.register_auth_enabled,
      register_auth_username: draft.register_auth_username,
      register_auth_password: registerAuthPasswordInput.value,
      register_auth_realm: draft.register_auth_realm,
      register_auth_algorithm: draft.register_auth_algorithm,
      catalog_subscribe_expires_sec: draft.catalog_subscribe_expires_sec
    }
    await gatewayApi.saveTunnelConfig(payload)
    message.success('GB/T 28181 注册与心跳配置保存成功')
    await load()
  } finally {
    saving.value = false
  }
}

const runAction = async (action: TunnelSessionActionPayload['action']) => {
  actionLoading.value = true
  try {
    await gatewayApi.triggerTunnelSessionAction({ action })
    message.success('会话动作已触发')
    await load()
  } finally {
    actionLoading.value = false
  }
}

onMounted(load)
</script>
