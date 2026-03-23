<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="本地资源" sub-title="这里只维护本机发布的资源定义：统一使用 20 位国标资源编码，方法采用多选；能从国标编码推导的类型信息不再单独展示。">
      <template #extra>
        <a-space>
          <a-button @click="load">刷新</a-button>
          <a-button :loading="syncingCatalog" @click="pushLocalCatalog">手动推送目录</a-button>
          <a-button type="primary" @click="openCreate">新建资源</a-button>
        </a-space>
      </template>
    </a-page-header>

    <a-alert v-if="notice" type="success" :message="notice" show-icon />
    <a-alert v-if="error" type="error" :message="error" show-icon />
    <a-alert
      type="info"
      show-icon
      :message="`资源编码、本级域编码、级联对端编码统一使用 20 位国标编码；本地资源页只维护资源定义与目标 URL，本地监听端口与路径由“隧道映射”单独管理。`"
    />

    <a-spin :spinning="loading || saving">
      <a-card>
        <a-table :data-source="rows" :columns="columns" row-key="resource_code" :pagination="false" :locale="{ emptyText: '暂无本地资源，请点击右上角“新建资源”。' }">
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'enabled'">
              <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag>
            </template>
            <template v-else-if="column.key === 'methods'">
              {{ record.methods.join(', ') }}
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space>
                <a-button type="link" @click="openEdit(record)">编辑</a-button>
                <a-button type="link" danger @click="removeResource(record)">删除</a-button>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-card>
    </a-spin>

    <a-drawer v-model:open="editorOpen" :title="editingId ? '编辑本地资源' : '新建本地资源'" :width="980" destroy-on-close>
      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :span="12"><a-form-item label="资源名称"><a-input v-model:value="editor.name" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="启用资源"><a-switch v-model:checked="editor.enabled" /></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="24"><a-form-item label="资源编码（20 位国标编码）"><a-space compact style="width: 100%"><a-input v-model:value="editor.resource_code" class="gb-code-input" maxlength="20" placeholder="例如 34020000001320000001" style="width: calc(100% - 104px); min-width: 360px" /><a-tooltip title="按服务器类型生成编码"><a-button style="width: 52px" @click="generateResourceCode('SERVICE')"><ApiOutlined /></a-button></a-tooltip><a-tooltip title="按摄像机类型生成编码"><a-button style="width: 52px" @click="generateResourceCode('CAMERA')"><VideoCameraOutlined /></a-button></a-tooltip></a-space></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="24"><a-form-item label="目标 URL"><a-input v-model:value="editor.target_url" placeholder="http://127.0.0.1:8080/api/orders" /></a-form-item></a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :span="12"><a-form-item label="允许方法"><a-select v-model:value="editor.methods" mode="multiple" :options="methodOptions" placeholder="请选择允许的方法" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="响应承载"><a-select v-model:value="editor.response_mode" :options="responseModeOptions" /></a-form-item></a-col>
        </a-row>
        <a-card size="small" title="自动推导的链路体量上限" :bordered="false" style="margin-bottom: 16px">
          <a-descriptions :column="1" size="small" bordered>
            <a-descriptions-item label="策略">{{ limitProfile.policyLabel }}</a-descriptions-item>
            <a-descriptions-item label="最大内联响应体">{{ formatBytes(limitProfile.maxInlineResponseBody) }}</a-descriptions-item>
            <a-descriptions-item label="最大请求体">{{ formatBytes(limitProfile.maxRequestBody) }}</a-descriptions-item>
            <a-descriptions-item label="最大响应体">{{ formatBytes(limitProfile.maxResponseBody) }}</a-descriptions-item>
          </a-descriptions>
        </a-card>
        <a-form-item label="描述"><a-textarea v-model:value="editor.description" :auto-size="{ minRows: 2, maxRows: 4 }" /></a-form-item>
      </a-form>
      <template #footer>
        <a-space style="float: right">
          <a-button @click="editorOpen = false">取消</a-button>
          <a-button type="primary" :loading="saving" @click="saveResource">保存</a-button>
        </a-space>
      </template>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Modal } from 'ant-design-vue'
import { ApiOutlined, VideoCameraOutlined } from '@ant-design/icons-vue'
import { gatewayApi } from '../api/gateway'
import type { LocalResourceItem, NodeTunnelWorkspace, TunnelCatalogActionResponse, LocalResourceSavePayload } from '../types/gateway'
import { deriveBodyLimitProfile, generateGBCode, isGBCode20, methodOptions, normalizeMethodSelection, responseModeOptions } from '../utils/gb28181'

interface LocalResourceEditor {
  resource_code: string
  name: string
  enabled: boolean
  target_url: string
  methods: string[]
  response_mode: 'AUTO' | 'INLINE' | 'RTP'
  description: string
}

const loading = ref(false)
const saving = ref(false)
const syncingCatalog = ref(false)
const error = ref('')
const notice = ref('')
const rows = ref<LocalResourceItem[]>([])
const workspace = ref<NodeTunnelWorkspace>()
const editorOpen = ref(false)
const editingId = ref('')

