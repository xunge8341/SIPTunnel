<template>
  <a-space direction="vertical" size="large" style="width:100%">
    <a-page-header title="隧道映射" sub-title="在拉取到的下级资源基础上，补充本地监听端口与路径；超时和链路体量上限由全局链路配置统一控制。">
      <template #extra>
        <a-space>
          <a-button @click="load">刷新</a-button>
          <a-button :loading="testing" @click="runTest()">链路自检</a-button>
          <a-button :loading="testing" @click="runCatalogAction('pull_remote')">手动拉取目录</a-button>
        </a-space>
      </template>
    </a-page-header>

    <a-alert v-if="notice" type="success" :message="notice" show-icon />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-alert type="info" show-icon :message="`监听端口是本地 HTTP 入口，不是 RTP 端口；请从本地隧道映射端口范围 ${mappingRangeText} 中选择，RTP 端口仍由系统从 ${rtpRangeText} 内动态分配。`" />

    <a-row :gutter="12">
      <a-col :xs="24" :md="6"><a-card><a-statistic title="下级资源" :value="overview?.summary?.resource_total ?? 0" /></a-card></a-col>
      <a-col :xs="24" :md="6"><a-card><a-statistic title="已映射" :value="overview?.summary?.mapped_total ?? 0" /></a-card></a-col>
      <a-col :xs="24" :md="6"><a-card><a-statistic title="手工映射" :value="overview?.summary?.manual_total ?? 0" /></a-card></a-col>
      <a-col :xs="24" :md="6"><a-card><a-statistic title="未映射" :value="overview?.summary?.unmapped_total ?? 0" /></a-card></a-col>
    </a-row>

    <a-spin :spinning="loading || testing || saving">
      <a-card title="资源与映射总览" :bordered="false">
        <a-table :data-source="rows" :columns="columns" row-key="device_id" :pagination="false" :locale="{ emptyText: '尚未拉取到下级资源。' }">
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'mapping_status'">
              <a-tag :color="record.mapping_status === 'MANUAL' ? 'green' : 'default'">{{ mappingStatusLabel(record.mapping_status) }}</a-tag>
            </template>
            <template v-else-if="column.key === 'response_mode'">
              <a-tag>{{ record.response_mode }}</a-tag>
            </template>
            <template v-else-if="column.key === 'listen_ports'">
              {{ formatListenEntry(record) }}
            </template>
            <template v-else-if="column.key === 'methods'">
              {{ (record.methods || []).join(', ') || '*' }}
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space>
                <a-button type="link" @click="openMapping(record)">{{ record.mapping_status === 'UNMAPPED' ? '创建映射' : '补充监听' }}</a-button>
                <a-button type="link" @click="openDetail(record)">详情</a-button>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-card>
    </a-spin>

    <a-drawer v-model:open="editorOpen" :title="editingId ? '编辑隧道映射' : '新建隧道映射'" :width="860" destroy-on-close>
      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :span="12"><a-form-item label="资源名称"><a-input :value="currentResource?.name" disabled /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="资源编码（20 位国标编码）"><a-input :value="currentResource?.resource_code || currentResource?.device_id" disabled /></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="12"><a-form-item label="响应承载"><a-input :value="currentResource?.response_mode || 'AUTO'" disabled /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="级联对端"><a-input :value="currentResource?.source_node || '-'" disabled /></a-form-item></a-col>
        </a-row>
        <a-form-item label="允许方法">
          <a-select :value="currentResource?.methods || ['*']" mode="multiple" :options="methodOptions" disabled />
        </a-form-item>
        <a-divider orientation="left">本地监听补充信息</a-divider>
        <a-row :gutter="12">
          <a-col :span="8"><a-form-item label="监听 IP"><a-input v-model:value="editor.local_bind_ip" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="监听端口" :extra="`需落在本地隧道映射端口范围 ${mappingRangeText}`"><a-input-number v-model:value="editor.local_bind_port" :min="1" style="width:100%" :placeholder="mappingRangeText === '未配置' ? undefined : `${mappingRangeText}`" /></a-form-item></a-col>
          <a-col :span="8"><a-form-item label="路径前缀"><a-input v-model:value="editor.local_base_path" /></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="12"><a-form-item label="启用映射"><a-switch v-model:checked="editor.enabled" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="描述"><a-input v-model:value="editor.description" /></a-form-item></a-col>
        </a-row>
        <a-alert type="info" show-icon message="连接超时、请求超时、响应头超时以及体量上限由全局链路设置自动控制，这里只补充本地监听信息；监听端口必须落在本地隧道映射端口范围内。" />
      </a-form>
      <template #footer>
        <a-space style="float:right">
          <a-button v-if="editingId" danger @click="removeMapping">删除</a-button>
          <a-button @click="editorOpen = false">取消</a-button>
          <a-button type="primary" :loading="saving" @click="saveMapping">保存</a-button>
        </a-space>
      </template>
    </a-drawer>

    <a-drawer v-model:open="detailOpen" title="隧道映射详情" :width="860">
      <a-descriptions v-if="currentResource" :column="1" bordered size="small">
        <a-descriptions-item label="资源名称">{{ currentResource.name }}</a-descriptions-item>
        <a-descriptions-item label="资源编码">{{ currentResource.resource_code || currentResource.device_id }}</a-descriptions-item>
        <a-descriptions-item label="来源节点">{{ currentResource.source_node || '-' }}</a-descriptions-item>
        <a-descriptions-item label="映射状态">{{ mappingStatusLabel(currentResource.mapping_status) }}</a-descriptions-item>
        <a-descriptions-item label="本地监听">{{ formatListenEntry(currentResource) }}</a-descriptions-item>
        <a-descriptions-item label="允许方法">{{ (currentResource.methods || []).join(', ') || '*' }}</a-descriptions-item>
        <a-descriptions-item label="响应承载">{{ currentResource.response_mode }}</a-descriptions-item>
        <a-descriptions-item label="资源状态">{{ currentResource.resource_status }}</a-descriptions-item>
        <a-descriptions-item label="映射 ID">{{ (currentResource.mapping_ids || []).join(', ') || '-' }}</a-descriptions-item>
        <a-descriptions-item label="监听状态">{{ currentRaw?.link_status_text || currentRaw?.link_status || '-' }}</a-descriptions-item>
        <a-descriptions-item label="最近失败原因">{{ currentRaw?.failure_reason || '-' }}</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { NodeTunnelWorkspace, TunnelCatalogPayload, TunnelMapping, TunnelMappingOverviewItem, TunnelMappingOverviewPayload, TunnelCatalogActionPayload } from '../types/gateway'
