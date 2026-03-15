<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-card :bordered="false">
      <a-page-header title="隧道映射" sub-title="按运维工作台设计：筛选、批量动作、主表格与详情抽屉。">
        <template #extra>
          <a-space wrap>
            <a-button type="primary" @click="openEditor()">新建映射</a-button>
            <a-button>全部测试</a-button>
            <a-button>批量测试</a-button>
            <a-button>批量启用</a-button>
            <a-button>批量停用</a-button>
            <a-button>导出配置</a-button>
          </a-space>
        </template>
      </a-page-header>
      <a-form layout="inline">
        <a-form-item label="关键字"><a-input v-model:value="filters.keyword" placeholder="映射名称/入口/目标" allow-clear /></a-form-item>
        <a-form-item label="状态"><a-select v-model:value="filters.status" :options="statusOptions" style="width: 120px" /></a-form-item>
        <a-form-item label="最近测试"><a-select v-model:value="filters.test" :options="testOptions" style="width: 120px" /></a-form-item>
        <a-form-item label="热度"><a-select v-model:value="filters.heat" :options="heatOptions" style="width: 120px" /></a-form-item>
        <a-form-item label="异常状态"><a-select v-model:value="filters.abnormal" :options="abnormalOptions" style="width: 140px" /></a-form-item>
        <a-form-item label="时间范围"><a-range-picker v-model:value="filters.range" /></a-form-item>
      </a-form>
    </a-card>

    <a-card :bordered="false">
      <a-table :columns="columns" :data-source="filteredRows" row-key="name">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'status'">
            <a-tag :color="record.status === '已启用' ? 'green' : 'default'">{{ record.status }}</a-tag>
          </template>
          <template v-else-if="column.key === 'testResult'">
            <a-tag :color="record.testResult === '通过' ? 'green' : 'red'">{{ record.testResult }}</a-tag>
          </template>
          <template v-else-if="column.key === 'risk'">
            <a-badge :status="record.risk === '正常' ? 'success' : 'warning'" :text="record.risk" />
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button type="link" @click="openDetail(record)">测试</a-button>
              <a-button type="link" @click="openEditor(record)">编辑</a-button>
              <a-dropdown>
                <a class="ant-dropdown-link" @click.prevent>更多操作</a>
                <template #overlay>
                  <a-menu>
                    <a-menu-item>复制映射</a-menu-item>
                    <a-menu-item>停用映射</a-menu-item>
                    <a-menu-item danger>删除映射</a-menu-item>
                  </a-menu>
                </template>
              </a-dropdown>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="detailOpen" title="映射详情" :width="620">
      <a-descriptions :column="1" bordered size="small">
        <a-descriptions-item label="映射名称">{{ currentRow?.name ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="mapping_id">{{ currentRow?.mappingId ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="调试信息">最近一次测试耗时 223ms，未触发重试。</a-descriptions-item>
        <a-descriptions-item label="能力约束">大请求需 RTP 承载；流式响应仅在增强模式开放。</a-descriptions-item>
        <a-descriptions-item label="最近诊断记录">10:38 对端目标超时，10:41 自动恢复。</a-descriptions-item>
        <a-descriptions-item label="高级字段">请求体上限 8MB；响应体上限 16MB；超时 5s。</a-descriptions-item>
      </a-descriptions>
    </a-drawer>

    <a-drawer v-model:open="editorOpen" :title="editorTitle" :width="720">
      <a-form layout="vertical">
        <a-card title="基础信息" size="small">
          <a-row :gutter="16">
            <a-col :span="12"><a-form-item label="映射名称" extra="用于运维识别，避免使用内部编号。"><a-input v-model:value="form.name" /></a-form-item></a-col>
            <a-col :span="12"><a-form-item label="启用状态" extra="关闭后不再接收新请求。"><a-switch v-model:checked="form.enabled" /></a-form-item></a-col>
          </a-row>
        </a-card>
        <a-card title="本端入口" size="small" style="margin-top: 12px">
          <a-row :gutter="16">
            <a-col :span="12"><a-form-item label="入口地址" extra="用户访问本端的入口地址。"><a-input v-model:value="form.local" /></a-form-item></a-col>
            <a-col :span="12"><a-form-item label="入口路径" extra="建议按业务域分组路径。"><a-input v-model:value="form.localPath" /></a-form-item></a-col>
          </a-row>
        </a-card>
        <a-card title="对端目标" size="small" style="margin-top: 12px">
          <a-row :gutter="16">
            <a-col :span="12"><a-form-item label="目标地址" extra="对端实际业务地址。"><a-input v-model:value="form.remote" /></a-form-item></a-col>
            <a-col :span="12"><a-form-item label="目标路径" extra="与对端服务约定一致。"><a-input v-model:value="form.remotePath" /></a-form-item></a-col>
          </a-row>
        </a-card>
        <a-card title="请求响应限制" size="small" style="margin-top: 12px">
          <a-row :gutter="16">
            <a-col :span="12"><a-form-item label="请求体上限（MB）" extra="超限将被拒绝并记录告警。"><a-input-number v-model:value="form.reqLimit" style="width: 100%" :min="1" /></a-form-item></a-col>
            <a-col :span="12"><a-form-item label="响应体上限（MB）" extra="建议与网络能力保持一致。"><a-input-number v-model:value="form.resLimit" style="width: 100%" :min="1" /></a-form-item></a-col>
          </a-row>
        </a-card>
        <a-collapse style="margin-top: 12px">
          <a-collapse-panel key="adv" header="高级设置">
            <a-row :gutter="16">
              <a-col :span="12"><a-form-item label="请求超时（毫秒）" extra="默认 5000，可按映射微调。"><a-input-number v-model:value="form.reqTimeout" style="width: 100%" :min="100" /></a-form-item></a-col>
              <a-col :span="12"><a-form-item label="响应超时（毫秒）" extra="建议大于请求超时。"><a-input-number v-model:value="form.resTimeout" style="width: 100%" :min="100" /></a-form-item></a-col>
            </a-row>
          </a-collapse-panel>
        </a-collapse>
      </a-form>
      <template #footer>
        <a-space style="width: 100%; justify-content: flex-end">
          <a-button @click="editorOpen = false">取消</a-button>
          <a-button type="primary">保存并应用</a-button>
        </a-space>
      </template>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'

type Row = {
  name: string
  mappingId: string
  local: string
  remote: string
  status: '已启用' | '已停用'
  testResult: '通过' | '失败'
  requests: number
  failures: number
  latency: string
  risk: '正常' | '关注'
}

const columns = [
  { title: '映射名称', dataIndex: 'name', key: 'name' },
  { title: '本端入口', dataIndex: 'local', key: 'local' },
  { title: '对端目标', dataIndex: 'remote', key: 'remote' },
  { title: '状态', dataIndex: 'status', key: 'status' },
  { title: '最近测试结果', dataIndex: 'testResult', key: 'testResult' },
  { title: '最近请求量', dataIndex: 'requests', key: 'requests' },
  { title: '最近失败数', dataIndex: 'failures', key: 'failures' },
  { title: '平均耗时', dataIndex: 'latency', key: 'latency' },
  { title: '风险提示', dataIndex: 'risk', key: 'risk' },
  { title: '操作', key: 'action' }
]

const rows = ref<Row[]>([
  { name: '订单同步', mappingId: 'map-001', local: '/api/order/sync', remote: 'http://10.9.2.20/sync', status: '已启用', testResult: '通过', requests: 1220, failures: 8, latency: '132ms', risk: '正常' },
  { name: '支付回调', mappingId: 'map-009', local: '/api/payment/callback', remote: 'http://10.9.2.31/callback', status: '已启用', testResult: '失败', requests: 930, failures: 19, latency: '281ms', risk: '关注' },
  { name: '账单归档', mappingId: 'map-016', local: '/api/bill/archive', remote: 'http://10.9.2.45/archive', status: '已停用', testResult: '失败', requests: 210, failures: 11, latency: '325ms', risk: '关注' }
])

const filters = reactive({ keyword: '', status: 'all', test: 'all', heat: 'all', abnormal: 'all', range: [] as string[] })
const statusOptions = [{ label: '全部', value: 'all' }, { label: '已启用', value: '已启用' }, { label: '已停用', value: '已停用' }]
const testOptions = [{ label: '全部', value: 'all' }, { label: '通过', value: '通过' }, { label: '失败', value: '失败' }]
const heatOptions = [{ label: '全部', value: 'all' }, { label: '高热度', value: 'hot' }, { label: '低热度', value: 'cold' }]
const abnormalOptions = [{ label: '全部', value: 'all' }, { label: '仅异常', value: 'abnormal' }]

const filteredRows = computed(() =>
  rows.value.filter((row) => {
    const byKeyword = !filters.keyword || [row.name, row.local, row.remote].join(' ').includes(filters.keyword)
    const byStatus = filters.status === 'all' || row.status === filters.status
    const byTest = filters.test === 'all' || row.testResult === filters.test
    const byAbnormal = filters.abnormal !== 'abnormal' || row.risk === '关注'
    return byKeyword && byStatus && byTest && byAbnormal
  })
)

const detailOpen = ref(false)
const editorOpen = ref(false)
const currentRow = ref<Row>()
const editorTitle = computed(() => (currentRow.value ? `编辑映射：${currentRow.value.name}` : '新建映射'))
const form = reactive({ name: '', enabled: true, local: '', localPath: '', remote: '', remotePath: '', reqLimit: 8, resLimit: 16, reqTimeout: 5000, resTimeout: 8000 })

const openDetail = (row: Row) => {
  currentRow.value = row
  detailOpen.value = true
}

const openEditor = (row?: Row) => {
  currentRow.value = row
  form.name = row?.name ?? ''
  form.enabled = row?.status !== '已停用'
  form.local = row?.local ?? ''
  form.localPath = '/'
  form.remote = row?.remote ?? ''
  form.remotePath = '/'
  editorOpen.value = true
}
</script>
