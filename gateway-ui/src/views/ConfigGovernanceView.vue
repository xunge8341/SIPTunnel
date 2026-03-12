<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card>
      <a-space style="width: 100%; justify-content: space-between" align="start">
        <a-space direction="vertical" size="small">
          <a-typography-title :level="5" style="margin: 0">配置治理</a-typography-title>
          <a-typography-text type="secondary">配置快照、当前生效配置、待发布配置与差异对比集中治理。</a-typography-text>
        </a-space>
        <a-space>
          <a-button @click="handleExportYaml('current')">导出生效 YAML</a-button>
          <a-button type="primary" @click="handleExportYaml('pending')">导出待发布 YAML</a-button>
        </a-space>
      </a-space>
    </a-card>

    <a-card title="筛选条件">
      <a-form layout="inline">
        <a-form-item label="时间范围">
          <a-range-picker v-model:value="timeRange" show-time value-format="YYYY-MM-DD HH:mm:ss" />
        </a-form-item>
        <a-form-item label="操作人">
          <a-input v-model:value="filters.operator" placeholder="例如 ops_admin" allow-clear style="width: 180px" />
        </a-form-item>
        <a-form-item label="版本号">
          <a-input v-model:value="filters.version" placeholder="例如 v2026.03" allow-clear style="width: 180px" />
        </a-form-item>
        <a-form-item>
          <a-space>
            <a-button type="primary" :loading="loading" @click="loadData">查询</a-button>
            <a-button @click="resetFilters">重置</a-button>
          </a-space>
        </a-form-item>
      </a-form>
    </a-card>

    <a-card title="配置快照列表">
      <a-table :columns="snapshotColumns" :data-source="state.snapshots" row-key="version" :pagination="false" size="middle">
        <template #bodyCell="{ column, record }">
          <template v-if="column.dataIndex === 'status'">
            <a-tag :color="statusColor(record.status)">{{ statusLabel(record.status) }}</a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button size="small" @click="openDrawer(record.version)">查看详情</a-button>
              <a-button
                size="small"
                danger
                :disabled="record.status === 'active'"
                @click="confirmRollback(record.version)"
              >
                回滚到此版本
              </a-button>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="drawerOpen" width="980" title="配置详情与差异视图" destroy-on-close>
      <a-tabs v-model:activeKey="activeTab">
        <a-tab-pane key="current" tab="当前生效配置">
          <pre class="config-panel">{{ stringifyConfig(state.currentConfig) }}</pre>
        </a-tab-pane>
        <a-tab-pane key="pending" tab="待发布配置">
          <pre class="config-panel">{{ stringifyConfig(state.pendingConfig) }}</pre>
        </a-tab-pane>
        <a-tab-pane key="diff" tab="配置差异对比">
          <a-table :columns="diffColumns" :data-source="state.diff" row-key="path" :pagination="false" size="small">
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'path'">
                <span :class="['diff-path', riskFieldSet.has(record.path) ? 'is-risk' : '']">{{ record.path }}</span>
              </template>
              <template v-else-if="column.dataIndex === 'before'">
                <span class="diff-before">- {{ record.before }}</span>
              </template>
              <template v-else-if="column.dataIndex === 'after'">
                <span class="diff-after">+ {{ record.after }}</span>
              </template>
              <template v-else-if="column.dataIndex === 'riskLevel'">
                <a-tag :color="record.riskLevel === 'high' ? 'red' : record.riskLevel === 'medium' ? 'orange' : 'blue'">
                  {{ record.riskLevel.toUpperCase() }}
                </a-tag>
              </template>
            </template>
          </a-table>
        </a-tab-pane>
      </a-tabs>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message, Modal } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { ConfigDiffItem, ConfigGovernancePayload, ConfigSnapshotFilters, RuntimeGatewayConfig } from '../types/gateway'

const riskFieldSet = new Set([
  'sip.listen_port',
  'sip.transport',
  'rtp.port_start',
  'rtp.port_end',
  'rtp.transport',
  'max_message_bytes'
])

const loading = ref(false)
const drawerOpen = ref(false)
const activeTab = ref('current')
const timeRange = ref<[string, string]>()