import { deriveBodyLimitProfile, isGBCode20, methodOptions } from '../utils/gb28181'

const loading = ref(false)
const testing = ref(false)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const workspace = ref<NodeTunnelWorkspace>()
const catalog = ref<TunnelCatalogPayload>()
const overview = ref<TunnelMappingOverviewPayload>()
const rawMappings = ref<TunnelMapping[]>([])
const detailOpen = ref(false)
const editorOpen = ref(false)
const editingId = ref('')
const currentResource = ref<TunnelMappingOverviewItem>()
const currentRaw = ref<TunnelMapping>()

const editor = reactive({
  local_bind_ip: '0.0.0.0',
  local_bind_port: 0,
  local_base_path: '/',
  enabled: true,
  description: ''
})

const columns = [
  { title: '资源名称', dataIndex: 'name', key: 'name' },
  { title: '资源编码', dataIndex: 'resource_code', key: 'resource_code' },
  { title: '来源节点', dataIndex: 'source_node', key: 'source_node' },
  { title: '方法', dataIndex: 'methods', key: 'methods', width: 180 },
  { title: '响应承载', dataIndex: 'response_mode', key: 'response_mode', width: 110 },
  { title: '映射状态', dataIndex: 'mapping_status', key: 'mapping_status', width: 110 },
  { title: '本地监听', dataIndex: 'listen_ports', key: 'listen_ports' },
  { title: '操作', key: 'action', width: 170 }
]

