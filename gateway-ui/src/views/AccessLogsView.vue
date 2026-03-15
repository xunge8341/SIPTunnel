<template>
  <a-space direction="vertical" style="width: 100%">
    <a-card title="访问日志">
      <a-form layout="inline">
        <a-form-item label="状态">
          <a-select v-model:value="filters.status" style="width: 150px" allow-clear>
            <a-select-option value="running">进行中</a-select-option>
            <a-select-option value="succeeded">成功</a-select-option>
            <a-select-option value="failed">失败</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="请求ID">
          <a-input v-model:value="filters.requestId" allow-clear placeholder="用于定位单次访问" />
        </a-form-item>
        <a-form-item><a-button type="primary" @click="load">查询</a-button></a-form-item>
      </a-form>
    </a-card>
    <a-card>
      <a-table :columns="columns" :data-source="rows" row-key="id" :pagination="false" />
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { CommandTask } from '../types/gateway'

const filters = reactive({ status: undefined as string | undefined, requestId: '' })
const rows = ref<CommandTask[]>([])

const columns = [
  { title: '时间', dataIndex: 'updatedAt', key: 'updatedAt' },
  { title: '映射名称', dataIndex: 'apiCode', key: 'apiCode' },
  { title: '来源节点', dataIndex: 'nodeId', key: 'nodeId' },
  { title: '状态', dataIndex: 'status', key: 'status' },
  { title: '请求ID', dataIndex: 'requestId', key: 'requestId' }
]

const load = async () => {
  const result = await gatewayApi.fetchCommandTasks(filters, 1, 100)
  rows.value = result.list
}

onMounted(load)
</script>