const filters = reactive<ConfigSnapshotFilters>({
  operator: '',
  version: ''
})

const state = reactive<ConfigGovernancePayload>({
  snapshots: [],
  currentConfig: {
    sip: { listen_port: 5060, transport: 'UDP', listen_ip: '0.0.0.0' },
    rtp: { port_start: 20000, port_end: 20999, transport: 'UDP', listen_ip: '0.0.0.0' },
    max_message_bytes: 1048576,
    heartbeat_interval_sec: 15
  },
  pendingConfig: {
    sip: { listen_port: 5060, transport: 'UDP', listen_ip: '0.0.0.0' },
    rtp: { port_start: 20000, port_end: 20999, transport: 'UDP', listen_ip: '0.0.0.0' },
    max_message_bytes: 1048576,
    heartbeat_interval_sec: 15
  },
  diff: []
})

const snapshotColumns = [
  { title: '版本号', dataIndex: 'version', key: 'version' },
  { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt' },
  { title: '操作人', dataIndex: 'operator', key: 'operator' },
  { title: '变更说明', dataIndex: 'changeSummary', key: 'changeSummary' },
  { title: '状态', dataIndex: 'status', key: 'status' },
  { title: '操作', key: 'action', width: 220 }
]

const diffColumns: Array<{ title: string; dataIndex: keyof ConfigDiffItem; key: string; width?: number }> = [
  { title: '字段路径', dataIndex: 'path', key: 'path', width: 250 },
  { title: '旧值', dataIndex: 'before', key: 'before' },
  { title: '新值', dataIndex: 'after', key: 'after' },
  { title: '风险级别', dataIndex: 'riskLevel', key: 'riskLevel', width: 120 }
]

const queryFilters = computed(() => ({
  startTime: timeRange.value?.[0],
  endTime: timeRange.value?.[1],
  operator: filters.operator?.trim() || undefined,
  version: filters.version?.trim() || undefined
}))

const stringifyConfig = (config: RuntimeGatewayConfig) => JSON.stringify(config, null, 2)

const statusColor = (status: string) => ({ active: 'green', pending: 'gold', archived: 'default' }[status] ?? 'default')
const statusLabel = (status: string) => ({ active: '生效中', pending: '待发布', archived: '历史' }[status] ?? status)

const loadData = async () => {
  loading.value = true
  try {
    const payload = await gatewayApi.fetchConfigGovernance(queryFilters.value)
    Object.assign(state, payload)
  } finally {
    loading.value = false
  }
}

const openDrawer = (tab: string) => {
  drawerOpen.value = true
  activeTab.value = tab.includes('pending') ? 'pending' : 'current'
}

const confirmRollback = (version: string) => {
  Modal.confirm({
    title: '确认回滚配置',
    content: `将回滚到 ${version}，该操作可能影响 SIP/RTP 通信。是否继续？`,
    okText: '确认回滚',
    cancelText: '取消',
    okButtonProps: { danger: true },
    onOk: async () => {
      const payload = await gatewayApi.rollbackConfig(version)
      Object.assign(state, payload)
      message.success(`已回滚到 ${version}`)
    }
  })
}

const handleExportYaml = async (target: 'current' | 'pending') => {
  const yaml = await gatewayApi.exportConfigYaml(target)
  await navigator.clipboard.writeText(yaml)
  message.success(`${target === 'current' ? '生效配置' : '待发布配置'} YAML 已复制到剪贴板`)
}

const resetFilters = () => {
  filters.operator = ''
  filters.version = ''
  timeRange.value = undefined
  loadData()
}

onMounted(loadData)
</script>

<style scoped>
.config-panel {
  background: #0f172a;
  color: #dbeafe;
  border-radius: 8px;
  padding: 12px;
  min-height: 380px;
  overflow: auto;
}

.diff-path {
  font-family: 'SFMono-Regular', Consolas, monospace;
}

.diff-path.is-risk {
  color: #cf1322;
  font-weight: 600;
  background: #fff1f0;
  padding: 2px 6px;
  border-radius: 4px;
}

.diff-before {
  color: #a8071a;
  background: #fff1f0;
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
}

.diff-after {
  color: #237804;
  background: #f6ffed;
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
}
</style>