const rows = computed(() => overview.value?.items ?? [])
const rtpRangeText = computed(() => {
  const start = workspace.value?.localNode?.rtp_port_start
  const end = workspace.value?.localNode?.rtp_port_end
  return start && end ? `[${start}, ${end}]` : '未配置'
})
const mappingRangeText = computed(() => {
  const start = workspace.value?.localNode?.mapping_port_start
  const end = workspace.value?.localNode?.mapping_port_end
  return start && end ? `[${start}, ${end}]` : '未配置'
})

const mappingStatusLabel = (value: string) => value === 'MANUAL' ? '手工映射' : '未映射'

const formatListenEntry = (item: TunnelMappingOverviewItem) => {
  const ports = Array.isArray(item.listen_ports) ? item.listen_ports : []
  if (!ports.length) return '-'
  const ip = item.listen_ip || workspace.value?.localNode?.node_ip || '127.0.0.1'
  const path = item.path_prefix || '/'
  return `${ip}:${ports.join(',')}${path}`
}

const load = async (resetNotice = true) => {
  loading.value = true
  error.value = ''
  if (resetNotice) notice.value = ''
  try {
    const [overviewResp, catalogResp, mappingsResp, workspaceResp] = await Promise.all([
      gatewayApi.fetchTunnelMappingOverview(),
      gatewayApi.fetchTunnelCatalog(),
      gatewayApi.fetchMappings(),
      gatewayApi.fetchNodeTunnelWorkspace()
    ])
    overview.value = overviewResp
    catalog.value = catalogResp
    rawMappings.value = mappingsResp.items
    workspace.value = workspaceResp
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载隧道映射失败'
  } finally {
    loading.value = false
  }
}


const runCatalogAction = async (action: TunnelCatalogActionPayload['action']) => {
  testing.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await gatewayApi.triggerTunnelCatalogAction({ action })
    const label = action === 'pull_remote' ? '手动拉取目录' : action === 'push_local' ? '手动推送目录' : '目录刷新'
    notice.value = `目录动作已执行：${label}（拉取=${Number(result?.subscribe_triggered || 0)}，推送=${Number(result?.notify_triggered || 0)}）`
    await load(false)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '执行目录动作失败'
  } finally {
    testing.value = false
  }
}

const runTest = async () => {
  testing.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await gatewayApi.testMapping()
    if (result.status === 'passed') {
      notice.value = '链路自检完成：通过。'
    } else {
      error.value = result.failure_reason || result.failure_stage || result.suggested_action || '请检查本地监听、注册状态与对端连通性。'
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '执行链路自检失败'
  } finally {
    testing.value = false
  }
}

const findBaseMapping = (resource: TunnelMappingOverviewItem) => {
  const ids = resource.mapping_ids || []
  const manual = rawMappings.value.find((item) => ids.includes(item.mapping_id) || item.device_id === resource.device_id)
  if (manual) return { mapping: manual, editingId: manual.mapping_id }
  const profile = deriveBodyLimitProfile(resource.response_mode, workspace.value?.networkMode)
  return {
    mapping: {
      mapping_id: resource.resource_code || resource.device_id,
      device_id: resource.resource_code || resource.device_id,
      resource_code: resource.resource_code || resource.device_id,
      resource_type: resource.resource_type || 'SERVICE',
      name: resource.name,
      enabled: true,
      peer_node_id: '',
      local_bind_ip: '0.0.0.0',
      local_bind_port: resource.listen_ports?.[0] || 0,
      local_base_path: '/',
      remote_target_ip: '127.0.0.1',
      remote_target_port: 80,
      remote_base_path: '/',
      allowed_methods: resource.methods,
      response_mode: resource.response_mode,
      connect_timeout_ms: 3000,
      request_timeout_ms: 15000,
      response_timeout_ms: 15000,
      max_inline_response_body: profile.maxInlineResponseBody,
      max_request_body_bytes: profile.maxRequestBody,
      max_response_body_bytes: profile.maxResponseBody,
      require_streaming_response: resource.response_mode !== 'INLINE',
      description: ''
    } as TunnelMapping,
    editingId: ''
  }
}

