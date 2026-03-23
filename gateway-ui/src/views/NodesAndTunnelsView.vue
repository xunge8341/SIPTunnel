<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="节点与级联" sub-title="这里按 GB/T 28181 的本级域 / 上级域 / 下级域关系配置级联参数；运行态统一放到“链路监控”；本页不再承载隧道映射信息。">
      <template #extra>
        <a-space>
          <a-button @click="() => load()">刷新回读</a-button>
          <a-button type="primary" :loading="saving" @click="save">保存并应用</a-button>
        </a-space>
      </template>
    </a-page-header>

    <a-alert v-if="notice" type="success" :message="notice" show-icon />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-alert v-if="validationErrors.length" type="warning" show-icon message="请先修正以下配置问题">
      <template #description>
        <ul class="validation-list">
          <li v-for="item in validationErrors" :key="item">{{ item }}</li>
        </ul>
      </template>
    </a-alert>

    <a-spin :spinning="loading || saving">
      <a-empty v-if="!workspace" description="暂无节点与级联配置" />
      <template v-else>
        <a-row :gutter="16" align="top">
          <a-col :xs="24" :xl="18">
            <a-alert type="info" show-icon :message="roleHint" style="margin-bottom: 16px" />
            <a-alert type="info" show-icon message="当前仅保留严格模式：控制面统一使用 MESSAGE + Application/MANSCDP+xml；密码字段为只写，留空表示保持不变。" style="margin-bottom: 16px" />
            <a-card title="节点与会话配置" :bordered="false">
              <a-form layout="vertical">
                <a-divider orientation="left">本级域</a-divider>
                <a-row :gutter="12">
                  <a-col :span="24"><a-form-item label="本级域编码（20 位国标编码）"><a-space compact style="width: 100%"><a-input v-model:value="workspace.localNode.device_id" class="gb-code-input" maxlength="20" style="width: calc(100% - 52px); min-width: 360px" /><a-tooltip title="按服务器类型生成编码"><a-button style="width: 52px" @click="generateLocalNodeCode"><ApiOutlined /></a-button></a-tooltip></a-space></a-form-item></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="12"><a-form-item label="本级域监听地址"><a-input v-model:value="workspace.localNode.node_ip" /></a-form-item></a-col>
                  <a-col :span="12"><a-form-item label="本级域 SIP 信令端口"><a-input-number v-model:value="workspace.localNode.signaling_port" style="width: 100%" /></a-form-item></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="12"><a-form-item label="本级域 RTP 端口范围"><a-space compact style="width: 100%"><a-input-number v-model:value="workspace.localNode.rtp_port_start" style="width: 50%" /><a-input-number v-model:value="workspace.localNode.rtp_port_end" style="width: 50%" /></a-space></a-form-item></a-col>
                  <a-col :span="12"><a-form-item label="本地隧道映射端口范围"><a-space compact style="width: 100%"><a-input-number v-model:value="workspace.localNode.mapping_port_start" style="width: 50%" /><a-input-number v-model:value="workspace.localNode.mapping_port_end" style="width: 50%" /></a-space></a-form-item></a-col>
                </a-row>

                <a-divider orientation="left">级联对端</a-divider>
                <a-row :gutter="12">
                  <a-col :span="24"><a-form-item label="级联对端编码（20 位国标编码）"><a-space compact style="width: 100%"><a-input v-model:value="workspace.peerNode.device_id" class="gb-code-input" maxlength="20" style="width: calc(100% - 52px); min-width: 360px" /><a-tooltip title="按服务器类型生成编码"><a-button style="width: 52px" @click="generatePeerNodeCode"><ApiOutlined /></a-button></a-tooltip></a-space></a-form-item></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="12"><a-form-item label="级联对端地址"><a-input v-model:value="workspace.peerNode.node_ip" /></a-form-item></a-col>
                  <a-col :span="12"><a-form-item label="级联对端 SIP 信令端口"><a-input-number v-model:value="workspace.peerNode.signaling_port" style="width: 100%" /></a-form-item></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="24"><a-form-item label="级联对端 RTP 端口范围"><a-space compact style="width: 100%"><a-input-number v-model:value="workspace.peerNode.rtp_port_start" style="width: 50%" /><a-input-number v-model:value="workspace.peerNode.rtp_port_end" style="width: 50%" /></a-space></a-form-item></a-col>
                </a-row>

                <a-divider orientation="left">会话与传输</a-divider>
                <a-row :gutter="12">
                  <a-col :span="12"><a-form-item label="连接发起方"><a-select v-model:value="workspace.sessionSettings.connection_initiator" :options="initiatorOptions" /></a-form-item></a-col>
                  <a-col :span="12"><a-form-item label="网络能力模式"><a-select v-model:value="workspace.networkMode" :options="networkModeOptions" /></a-form-item></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="12"><a-form-item label="SIP 传输"><a-select v-model:value="workspace.sipCapability.transport" :options="transportOptions" /></a-form-item></a-col>
                  <a-col :span="12"><a-form-item label="RTP 传输"><a-select v-model:value="workspace.rtpCapability.transport" :options="transportOptions" /></a-form-item></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="24"><a-form-item label="映射承载模式"><a-select v-model:value="workspace.sessionSettings.mapping_relay_mode" :options="mappingRelayModeOptions" /></a-form-item></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="8"><a-form-item label="心跳间隔（秒）"><a-input-number v-model:value="workspace.sessionSettings.heartbeat_interval_sec" :min="5" style="width: 100%" /></a-form-item></a-col>
                  <a-col :span="8"><a-form-item label="注册重试次数"><a-input-number v-model:value="workspace.sessionSettings.register_retry_count" :min="0" style="width: 100%" /></a-form-item></a-col>
                  <a-col :span="8"><a-form-item label="重试间隔（秒）"><a-input-number v-model:value="workspace.sessionSettings.register_retry_interval_sec" :min="1" style="width: 100%" /></a-form-item></a-col>
                </a-row>

                <a-divider orientation="left">REGISTER 鉴权与目录订阅</a-divider>
                <a-row :gutter="12">
                  <a-col :span="8"><a-form-item label="启用 Digest 鉴权"><a-switch v-model:checked="workspace.sessionSettings.register_auth_enabled" /></a-form-item></a-col>
                  <a-col :span="8"><a-form-item label="Catalog 续订周期（秒）"><a-input-number v-model:value="workspace.sessionSettings.catalog_subscribe_expires_sec" :min="60" style="width: 100%" /></a-form-item></a-col>
                  <a-col :span="8"><a-form-item label="鉴权算法"><a-select v-model:value="workspace.sessionSettings.register_auth_algorithm" :options="registerAuthAlgorithmOptions" /></a-form-item></a-col>
                </a-row>
                <a-row :gutter="12">
                  <a-col :span="8"><a-form-item label="鉴权用户名"><a-input v-model:value="workspace.sessionSettings.register_auth_username" :disabled="!workspace.sessionSettings.register_auth_enabled" /></a-form-item></a-col>
                  <a-col :span="8"><a-form-item label="鉴权密码" extra="留空表示保持不变；读接口不回显密码原文。"><a-input-password v-model:value="registerAuthPasswordInput" :disabled="!workspace.sessionSettings.register_auth_enabled" /><a-typography-text type="secondary">当前状态：{{ workspace.sessionSettings.register_auth_password_configured ? "已配置" : "未配置" }}</a-typography-text></a-form-item></a-col>
                  <a-col :span="8"><a-form-item label="鉴权域 Realm"><a-input v-model:value="workspace.sessionSettings.register_auth_realm" :disabled="!workspace.sessionSettings.register_auth_enabled" /></a-form-item></a-col>
                </a-row>
              </a-form>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="6">
            <a-space direction="vertical" size="middle" style="width: 100%">
              <a-card title="能力矩阵" :bordered="false">
                <a-table :data-source="capabilityRows" :columns="capabilityColumns" row-key="key" :pagination="false" />
              </a-card>
              <a-card title="使用边界" :bordered="false">
                <a-typography-paragraph>本页统一使用“编码”而非“设备 / 资源”混称；本级域编码、级联对端编码均要求使用 20 位国标编码，默认生成时按服务器类型生成。</a-typography-paragraph>
                <a-typography-paragraph>资源定义请到“本地资源”；下级资源的监听补充请到“隧道映射”；注册与会话运行态请到“链路监控”。</a-typography-paragraph>
              </a-card>
            </a-space>
          </a-col>
        </a-row>
      </template>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { gatewayApi } from '../api/gateway'
