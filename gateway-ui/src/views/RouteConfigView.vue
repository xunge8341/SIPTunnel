<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="路由配置查询">
      <a-form layout="inline">
        <a-form-item label="api_code">
          <a-input v-model:value="keyword" allow-clear placeholder="ORDER_SYNC" />
        </a-form-item>
      </a-form>
    </a-card>

    <a-card title="路由配置列表">
      <a-table :columns="columns" :data-source="filteredRoutes" row-key="apiCode" :pagination="false">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'mappingOverview'">
            <a-space>
              <a-tag color="blue">Header {{ record.headerMapping.length }}</a-tag>
              <a-tag color="purple">Body {{ record.bodyMapping.length }}</a-tag>
            </a-space>
          </template>
          <template v-if="column.key === 'action'">
            <a-button type="link" @click="openEditor(record)">编辑</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="drawerVisible" title="编辑路由配置" width="520" @close="drawerVisible = false">
      <a-form layout="vertical">
        <a-form-item label="api_code"><a-input v-model:value="editingRoute.apiCode" disabled /></a-form-item>
        <a-form-item label="target_service"><a-input v-model:value="editingRoute.targetService" /></a-form-item>
        <a-row :gutter="12">
          <a-col :span="12"><a-form-item label="method"><a-select v-model:value="editingRoute.method" :options="methodOptions" /></a-form-item></a-col>
          <a-col :span="12"><a-form-item label="timeout(ms)"><a-input-number v-model:value="editingRoute.timeout" :min="100" style="width: 100%" /></a-form-item></a-col>
        </a-row>
        <a-form-item label="path"><a-input v-model:value="editingRoute.path" /></a-form-item>
        <a-form-item label="retry"><a-input-number v-model:value="editingRoute.retry" :min="0" :max="5" style="width: 100%" /></a-form-item>
        <a-divider>Header Mapping 概览</a-divider>
        <a-space wrap>
          <a-tag v-for="item in editingRoute.headerMapping" :key="item">{{ item }}</a-tag>
        </a-space>
        <a-divider>Body Mapping 概览</a-divider>
        <a-space wrap>
          <a-tag color="purple" v-for="item in editingRoute.bodyMapping" :key="item">{{ item }}</a-tag>
        </a-space>
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
import { computed, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'

interface RouteItem {
  apiCode: string
  targetService: string
  method: 'GET' | 'POST' | 'PUT'
  path: string
  timeout: number
  retry: number
  headerMapping: string[]
  bodyMapping: string[]
}

const methodOptions = [
  { label: 'GET', value: 'GET' },
  { label: 'POST', value: 'POST' },
  { label: 'PUT', value: 'PUT' }
]

const keyword = ref('')
const drawerVisible = ref(false)

const routes = ref<RouteItem[]>([
  {
    apiCode: 'ORDER_SYNC',
    targetService: 'order-service',
    method: 'POST',
    path: '/internal/order/sync',
    timeout: 3000,
    retry: 2,
    headerMapping: ['x-request-id <- request_id', 'x-trace-id <- trace_id'],
    bodyMapping: ['order_id <- payload.orderId', 'items <- payload.items']
  },
  {
    apiCode: 'USER_QUERY',
    targetService: 'user-service',
    method: 'GET',
    path: '/internal/user/query',
    timeout: 2000,
    retry: 1,
    headerMapping: ['x-request-id <- request_id'],
    bodyMapping: ['user_id <- payload.userId']
  }
])

const editingRoute = reactive<RouteItem>({
  apiCode: '',
  targetService: '',
  method: 'POST',
  path: '',
  timeout: 1000,
  retry: 0,
  headerMapping: [],
  bodyMapping: []
})

const columns = [
  { title: 'api_code', dataIndex: 'apiCode', key: 'apiCode' },
  { title: 'target_service', dataIndex: 'targetService', key: 'targetService' },
  { title: 'method', dataIndex: 'method', key: 'method' },
  { title: 'path', dataIndex: 'path', key: 'path' },
  { title: 'timeout', dataIndex: 'timeout', key: 'timeout' },
  { title: 'retry', dataIndex: 'retry', key: 'retry' },
  { title: 'mapping 概览', key: 'mappingOverview' },
  { title: '操作', key: 'action' }
]

const filteredRoutes = computed(() =>
  routes.value.filter((item) => item.apiCode.toLowerCase().includes(keyword.value.trim().toLowerCase()))
)

const openEditor = (route: RouteItem) => {
  Object.assign(editingRoute, JSON.parse(JSON.stringify(route)))
  drawerVisible.value = true
}

const saveRoute = () => {
  const index = routes.value.findIndex((item) => item.apiCode === editingRoute.apiCode)
  if (index >= 0) {
    routes.value[index] = JSON.parse(JSON.stringify(editingRoute))
  }
  message.success('路由配置已保存')
  drawerVisible.value = false
}
</script>
