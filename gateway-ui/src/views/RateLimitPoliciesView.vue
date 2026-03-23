<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="限流策略">
      <a-table :columns="columns" :data-source="[limits]" row-key="id" :pagination="false">
        <template #bodyCell="{ column }">
          <template v-if="column.key === 'action'">
            <a-button type="link" @click="openEdit">编辑</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal v-model:open="editVisible" title="编辑限流策略" @ok="savePolicy">
      <a-form layout="vertical">
        <a-form-item label="RPS">
          <a-input-number v-model:value="formState.rps" :min="1" style="width: 100%" />
        </a-form-item>
        <a-form-item label="Burst">
          <a-input-number v-model:value="formState.burst" :min="1" style="width: 100%" />
        </a-form-item>
        <a-form-item label="最大并发">
          <a-input-number v-model:value="formState.maxConcurrent" :min="1" style="width: 100%" />
        </a-form-item>
      </a-form>
    </a-modal>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'

const editVisible = ref(false)
const limits = reactive({ id: 'global', rps: 0, burst: 0, maxConcurrent: 0 })
const formState = reactive({ rps: 0, burst: 0, maxConcurrent: 0 })

const columns = [
  { title: '策略名', dataIndex: 'id', key: 'id' },
  { title: 'RPS', dataIndex: 'rps', key: 'rps' },
  { title: 'Burst', dataIndex: 'burst', key: 'burst' },
  { title: '最大并发', dataIndex: 'maxConcurrent', key: 'maxConcurrent' },
  { title: '操作', key: 'action' }
]

const loadLimits = async () => {
  const data = await gatewayApi.fetchLimits()
  Object.assign(limits, { id: 'global', ...data })
}

const openEdit = () => {
  Object.assign(formState, limits)
  editVisible.value = true
}

const savePolicy = async () => {
  const saved = await gatewayApi.updateLimits({ rps: formState.rps, burst: formState.burst, maxConcurrent: formState.maxConcurrent })
  Object.assign(limits, { id: 'global', ...saved })
  message.success('限流策略已保存')
  editVisible.value = false
}

onMounted(loadLimits)
</script>