import { ApiOutlined } from '@ant-design/icons-vue'
import type { NodeTunnelWorkspace } from '../types/gateway'
import { generateGBCode, isGBCode20 } from '../utils/gb28181'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const workspace = ref<NodeTunnelWorkspace>()
const validationErrors = ref<string[]>([])
const registerAuthPasswordInput = ref('')

const transportOptions = [{ label: 'UDP', value: 'UDP' }, { label: 'TCP', value: 'TCP' }]
const mappingRelayModeOptions = [{ label: '自动（按网络模式能力）', value: 'AUTO' }, { label: '仅 SIP 链路（小请求/小响应）', value: 'SIP_ONLY' }]
const initiatorOptions = [{ label: '本级域作为下级域：主动向上级域 REGISTER', value: 'LOCAL' }, { label: '本级域作为上级域：等待下级域 REGISTER', value: 'PEER' }]
const registerAuthAlgorithmOptions = [{ label: 'MD5', value: 'MD5' }]
const networkModeOptions = [
  { label: '仅 SIP：本级域发起；级联对端经 SIP 回传（小请求/小响应）', value: 'SENDER_SIP__RECEIVER_SIP' },
  { label: '单向请求：本级域发起；级联对端经 RTP 回传', value: 'SENDER_SIP__RECEIVER_RTP' },
  { label: '单向请求：本级域发起；级联对端经 SIP/RTP 回传', value: 'SENDER_SIP__RECEIVER_SIP_RTP' },
  { label: '双方请求：本级域与级联对端均支持 SIP/RTP', value: 'SENDER_SIP_RTP__RECEIVER_SIP_RTP' }
]
const capabilityColumns = [{ title: '能力项', dataIndex: 'label', key: 'label' }, { title: '结果', dataIndex: 'result', key: 'result' }]
const capabilityLabelMap: Record<string, string> = {
  supports_small_request_body: '小请求体',
  supports_large_request_body: '大请求体',
  supports_large_response_body: '大响应体',
  supports_streaming_response: '流式响应',
  supports_bidirectional_http_tunnel: '双向 HTTP 隧道'
}