const editor = reactive<LocalResourceEditor>({
  resource_code: '',
  name: '',
  enabled: true,
  target_url: 'http://127.0.0.1:8080/',
  methods: ['GET', 'POST'],
  response_mode: 'RTP',
  description: ''
})

const columns = [
  { title: '资源名称', dataIndex: 'name', key: 'name' },
  { title: '资源编码', dataIndex: 'resource_code', key: 'resource_code' },
  { title: '目标 URL', dataIndex: 'target_url', key: 'target_url' },
  { title: '方法', dataIndex: 'methods', key: 'methods', width: 180 },
  { title: '响应承载', dataIndex: 'response_mode', key: 'response_mode', width: 110 },
  { title: '自动体量策略', dataIndex: 'body_limit_policy', key: 'body_limit_policy', width: 120 },
  { title: '状态', dataIndex: 'enabled', key: 'enabled', width: 90 },
  { title: '操作', key: 'action', width: 160 }
]

const limitProfile = computed(() => deriveBodyLimitProfile(editor.response_mode, workspace.value?.networkMode))

const formatBytes = (value: number) => {
  if (value >= 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(0)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(0)} KB`
  return `${value} B`
}


const generateResourceCode = (kind: 'SERVICE' | 'CAMERA' = 'SERVICE') => {
  editor.resource_code = generateGBCode(kind)
}

const pushLocalCatalog = async () => {
  syncingCatalog.value = true
  error.value = ''
  try {
    const result: TunnelCatalogActionResponse = await gatewayApi.triggerTunnelCatalogAction({ action: 'push_local' })
    notice.value = `目录推送已触发（推送=${Number(result.notify_triggered || 0)}）`
    await load(false)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '手动推送目录失败'
  } finally {
    syncingCatalog.value = false
  }
}


const resetEditor = () => {
  Object.assign(editor, {
    resource_code: '',
    name: '',
    enabled: true,
    target_url: 'http://127.0.0.1:8080/',
    methods: ['GET', 'POST'],
    response_mode: 'RTP',
    description: ''
  })
}

const localResourceToEditor = (item: LocalResourceItem): LocalResourceEditor => ({
  resource_code: item.resource_code || item.device_id || item.resource_id,
  name: item.name || item.resource_code,
  enabled: item.enabled,
  target_url: item.target_url,
  methods: normalizeMethodSelection(item.methods),
  response_mode: item.response_mode || 'RTP',
  description: item.description || ''
})

const editorToPayload = (): LocalResourceSavePayload => {
  if (!isGBCode20(editor.resource_code)) {
    throw new Error('资源编码必须为 20 位国标编码')
  }
  const code = editor.resource_code.trim()
  if (rows.value.some((item) => item.resource_code === code && (editingId.value ? item.resource_code !== editingId.value : true))) {
    throw new Error(`资源编码 ${code} 已存在，请勿重复使用`)
  }
  try {
    new URL(editor.target_url)
  } catch {
    throw new Error('目标 URL 格式不正确')
  }
  return {
    resource_code: code,
    name: editor.name.trim(),
    enabled: editor.enabled,
    target_url: editor.target_url.trim(),
    methods: normalizeMethodSelection(editor.methods),
    response_mode: editor.response_mode,
    description: editor.description || ''
  }
}

const load = async (resetNotice = true) => {
  loading.value = true
  error.value = ''
  if (resetNotice) notice.value = ''
  try {
    const [list, nodeWorkspace] = await Promise.all([
      gatewayApi.fetchLocalResources(),
      gatewayApi.fetchNodeTunnelWorkspace()
    ])
    rows.value = list.items
    workspace.value = nodeWorkspace
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载本地资源失败'
  } finally {
    loading.value = false
  }
}

const openCreate = () => {
  editingId.value = ''
  resetEditor()
  editorOpen.value = true
}

const openEdit = (row: LocalResourceItem) => {
  editingId.value = row.resource_code
  Object.assign(editor, localResourceToEditor(row))
  editorOpen.value = true
}

const saveResource = async () => {
  if (!String(editor.resource_code || '').trim()) {
    generateResourceCode('SERVICE')
  }
  saving.value = true
  error.value = ''
  try {
    const payload = editorToPayload()
    if (editingId.value) {
      await gatewayApi.updateLocalResource(editingId.value, payload)
      notice.value = '本地资源已更新'
    } else {
      await gatewayApi.createLocalResource(payload)
      notice.value = '本地资源已创建'
    }
    editorOpen.value = false
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '保存本地资源失败'
  } finally {
    saving.value = false
  }
}

const removeResource = (row: LocalResourceItem) => {
  Modal.confirm({
    title: `删除本地资源：${row.name}`,
    content: '删除后将不再参与本地目录发布，是否继续？',
    okButtonProps: { danger: true },
    onOk: async () => {
      try {
        await gatewayApi.deleteLocalResource(row.resource_code || row.resource_id)
        notice.value = '本地资源已删除'
        await load()
      } catch (err) {
        error.value = err instanceof Error ? err.message : '删除本地资源失败'
      }
    }
  })
}

onMounted(load)
</script>

<style scoped>
.gb-code-input :deep(input) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  letter-spacing: 0.5px;
  min-width: 360px;
}
</style>
