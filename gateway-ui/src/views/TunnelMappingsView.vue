<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="隧道映射查询">
      <a-form layout="inline">
        <a-form-item label="映射ID">
          <a-input v-model:value="keyword" allow-clear placeholder="输入映射ID" />
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button type="primary" @click="openCreate">新建映射</a-button>
            <a-button :loading="testingMapping" @click="runMappingTest">测试映射</a-button>
          </a-space>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card title="全局承载策略（只读）">
      <a-alert
        type="info"
        show-icon
        message="能力矩阵由后端实时返回（网络模式全局约束）"
        description="用于指导映射规则配置：超出当前网络模式能力的配置会提示告警并在保存时拦截。"
        style="margin-bottom: 12px"
      />
      <a-descriptions bordered :column="2" size="small">
        <a-descriptions-item label="网络模式">{{ networkModeProfile?.shortLabel ?? startupSummary?.network_mode ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="发送端 / 接收端">{{ networkModeProfile?.flowLabel ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="能力摘要">{{ capabilitySummaryText }}</a-descriptions-item>
        <a-descriptions-item label="request_meta_transport">{{ startupSummary?.transport_plan.request_meta_transport ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="request_body_transport">{{ startupSummary?.transport_plan.request_body_transport ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="response_meta_transport">{{ startupSummary?.transport_plan.response_meta_transport ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="response_body_transport">{{ startupSummary?.transport_plan.response_body_transport ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="请求方向">{{ networkModeProfile?.requestDirection ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="响应方向">{{ networkModeProfile?.responseDirection ?? '-' }}</a-descriptions-item>
      </a-descriptions>

      <a-table
        size="small"
        :pagination="false"
        :columns="capabilityColumns"
        :data-source="capabilityMatrix"
        row-key="key"
        style="margin-top: 12px"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'supported'">
            <a-tag :color="record.supported ? 'green' : 'red'">{{ record.supported ? '支持' : '不支持' }}</a-tag>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-card title="当前绑定对端（只读）">
      <a-alert
        v-if="mappingBindingError"
        type="error"
        show-icon
        :message="mappingBindingError"
        description="隧道映射当前按单对单模式运行：必须且仅能有一个启用的对端节点。"
        style="margin-bottom: 12px"
      />
      <a-descriptions bordered :column="2" size="small">
        <a-descriptions-item label="peer_node_id">{{ boundPeer?.peer_node_id ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="peer_name">{{ boundPeer?.peer_name ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="peer_signaling">{{ boundPeerEndpoint }}</a-descriptions-item>
        <a-descriptions-item label="映射绑定策略">隧道映射自动绑定唯一启用对端（不可编辑）</a-descriptions-item>
      </a-descriptions>
    </a-card>

    <a-card title="隧道映射列表">
      <a-alert
        v-if="mappingTestResult"
        :type="mappingTestPassed ? 'success' : 'error'"
        show-icon
        :message="mappingTestSummary"
        :description="mappingTestResult.failure_reason ? `阻塞原因：${mappingTestResult.failure_reason}` : undefined"
        style="margin-bottom: 12px"
      />
      <a-table
        v-if="mappingTestResult"
        size="small"
        :pagination="false"
        :columns="mappingTestColumns"
        :data-source="mappingTestResult.stages"
        row-key="key"
        style="margin-bottom: 12px"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="mappingStageColor(record.status)">{{ mappingStageLabel(record.status) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'blocking_reason'">{{ record.blocking_reason || '-' }}</template>
          <template v-else-if="column.key === 'suggested_action'">{{ record.suggested_action || '-' }}</template>
        </template>
      </a-table>
      <a-alert v-if="warnings.length" type="warning" show-icon :message="warnings.join('；')" style="margin-bottom: 12px" />
      <a-table :columns="columns" :data-source="filteredMappings" row-key="mapping_id" :pagination="false">
        <template #bodyCell="{ column, record, index }">
          <template v-if="column.key === 'index'">{{ index + 1 }}</template>
          <template v-else-if="column.key === 'local'">
            {{ endpointText(record.local_bind_ip, record.local_bind_port, record.local_base_path) }}
          </template>
          <template v-else-if="column.key === 'remote'">
            {{ endpointText(record.remote_target_ip, record.remote_target_port, record.remote_base_path) }}
          </template>
          <template v-else-if="column.key === 'protocol'">HTTP</template>
          <template v-else-if="column.key === 'status'">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '已启用' : '未启用' }}</a-tag>
          </template>
          <template v-else-if="column.key === 'link_status'">
            <a-tag :color="mappingLinkColor(record.link_status, record.link_status_text)">{{ mappingLinkText(record.link_status, record.link_status_text) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'status_reason'">{{ mappingReasonText(record.failure_reason || record.status_reason) }}</template>
          <template v-else-if="column.key === 'suggested_action'">{{ mappingActionText(record.suggested_action) }}</template>
          <template v-else-if="column.key === 'updated_at'">{{ record.updated_at || '-' }}</template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button type="link" @click="openEditor(record)">编辑</a-button>
              <a-popconfirm title="确认删除该映射？" @confirm="removeMapping(record.mapping_id)">
                <a-button type="link" danger>删除</a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="drawerVisible" :title="drawerTitle" width="640" @close="drawerVisible = false">
      <a-alert
        type="warning"
        show-icon
        message="当前映射规则配置超出网络模式能力"
        :description="editorBlockingIssues.join('；')"
        style="margin-bottom: 12px"
        v-if="editorBlockingIssues.length"
      />
      <a-alert
        type="info"
        show-icon
        message="配置风险提示"
        :description="editorAdvisoryWarnings.join('；')"
        style="margin-bottom: 12px"
        v-if="editorAdvisoryWarnings.length"
      />
      <a-form layout="vertical">
        <a-form-item label="映射ID">
          <a-input v-model:value="editing.mapping_id" :disabled="editingMode === 'edit'" />
        </a-form-item>
        <a-form-item label="本端入口 IP" extra="填写本端业务入口地址。">
          <a-input v-model:value="editing.local_bind_ip" />
        </a-form-item>
        <a-form-item label="本端入口端口" extra="监听端口范围 1-65535。">
          <a-input-number v-model:value="editing.local_bind_port" :min="1" :max="65535" style="width: 100%" />
        </a-form-item>

        <a-form-item label="对端目标 IP" extra="填写对端服务可达地址。">
          <a-input v-model:value="editing.remote_target_ip" />
        </a-form-item>
        <a-form-item label="对端目标端口" extra="目标端口范围 1-65535。">
          <a-input-number v-model:value="editing.remote_target_port" :min="1" :max="65535" style="width: 100%" />
        </a-form-item>

        <a-form-item label="请求超时（毫秒）" extra="控制请求等待时长，建议按链路质量设置。">
          <a-input-number v-model:value="editing.request_timeout_ms" :min="1" style="width: 100%" />
        </a-form-item>
        <a-form-item label="响应超时（毫秒）" extra="控制响应等待时长，超时后自动失败返回。">
          <a-input-number v-model:value="editing.response_timeout_ms" :min="1" style="width: 100%" />
        </a-form-item>
        <a-form-item label="请求体大小上限（字节）" extra="系统按动作类型自动选择命令或文件传输链路；route/api_code/template 为兼容术语（deprecated）。">
          <a-input-number v-model:value="editing.max_request_body_bytes" :min="1" style="width: 100%" />
        </a-form-item>
        <a-form-item label="响应体大小上限（字节）" extra="建议与对端能力矩阵保持一致。">
          <a-input-number v-model:value="editing.max_response_body_bytes" :min="1" style="width: 100%" />
        </a-form-item>
        <a-form-item label="启用状态">
          <a-switch v-model:checked="editing.enabled" />
        </a-form-item>
        <a-form-item label="本端入口路径" extra="用于拼接本端入口完整请求路径。">
          <a-input v-model:value="editing.local_base_path" />
        </a-form-item>
        <a-form-item label="对端目标路径" extra="用于拼接对端目标完整请求路径。">
          <a-input v-model:value="editing.remote_base_path" />
        </a-form-item>
        <a-form-item>
          <template #label>
            流式响应（仅在当前网络模式支持时可启用）
          </template>
          <a-switch v-model:checked="editing.require_streaming_response" />
        </a-form-item>
        <a-form-item label="备注"><a-textarea v-model:value="editing.description" :rows="3" /></a-form-item>
      </a-form>
      <template #footer>
        <a-space style="width: 100%; justify-content: flex-end">
          <a-button @click="drawerVisible = false">取消</a-button>
          <a-button type="primary" :disabled="editorBlockingIssues.length > 0" @click="save">保存</a-button>
        </a-space>
      </template>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { CapabilityItem, MappingTestPayload, PeerBinding, StartupSummaryPayload, TunnelMapping } from '../types/gateway'
import { buildCapabilityMatrix, evaluateMappingCapability } from '../utils/capability'
import { getNetworkModeProfile } from '../utils/networkMode'

const keyword = ref('')
const drawerVisible = ref(false)
const editingMode = ref<'create' | 'edit'>('create')
const mappings = ref<TunnelMapping[]>([])
const warnings = ref<string[]>([])
const boundPeer = ref<PeerBinding>()
const mappingBindingError = ref('')
const startupSummary = ref<StartupSummaryPayload>()
const testingMapping = ref(false)
const mappingTestResult = ref<MappingTestPayload>()

const emptyMapping = (): TunnelMapping => ({
  mapping_id: '',
  enabled: true,
  local_bind_ip: '',
  local_bind_port: 18080,
  local_base_path: '/',
  remote_target_ip: '',
  remote_target_port: 8080,
  remote_base_path: '/',
  connect_timeout_ms: 500,
  request_timeout_ms: 3000,
  response_timeout_ms: 3000,
  max_request_body_bytes: 1048576,
  max_response_body_bytes: 1048576,
  require_streaming_response: false,
  description: ''
})

const editing = reactive<TunnelMapping>(emptyMapping())

const endpointText = (ip: string, port: number, path: string) => `${ip}:${port}${path}`
const networkModeProfile = computed(() => getNetworkModeProfile(startupSummary.value?.network_mode ?? ""))

const capabilitySummaryText = computed(() => {
  if (!startupSummary.value) return '-'
  return `支持: ${startupSummary.value.capability_summary.supported.join(', ') || '-'}；不支持: ${startupSummary.value.capability_summary.unsupported.join(', ') || '-'}`
})

const capabilityColumns = [
  { title: '能力项', dataIndex: 'label', key: 'label' },
  { title: '当前模式', key: 'supported' },
  { title: '运维提示', dataIndex: 'note', key: 'note' }
]

const capabilityMatrix = computed<CapabilityItem[]>(() => {
  const capability = startupSummary.value?.capability
  if (!capability) return []
  return buildCapabilityMatrix(capability, startupSummary.value?.capability_summary)
})

const editorCapabilityEvaluation = computed(() => evaluateMappingCapability(editing, startupSummary.value))

const boundPeerEndpoint = computed(() => {
  if (!boundPeer.value?.peer_signaling_ip || !boundPeer.value?.peer_signaling_port) return '-'
  return `${boundPeer.value.peer_signaling_ip}:${boundPeer.value.peer_signaling_port}`
})
const editorBlockingIssues = computed(() => editorCapabilityEvaluation.value.blockingIssues)
const editorAdvisoryWarnings = computed(() => editorCapabilityEvaluation.value.advisoryWarnings)

const mappingTestPassed = computed(() => mappingTestResult.value?.passed ?? false)

const mappingTestSummary = computed(() => {
  if (!mappingTestResult.value) return ''
  const failed = mappingTestResult.value.stages.find((item) => !item.passed)
  if (!failed) {
    return '映射联调测试通过：本地监听、注册、心跳、对端可达、会话准备和映射转发均已就绪。'
  }
  return `映射联调测试未通过：卡在“${failed.name}”阶段。`
})

const mappingTestColumns = [
  { title: '阶段', dataIndex: 'name', key: 'name' },
  { title: '结果', key: 'status' },
  { title: '详情', dataIndex: 'detail', key: 'detail' },
  { title: '阻塞原因', key: 'blocking_reason' },
  { title: '建议动作', key: 'suggested_action' }
]

const mappingStageLabel = (status: 'passed' | 'failed' | 'blocked') => {
  if (status === 'passed') return '通过'
  if (status === 'blocked') return '阻塞'
  return '失败'
}

const mappingStageColor = (status: 'passed' | 'failed' | 'blocked') => {
  if (status === 'passed') return 'success'
  if (status === 'blocked') return 'warning'
  return 'error'
}

const columns = [
  { title: '序号', key: 'index' },
  { title: '本端入口', key: 'local' },
  { title: '对端目标', key: 'remote' },
  { title: '协议', key: 'protocol' },
  { title: '状态', key: 'status' },
  { title: '映射链路状态', key: 'link_status' },
  { title: '异常原因', key: 'status_reason' },
  { title: '建议动作', key: 'suggested_action' },
  { title: '更新时间', key: 'updated_at' },
  { title: '操作', key: 'action' }
]

const mappingLinkText = (status?: TunnelMapping['link_status'], text?: TunnelMapping['link_status_text']) => {
  if (text) return text
  if (status === 'listening') return '监听中'
  if (status === 'start_failed') return '启动失败'
  if (status === 'interrupted' || status === 'abnormal') return '异常'
  if (status === 'disabled') return '未启用'
  if (status === 'connected') return '已连接'
  return '异常'
}

const mappingLinkColor = (status?: TunnelMapping['link_status'], text?: TunnelMapping['link_status_text']) => {
  const normalized = text || mappingLinkText(status)
  if (normalized === '未启用') return 'default'
  if (normalized === '监听中' || normalized === '已连接') return 'success'
  if (normalized === '启动失败' || normalized === '异常') return 'error'
  return 'error'
}

const mappingReasonText = (reason?: string) => {
  if (reason && reason.trim()) return reason
  return '原因未上报，请检查注册状态、心跳状态和对端可达性。'
}

const mappingActionText = (action?: string) => {
  if (action && action.trim()) return action
  return '建议先执行链路测试，再按节点状态页面提示逐项排查。'
}

const filteredMappings = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  if (!k) return mappings.value
  return mappings.value.filter((item) => item.mapping_id.toLowerCase().includes(k))
})

const drawerTitle = computed(() => (editingMode.value === 'create' ? '新建隧道映射' : '编辑隧道映射'))

const inferMappingRuntimeStatus = (
  item: TunnelMapping,
  systemStatus?: { tunnel_status: string; registration_status?: string; heartbeat_status?: string; connection_reason?: string; peer_binding_error?: string }
): Pick<TunnelMapping, 'link_status' | 'link_status_text' | 'status_reason' | 'failure_reason' | 'suggested_action'> => {
  if (item.link_status && item.status_reason) {
    return {
      link_status: item.link_status,
      link_status_text: item.link_status_text,
      status_reason: item.status_reason,
      failure_reason: item.failure_reason ?? item.status_reason,
      suggested_action: item.suggested_action
    }
  }
  if (!item.enabled) {
    return { link_status: 'disabled', link_status_text: '未启用', status_reason: '规则未启用。', failure_reason: '规则未启用。', suggested_action: '按需开启规则后再观察链路状态。' }
  }
  if (!systemStatus) {
    return { link_status: 'abnormal', link_status_text: '异常', status_reason: '未获取到系统状态。', failure_reason: '未获取到系统状态。', suggested_action: '先检查网关服务状态，再刷新页面重试。' }
  }
  if (systemStatus.peer_binding_error) {
    return { link_status: 'abnormal', link_status_text: '异常', status_reason: '未建立响应通道：' + systemStatus.peer_binding_error, failure_reason: '未建立响应通道：' + systemStatus.peer_binding_error, suggested_action: '确保仅启用一个对端节点并核对对端信令配置。' }
  }
  if (systemStatus.registration_status !== 'registered') {
    return { link_status: 'abnormal', link_status_text: '异常', status_reason: '未注册，GB/T 28181 注册尚未完成。', failure_reason: '未注册，GB/T 28181 注册尚未完成。', suggested_action: '检查注册参数和认证信息，确认注册成功后再联调。' }
  }
  if (systemStatus.heartbeat_status !== 'healthy') {
    return { link_status: 'abnormal', link_status_text: '异常', status_reason: '心跳超时，等待下一次心跳恢复。', failure_reason: '心跳超时，等待下一次心跳恢复。', suggested_action: '检查网络抖动和心跳周期配置，恢复后再观察。' }
  }
  if (systemStatus.tunnel_status !== 'connected') {
    const reason = systemStatus.connection_reason || '对端不可达。'
    return { link_status: 'abnormal', link_status_text: '异常', status_reason: reason, failure_reason: reason, suggested_action: '检查对端可达性与链路策略，再执行规则测试。' }
  }
  return { link_status: 'connected', link_status_text: '已连接', status_reason: '链路正常。', failure_reason: '链路正常。', suggested_action: '无需处理，持续关注运行指标。' }
}

const openCreate = () => {
  editingMode.value = 'create'
  Object.assign(editing, emptyMapping())
  drawerVisible.value = true
}

const openEditor = (item: TunnelMapping) => {
  editingMode.value = 'edit'
  Object.assign(editing, JSON.parse(JSON.stringify(item)))
  drawerVisible.value = true
}

const loadMappings = async () => {
  const [result, systemStatus] = await Promise.all([gatewayApi.fetchMappings(), gatewayApi.fetchSystemStatus()])
  mappings.value = result.items.map((item) => ({
    ...item,
    ...inferMappingRuntimeStatus(item, systemStatus)
  }))
  warnings.value = result.warnings ?? []
  boundPeer.value = result.bound_peer
  mappingBindingError.value = result.binding_error ?? ''
}

const loadReadonlyContext = async () => {
  startupSummary.value = await gatewayApi.fetchStartupSummary()
}

const runMappingTest = async () => {
  testingMapping.value = true
  try {
    mappingTestResult.value = await gatewayApi.testMapping()
    if (mappingTestPassed.value) {
      message.success('隧道映射联调测试通过，链路阶段全部就绪')
    } else {
      message.warning(`隧道映射联调测试未通过，阻塞阶段：${mappingTestResult.value.failure_stage || '未知'}`)
    }
  } finally {
    testingMapping.value = false
  }
}

const save = async () => {
  if (editorBlockingIssues.value.length > 0) {
    message.error(editorBlockingIssues.value.join('；'))
    return
  }
  if (mappingBindingError.value) {
    message.error(mappingBindingError.value)
    return
  }
  const payload: TunnelMapping = {
    ...JSON.parse(JSON.stringify(editing)),
    allowed_methods: ['*']
  }
  if (editingMode.value === 'create') {
    const result = await gatewayApi.createMapping(payload)
    message.success('映射规则已创建')
    if (result.warnings?.length) {
      message.warning(`后端提示：${result.warnings.join('；')}`)
    }
  } else {
    const result = await gatewayApi.updateMapping(editing.mapping_id, payload)
    message.success('映射规则已更新')
    if (result.warnings?.length) {
      message.warning(`后端提示：${result.warnings.join('；')}`)
    }
  }
  drawerVisible.value = false
  await loadMappings()
}

const removeMapping = async (id: string) => {
  await gatewayApi.deleteMapping(id)
  message.success('映射规则已删除')
  await loadMappings()
}

onMounted(async () => {
  await Promise.all([loadMappings(), loadReadonlyContext()])
})
</script>