const deriveCapabilityMatrix = (mode: string) => {
  if (mode === 'SENDER_SIP__RECEIVER_SIP') {
    return [
      { key: 'supports_small_request_body', supported: true },
      { key: 'supports_large_request_body', supported: false },
      { key: 'supports_large_response_body', supported: false },
      { key: 'supports_streaming_response', supported: false },
      { key: 'supports_bidirectional_http_tunnel', supported: false }
    ]
  }
  if (mode === 'SENDER_SIP__RECEIVER_RTP' || mode === 'SENDER_SIP__RECEIVER_SIP_RTP') {
    return [
      { key: 'supports_small_request_body', supported: true },
      { key: 'supports_large_request_body', supported: false },
      { key: 'supports_large_response_body', supported: true },
      { key: 'supports_streaming_response', supported: true },
      { key: 'supports_bidirectional_http_tunnel', supported: false }
    ]
  }
  return [
    { key: 'supports_small_request_body', supported: true },
    { key: 'supports_large_request_body', supported: true },
    { key: 'supports_large_response_body', supported: true },
    { key: 'supports_streaming_response', supported: true },
    { key: 'supports_bidirectional_http_tunnel', supported: true }
  ]
}

const capabilityRows = computed(() => (workspace.value?.capabilityMatrix ?? []).map((item) => ({ key: item.key, label: capabilityLabelMap[item.key] ?? item.key, result: item.supported ? '支持' : '不支持' })))

const roleHint = computed(() => {
  const initiator = String(workspace.value?.sessionSettings.connection_initiator || '').toUpperCase()
  if (initiator === 'PEER') return '当前按 GB/T 28181 上级域模式运行：本级域监听 SIP 信令端口，等待下级域主动 REGISTER；级联对端是下级域。'
  return '当前按 GB/T 28181 下级域严格模式运行：本级域主动向上级域发起 REGISTER / MESSAGE(保活/控制) / SUBSCRIBE，级联对端是上级域。'
})

const applyNetworkModeLinkage = () => {
  if (!workspace.value) return
  const mode = workspace.value.networkMode
  workspace.value.capabilityMatrix = deriveCapabilityMatrix(mode)
  workspace.value.sessionSettings.network_mode = mode
  workspace.value.sessionSettings.request_channel = 'SIP'
  const relayMode = String(workspace.value.sessionSettings.mapping_relay_mode || 'AUTO').toUpperCase()
  if (mode === 'SENDER_SIP__RECEIVER_SIP') workspace.value.sessionSettings.response_channel = 'SIP'
  else if (mode === 'SENDER_SIP__RECEIVER_RTP') workspace.value.sessionSettings.response_channel = relayMode === 'SIP_ONLY' ? 'SIP' : 'RTP'
  else workspace.value.sessionSettings.response_channel = relayMode === 'SIP_ONLY' ? 'SIP' : 'SIP/RTP'
}

