<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="隧道映射查询">
      <a-form layout="inline">
        <a-form-item label="名称 / mapping_id">
          <a-input v-model:value="keyword" allow-clear placeholder="输入名称或 mapping_id" />
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button type="primary" @click="openCreate">新建映射</a-button>
            <a-button :loading="testingMapping" @click="runMappingTest">测试映射规则</a-button>
          </a-space>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card title="全局承载策略（只读）">
      <a-alert
        type="info"
        show-icon
        message="能力矩阵由后端实时返回（网络模式全局约束）"
        description="用于指导隧道映射配置：超出当前网络模式能力的配置会提示告警并在保存时拦截。"
        style="margin-bottom: 12px"
      />
      <a-descriptions bordered :column="2" size="small">
        <a-descriptions-item label="NetworkMode">{{ startupSummary?.network_mode ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="Capability 摘要">{{ capabilitySummaryText }}</a-descriptions-item>
        <a-descriptions-item label="request_meta_transport">{{ startupSummary?.transport_plan.request_meta_transport ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="request_body_transport">{{ startupSummary?.transport_plan.request_body_transport ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="response_meta_transport">{{ startupSummary?.transport_plan.response_meta_transport ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="response_body_transport">{{ startupSummary?.transport_plan.response_body_transport ?? '-' }}</a-descriptions-item>
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

    <a-card title="隧道映射列表">
      <a-alert
        v-if="mappingTestResult"
        :type="mappingTestPassed ? 'success' : 'error'"
        show-icon
        :message="`映射规则测试：SIP 请求 ${mappingTestResult.sip_request}，RTP 通道 ${mappingTestResult.rtp_channel}`"
        style="margin-bottom: 12px"
      />
      <a-alert v-if="warnings.length" type="warning" show-icon :message="warnings.join('；')" style="margin-bottom: 12px" />
      <a-table :columns="columns" :data-source="filteredMappings" row-key="mapping_id" :pagination="false">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-switch :checked="record.enabled" disabled />
          </template>
          <template v-if="column.key === 'local'">
            {{ endpointText(record.local_bind_ip, record.local_bind_port, record.local_base_path) }}
          </template>
          <template v-if="column.key === 'remote'">
            {{ endpointText(record.remote_target_ip, record.remote_target_port, record.remote_base_path) }}
          </template>
          <template v-if="column.key === 'methods'">
            {{ record.allowed_methods.join(', ') }}
          </template>
          <template v-if="column.key === 'timeouts'">
            req {{ record.request_timeout_ms }}ms / resp {{ record.response_timeout_ms }}ms
          </template>
          <template v-if="column.key === 'bodyLimits'">
            req {{ record.max_request_body_bytes }} / resp {{ record.max_response_body_bytes }}
          </template>
          <template v-if="column.key === 'linkStatus'">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? 'active' : 'disabled' }}</a-tag>
          </template>
          <template v-if="column.key === 'action'">
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
        message="当前隧道映射配置超出网络模式能力"
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
        <a-form-item label="mapping_id">
          <a-input v-model:value="editing.mapping_id" :disabled="editingMode === 'edit'" />
        </a-form-item>
        <a-form-item label="名称"><a-input v-model:value="editing.name" /></a-form-item>
        <a-form-item label="启用"><a-switch v-model:checked="editing.enabled" /></a-form-item>
        <a-form-item label="对端节点"><a-input v-model:value="editing.peer_node_id" /></a-form-item>
        <a-row :gutter="12">
          <a-col :span="8"><a-form-item label="本端 IP"><a-input v-model:value="editing.local_bind_ip" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="本端 Port"><a-input-number v-model:value="editing.local_bind_port" :min="1" :max="65535" style="width: 100%" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="本端 basePath"><a-input v-model:value="editing.local_base_path" /></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="8"><a-form-item label="对端 IP"><a-input v-model:value="editing.remote_target_ip" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="对端 Port"><a-input-number v-model:value="editing.remote_target_port" :min="1" :max="65535" style="width: 100%" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="对端 basePath"><a-input v-model:value="editing.remote_base_path" /></a-form-item></a-col>
        </a-row>
        <a-form-item label="方法白名单（逗号分隔）"><a-input v-model:value="allowedMethodsText" /></a-form-item>
        <a-row :gutter="12">
          <a-col :span="12"><a-form-item label="request timeout (ms)"><a-input-number v-model:value="editing.request_timeout_ms" :min="1" style="width: 100%" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="response timeout (ms)"><a-input-number v-model:value="editing.response_timeout_ms" :min="1" style="width: 100%" /></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="12"><a-form-item label="max_request_body_bytes"><a-input-number v-model:value="editing.max_request_body_bytes" :min="1" style="width: 100%" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="max_response_body_bytes"><a-input-number v-model:value="editing.max_response_body_bytes" :min="1" style="width: 100%" /></a-form-item></a-col>
        </a-row>
        <a-form-item>
          <template #label>
            流式响应（仅在当前网络模式支持时可启用）
          </template>
          <a-switch v-model:checked="editing.require_streaming_response" />
        </a-form-item>
        <a-form-item label="description"><a-textarea v-model:value="editing.description" :rows="3" /></a-form-item>
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
import type { CapabilityItem, MappingTestPayload, StartupSummaryPayload, TunnelMapping } from '../types/gateway'
import { buildCapabilityMatrix, evaluateMappingCapability } from '../utils/capability'

const keyword = ref('')
const drawerVisible = ref(false)
const editingMode = ref<'create' | 'edit'>('create')
const mappings = ref<TunnelMapping[]>([])
const warnings = ref<string[]>([])
const startupSummary = ref<StartupSummaryPayload>()
const testingMapping = ref(false)
const mappingTestResult = ref<MappingTestPayload>()

const emptyMapping = (): TunnelMapping => ({
  mapping_id: '',
  name: '',
  enabled: true,
  peer_node_id: '',
  local_bind_ip: '',
  local_bind_port: 18080,
  local_base_path: '/',
  remote_target_ip: '',
  remote_target_port: 8080,
  remote_base_path: '/',
  allowed_methods: ['POST'],
  connect_timeout_ms: 500,
  request_timeout_ms: 3000,
  response_timeout_ms: 3000,
  max_request_body_bytes: 1048576,
  max_response_body_bytes: 1048576,
  require_streaming_response: false,
  description: ''
})

const editing = reactive<TunnelMapping>(emptyMapping())
const allowedMethodsText = computed({
  get: () => editing.allowed_methods.join(', '),
  set: (value: string) => {
    editing.allowed_methods = value
      .split(',')
      .map((item) => item.trim().toUpperCase())
      .filter(Boolean)
  }
})

const endpointText = (ip: string, port: number, path: string) => `${ip}:${port}${path}`
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
const editorBlockingIssues = computed(() => editorCapabilityEvaluation.value.blockingIssues)
const editorAdvisoryWarnings = computed(() => editorCapabilityEvaluation.value.advisoryWarnings)


const mappingTestPassed = computed(() => {
  if (!mappingTestResult.value) return false
  return mappingTestResult.value.sip_request === 'success' && mappingTestResult.value.rtp_channel === 'success'
})

const columns = [
  { title: '名称', dataIndex: 'name', key: 'name' },
  { title: '启用', key: 'enabled' },
  { title: '对端节点', dataIndex: 'peer_node_id', key: 'peer_node_id' },
  { title: '本端入口', key: 'local' },
  { title: '对端目标', key: 'remote' },
  { title: '方法白名单', key: 'methods' },
  { title: '请求/响应超时', key: 'timeouts' },
  { title: '请求/响应体大小限制', key: 'bodyLimits' },
  { title: '链路状态', key: 'linkStatus' },
  { title: '操作', key: 'action' }
]

const filteredMappings = computed(() => {
  const k = keyword.value.trim().toLowerCase()
  if (!k) return mappings.value
  return mappings.value.filter((item) => item.name.toLowerCase().includes(k) || item.mapping_id.toLowerCase().includes(k))
})

const drawerTitle = computed(() => (editingMode.value === 'create' ? '新建隧道映射' : '编辑隧道映射'))

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
  const result = await gatewayApi.fetchMappings()
  mappings.value = result.items
  warnings.value = result.warnings ?? []
}

const loadReadonlyContext = async () => {
  startupSummary.value = await gatewayApi.fetchStartupSummary()
}


const runMappingTest = async () => {
  testingMapping.value = true
  try {
    mappingTestResult.value = await gatewayApi.testMapping()
    if (mappingTestPassed.value) {
      message.success('映射规则测试通过，隧道可用')
    } else {
      message.warning('映射规则测试未通过，请检查 SIP/RTP 链路')
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
  if (editingMode.value === 'create') {
    const result = await gatewayApi.createMapping(JSON.parse(JSON.stringify(editing)))
    message.success('隧道映射已创建')
    if (result.warnings?.length) {
      message.warning(`后端提示：${result.warnings.join('；')}`)
    }
  } else {
    const result = await gatewayApi.updateMapping(editing.mapping_id, JSON.parse(JSON.stringify(editing)))
    message.success('隧道映射已更新')
    if (result.warnings?.length) {
      message.warning(`后端提示：${result.warnings.join('；')}`)
    }
  }
  drawerVisible.value = false
  await loadMappings()
}

const removeMapping = async (id: string) => {
  await gatewayApi.deleteMapping(id)
  message.success('隧道映射已删除')
  await loadMappings()
}

onMounted(async () => {
  await Promise.all([loadMappings(), loadReadonlyContext()])
})
</script>