const openMapping = (resource: TunnelMappingOverviewItem) => {
  currentResource.value = resource
  const base = findBaseMapping(resource)
  currentRaw.value = base.mapping
  editingId.value = base.editingId
  Object.assign(editor, {
    local_bind_ip: base.mapping.local_bind_ip || '0.0.0.0',
    local_bind_port: base.mapping.local_bind_port || null,
    local_base_path: base.mapping.local_base_path || '/',
    enabled: base.mapping.enabled ?? true,
    description: base.mapping.description || ''
  })
  editorOpen.value = true
}

const openDetail = (resource: TunnelMappingOverviewItem) => {
  currentResource.value = resource
  currentRaw.value = rawMappings.value.find((item) => (resource.mapping_ids || []).includes(item.mapping_id) || item.device_id === resource.device_id)
  detailOpen.value = true
}

const saveMapping = async () => {
  if (!currentResource.value) return
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const resourceCode = currentResource.value.resource_code || currentResource.value.device_id
    if (!isGBCode20(resourceCode)) {
      throw new Error('资源编码必须为 20 位国标编码')
    }
    const sipPort = workspace.value?.localNode?.signaling_port
    const rtpStart = workspace.value?.localNode?.rtp_port_start
    const rtpEnd = workspace.value?.localNode?.rtp_port_end
    const mappingStart = workspace.value?.localNode?.mapping_port_start
    const mappingEnd = workspace.value?.localNode?.mapping_port_end
    if (!editor.local_bind_port) throw new Error('监听端口必须手工填写，系统不再自动分配端口')
    if (rows.value.some((item) => Array.isArray(item.listen_ports) && item.listen_ports.includes(Number(editor.local_bind_port || 0)) && item.resource_code !== currentResource.value?.resource_code)) {
      throw new Error(`监听端口 ${editor.local_bind_port} 已被其他映射占用`)
    }
    if (sipPort && editor.local_bind_port === sipPort) throw new Error(`监听端口 ${editor.local_bind_port} 不能与本端 SIP 监听端口 ${sipPort} 相同`)
    if (rtpStart && rtpEnd && editor.local_bind_port >= rtpStart && editor.local_bind_port <= rtpEnd) {
      throw new Error(`监听端口 ${editor.local_bind_port} 不能落入本端 RTP 端口范围 ${rtpRangeText.value}`)
    }
    if (mappingStart && mappingEnd && (editor.local_bind_port < mappingStart || editor.local_bind_port > mappingEnd)) {
      throw new Error(`监听端口必须落在本地映射端口范围 ${mappingRangeText.value} 内`)
    }
    const profile = deriveBodyLimitProfile(currentResource.value.response_mode, workspace.value?.networkMode)
    const base = currentRaw.value || findBaseMapping(currentResource.value).mapping
    const payload: TunnelMapping = {
      ...base,
      mapping_id: editingId.value || base.mapping_id || resourceCode,
      device_id: resourceCode,
      resource_code: resourceCode,
      resource_type: currentResource.value.resource_type || base.resource_type || 'SERVICE',
      name: currentResource.value.name,
      enabled: editor.enabled,
      local_bind_ip: editor.local_bind_ip.trim() || '0.0.0.0',
      local_bind_port: Number(editor.local_bind_port || 0),
      local_base_path: editor.local_base_path.trim() || '/',
      allowed_methods: currentResource.value.methods,
      response_mode: currentResource.value.response_mode,
      connect_timeout_ms: 3000,
      request_timeout_ms: 15000,
      response_timeout_ms: 15000,
      max_inline_response_body: profile.maxInlineResponseBody,
      max_request_body_bytes: profile.maxRequestBody,
      max_response_body_bytes: profile.maxResponseBody,
      require_streaming_response: currentResource.value.response_mode !== 'INLINE',
      description: editor.description || ''
    }
    if (editingId.value) await gatewayApi.updateMapping(editingId.value, payload)
    else await gatewayApi.createMapping(payload)
    editorOpen.value = false
    notice.value = editingId.value ? '隧道映射已更新' : '隧道映射已创建'
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存隧道映射失败'
  } finally {
    saving.value = false
  }
}

const removeMapping = async () => {
  if (!editingId.value) return
  saving.value = true
  error.value = ''
  try {
    await gatewayApi.deleteMapping(editingId.value)
    editorOpen.value = false
    notice.value = '隧道映射已删除'
    await load()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '删除隧道映射失败'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