const validate = () => {
  const issues: string[] = []
  const data = workspace.value
  if (!data) return ['节点与级联数据尚未加载完成']
  if (!isGBCode20(data.localNode.device_id)) issues.push('本级域编码必须为 20 位国标编码')
  if (!isGBCode20(data.peerNode.device_id)) issues.push('级联对端编码必须为 20 位国标编码')
  if (!data.localNode.node_ip) issues.push('本级域监听地址不能为空')
  if (!data.peerNode.node_ip) issues.push('级联级联对端地址不能为空')
  if (!data.localNode.signaling_port) issues.push('本级域 SIP 信令端口不能为空')
  if (!data.peerNode.signaling_port) issues.push('级联对端 SIP 信令端口不能为空')
  if (!data.localNode.mapping_port_start || !data.localNode.mapping_port_end) issues.push('本地隧道映射端口范围不能为空')
  if ((data.localNode.mapping_port_start || 0) > (data.localNode.mapping_port_end || 0)) issues.push('本地隧道映射起始端口不能大于结束端口')
  if ((data.localNode.mapping_port_start || 0) <= (data.localNode.signaling_port || 0) && (data.localNode.signaling_port || 0) <= (data.localNode.mapping_port_end || 0)) issues.push('本地隧道映射端口范围不能覆盖本级域 SIP 信令端口')
  if ((data.localNode.mapping_port_start || 0) <= (data.localNode.rtp_port_end || 0) && (data.localNode.mapping_port_end || 0) >= (data.localNode.rtp_port_start || 0)) issues.push('本地隧道映射端口范围不能与本级域 RTP 端口范围重叠')
  if ((data.sessionSettings.catalog_subscribe_expires_sec || 0) < 60) issues.push('Catalog 续订周期至少为 60 秒')
  if ((data.sessionSettings.heartbeat_interval_sec || 0) < 5) issues.push('心跳间隔至少为 5 秒')
  if (data.sessionSettings.register_auth_enabled && !String(registerAuthPasswordInput.value || '').trim() && !data.sessionSettings.register_auth_password_configured) issues.push('启用 Digest 鉴权时必须填写鉴权密码')
  validationErrors.value = issues
  return issues
}


const generateLocalNodeCode = () => {
  if (!workspace.value) return
  workspace.value.localNode.device_id = generateGBCode('SERVER')
  validate()
}

const generatePeerNodeCode = () => {
  if (!workspace.value) return
  workspace.value.peerNode.device_id = generateGBCode('SERVER')
  validate()
}

const load = async (resetNotice = true) => {
  loading.value = true
  error.value = ''
  if (resetNotice) notice.value = ''
  try {
    workspace.value = await gatewayApi.fetchNodeTunnelWorkspace()
    registerAuthPasswordInput.value = ""
    applyNetworkModeLinkage()
    validate()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载节点与级联配置失败'
  } finally {
    loading.value = false
  }
}

const save = async () => {
  if (workspace.value && !String(workspace.value.localNode.device_id || '').trim()) generateLocalNodeCode()
  if (workspace.value && !String(workspace.value.peerNode.device_id || '').trim()) generatePeerNodeCode()
  if (validate().length) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const expectedStart = Number(workspace.value?.localNode?.mapping_port_start || 0)
    const expectedEnd = Number(workspace.value?.localNode?.mapping_port_end || 0)
    if (workspace.value) workspace.value.sessionSettings.register_auth_password = registerAuthPasswordInput.value
    await gatewayApi.saveNodeTunnelWorkspace(workspace.value as NodeTunnelWorkspace)
    const reloaded = await gatewayApi.fetchNodeTunnelWorkspace()
    workspace.value = reloaded
    registerAuthPasswordInput.value = ''
    applyNetworkModeLinkage()
    validate()
    const actualStart = Number(workspace.value?.localNode?.mapping_port_start || 0)
    const actualEnd = Number(workspace.value?.localNode?.mapping_port_end || 0)
    if (expectedStart > 0 && expectedEnd > 0 && (actualStart != expectedStart || actualEnd != expectedEnd)) {
      throw new Error(`保存后回读不一致：期望本地隧道映射端口范围 ${expectedStart}-${expectedEnd}，实际回读为 ${actualStart || '-'}-${actualEnd || '-'}`)
    }
    notice.value = '节点与级联配置已保存并应用'
  } catch (err) {
    error.value = err instanceof Error ? err.message : '保存节点与级联配置失败'
  } finally {
    saving.value = false
  }
}

watch(() => workspace.value?.networkMode, () => applyNetworkModeLinkage())
watch(() => workspace.value?.sessionSettings.mapping_relay_mode, () => applyNetworkModeLinkage())

onMounted(() => load())
</script>

<style scoped>
.validation-list {
  margin: 0;
  padding-left: 18px;
}
.gb-code-input :deep(input) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  letter-spacing: 0.5px;
  min-width: 360px;
}
</style>
