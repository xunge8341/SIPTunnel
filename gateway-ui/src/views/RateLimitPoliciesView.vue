<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="限流策略">
      <a-tabs v-model:activeKey="activeTab">
        <a-tab-pane key="global" tab="全局限流" />
        <a-tab-pane key="source" tab="来源系统限流" />
        <a-tab-pane key="apiCode" tab="api_code 限流" />
        <a-tab-pane key="target" tab="目标服务限流" />
      </a-tabs>

      <a-table :columns="columnsByTab[activeTab]" :data-source="dataByTab[activeTab]" row-key="id" :pagination="false">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-switch :checked="record.enabled" disabled />
          </template>
          <template v-if="column.key === 'action'">
            <a-button type="link" @click="openEdit(record)">编辑</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal v-model:open="editVisible" title="编辑限流策略" @ok="savePolicy">
      <a-form layout="vertical">
        <a-form-item :label="currentLabel">
          <a-input :value="editingPolicy?.name" disabled />
        </a-form-item>
        <a-row :gutter="12">
          <a-col :span="12">
            <a-form-item label="QPS">
              <a-input-number v-model:value="formState.qps" :min="1" style="width: 100%" />
            </a-form-item>
          </a-col>
          <a-col :span="12">
            <a-form-item label="Burst">
              <a-input-number v-model:value="formState.burst" :min="1" style="width: 100%" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-form-item label="启用状态">
          <a-switch v-model:checked="formState.enabled" />
        </a-form-item>
      </a-form>
    </a-modal>
  </a-space>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'

type PolicyType = 'global' | 'source' | 'apiCode' | 'target'

interface PolicyItem {
  id: string
  name: string
  qps: number
  burst: number
  enabled: boolean
}

const activeTab = ref<PolicyType>('global')
const editVisible = ref(false)
const editingPolicy = ref<PolicyItem | null>(null)
const formState = reactive({ qps: 2000, burst: 4000, enabled: true })

const globalPolicies = ref<PolicyItem[]>([{ id: 'global', name: '全局默认', qps: 2000, burst: 4000, enabled: true }])
const sourcePolicies = ref<PolicyItem[]>([
  { id: 'src-erp', name: 'ERP', qps: 800, burst: 1200, enabled: true },
  { id: 'src-crm', name: 'CRM', qps: 500, burst: 900, enabled: true }
])
const apiCodePolicies = ref<PolicyItem[]>([
  { id: 'api-order', name: 'ORDER_SYNC', qps: 300, burst: 480, enabled: true },
  { id: 'api-user', name: 'USER_QUERY', qps: 600, burst: 900, enabled: true }
])
const targetPolicies = ref<PolicyItem[]>([
  { id: 'target-order', name: 'order-service', qps: 450, burst: 700, enabled: true },
  { id: 'target-policy', name: 'policy-service', qps: 260, burst: 390, enabled: false }
])

interface TableColumn {
  title: string
  dataIndex?: string
  key: string
}

const columnsByTab: Record<PolicyType, TableColumn[]> = {
  global: [
    { title: '策略名', dataIndex: 'name', key: 'name' },
    { title: 'QPS', dataIndex: 'qps', key: 'qps' },
    { title: 'Burst', dataIndex: 'burst', key: 'burst' },
    { title: '启用', key: 'enabled' },
    { title: '操作', key: 'action' }
  ],
  source: [
    { title: '来源系统', dataIndex: 'name', key: 'name' },
    { title: 'QPS', dataIndex: 'qps', key: 'qps' },
    { title: 'Burst', dataIndex: 'burst', key: 'burst' },
    { title: '启用', key: 'enabled' },
    { title: '操作', key: 'action' }
  ],
  apiCode: [
    { title: 'api_code', dataIndex: 'name', key: 'name' },
    { title: 'QPS', dataIndex: 'qps', key: 'qps' },
    { title: 'Burst', dataIndex: 'burst', key: 'burst' },
    { title: '启用', key: 'enabled' },
    { title: '操作', key: 'action' }
  ],
  target: [
    { title: '目标服务', dataIndex: 'name', key: 'name' },
    { title: 'QPS', dataIndex: 'qps', key: 'qps' },
    { title: 'Burst', dataIndex: 'burst', key: 'burst' },
    { title: '启用', key: 'enabled' },
    { title: '操作', key: 'action' }
  ]
}

const dataByTab = computed<Record<PolicyType, PolicyItem[]>>(() => ({
  global: globalPolicies.value,
  source: sourcePolicies.value,
  apiCode: apiCodePolicies.value,
  target: targetPolicies.value
}))

const currentLabel = computed(() => columnsByTab[activeTab.value][0].title as string)

const openEdit = (record: PolicyItem) => {
  editingPolicy.value = record
  formState.qps = record.qps
  formState.burst = record.burst
  formState.enabled = record.enabled
  editVisible.value = true
}

const savePolicy = () => {
  if (!editingPolicy.value) return
  const list = dataByTab.value[activeTab.value]
  const target = list.find((item) => item.id === editingPolicy.value?.id)
  if (!target) return
  target.qps = formState.qps
  target.burst = formState.burst
  target.enabled = formState.enabled
  message.success('限流策略已保存')
  editVisible.value = false
}
</script>
