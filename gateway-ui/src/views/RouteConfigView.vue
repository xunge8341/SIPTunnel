<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="路由配置查询">
      <a-form layout="inline">
        <a-form-item label="api_code">
          <a-input v-model:value="keyword" allow-clear placeholder="asset.sync" />
        </a-form-item>
      </a-form>
    </a-card>

    <a-card title="路由配置列表">
      <a-table :columns="columns" :data-source="filteredRoutes" row-key="api_code" :pagination="false">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'enabled'">
            <a-switch :checked="record.enabled" disabled />
          </template>
          <template v-if="column.key === 'action'">
            <a-button type="link" @click="openEditor(record)">编辑</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="drawerVisible" title="编辑路由配置" width="520" @close="drawerVisible = false">
      <a-form layout="vertical">
        <a-form-item label="api_code"><a-input v-model:value="editingRoute.api_code" disabled /></a-form-item>
        <a-row :gutter="12">
          <a-col :span="12"><a-form-item label="method"><a-select v-model:value="editingRoute.http_method" :options="methodOptions" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="path"><a-input v-model:value="editingRoute.http_path" /></a-form-item></a-col>
        </a-row>
        <a-form-item label="启用">
          <a-switch v-model:checked="editingRoute.enabled" />
        </a-form-item>
      </a-form>
      <template #footer>
        <a-space style="width: 100%; justify-content: flex-end">
          <a-button @click="drawerVisible = false">取消</a-button>
          <a-button type="primary" @click="saveRoute">保存</a-button>
        </a-space>
      </template>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { OpsRoute } from '../types/gateway'

const methodOptions = [
  { label: 'GET', value: 'GET' },
  { label: 'POST', value: 'POST' },
  { label: 'PUT', value: 'PUT' },
  { label: 'DELETE', value: 'DELETE' }
]

const keyword = ref('')
const drawerVisible = ref(false)
const routes = ref<OpsRoute[]>([])

const editingRoute = reactive<OpsRoute>({
  api_code: '',
  http_method: 'POST',
  http_path: '',
  enabled: true
})

const columns = [
  { title: 'api_code', dataIndex: 'api_code', key: 'api_code' },
  { title: 'method', dataIndex: 'http_method', key: 'http_method' },
  { title: 'path', dataIndex: 'http_path', key: 'http_path' },
  { title: '启用', key: 'enabled' },
  { title: '操作', key: 'action' }
]

const filteredRoutes = computed(() =>
  routes.value.filter((item) => item.api_code.toLowerCase().includes(keyword.value.trim().toLowerCase()))
)

const openEditor = (route: OpsRoute) => {
  Object.assign(editingRoute, JSON.parse(JSON.stringify(route)))
  drawerVisible.value = true
}

const loadRoutes = async () => {
  routes.value = await gatewayApi.fetchRoutes()
}

const saveRoute = async () => {
  const index = routes.value.findIndex((item) => item.api_code === editingRoute.api_code)
  if (index >= 0) {
    routes.value[index] = JSON.parse(JSON.stringify(editingRoute))
  }
  routes.value = await gatewayApi.updateRoutes(routes.value)
  message.success('路由配置已保存')
  drawerVisible.value = false
}

onMounted(loadRoutes)
</script>
