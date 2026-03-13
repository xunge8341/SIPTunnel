<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card>
      <div class="toolbar">
        <a-typography-text>节点健康监控（每 30s 刷新）</a-typography-text>
        <a-space>
          <a-select v-model:value="selectedNodeId" style="min-width: 220px">
            <a-select-option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.id }}</a-select-option>
          </a-select>
          <a-button @click="refresh">手动刷新</a-button>
        </a-space>
      </div>
    </a-card>

    <a-row :gutter="[16, 16]">
      <a-col v-for="node in nodes" :key="node.id" :xs="24" :xl="12">
        <a-card :title="node.id" :class="{ 'node-selected': node.id === selectedNodeId }">
          <a-descriptions :column="2" size="small">
            <a-descriptions-item label="在线状态"><status-pill :value="node.status" kind="online" /></a-descriptions-item>
            <a-descriptions-item label="当前并发">{{ node.concurrency }}</a-descriptions-item>
          </a-descriptions>
          <a-typography-text type="secondary">CPU / 内存</a-typography-text>
          <a-progress :percent="node.cpu" size="small" :status="node.cpu > 80 ? 'exception' : 'active'" />
          <a-progress :percent="node.memory" size="small" :status="node.memory > 80 ? 'exception' : 'active'" />
          <a-typography-text type="secondary">队列积压</a-typography-text>
          <a-statistic :value="node.backlog" suffix="条" :value-style="{ fontSize: '20px' }" />
        </a-card>
      </a-col>
    </a-row>

    <a-card title="节点详情（端口与自检）" v-if="selectedNode">
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :lg="12">
          <a-card size="small" title="端口绑定状态">
            <a-table :data-source="selectedNode.portBindings" :pagination="false" size="small" row-key="service">
              <a-table-column title="服务" data-index="service" key="service" />
              <a-table-column title="协议" data-index="protocol" key="protocol" />
              <a-table-column title="绑定地址" data-index="bindAddress" key="bindAddress" />
              <a-table-column title="状态" key="status">
                <template #default="{ record }">
                  <a-tag :color="portStatusColor(record.status)">{{ portStatusLabel(record.status) }}</a-tag>
                </template>
              </a-table-column>
            </a-table>
          </a-card>
        </a-col>

        <a-col :xs="24" :lg="12">
          <a-card size="small" title="最近端口绑定失败事件">
            <a-list :data-source="selectedNode.bindingFailures" size="small" bordered>
              <template #renderItem="{ item }">
                <a-list-item>
                  <a-space direction="vertical" size="small" style="width: 100%">
                    <a-space>
                      <a-tag color="error">{{ item.service }}</a-tag>
                      <a-typography-text>{{ item.occurredAt }}</a-typography-text>
                    </a-space>
                    <a-typography-text type="secondary">{{ item.reason }}</a-typography-text>
                  </a-space>
                </a-list-item>
              </template>
            </a-list>
          </a-card>
        </a-col>

        <a-col :xs="24" :lg="12">
          <a-card size="small" title="自检结果摘要">
            <a-space direction="vertical" style="width: 100%" size="middle">
              <a-tag :color="selfCheckColor(selectedNode.selfCheck.status)">{{ selfCheckLabel(selectedNode.selfCheck.status) }}</a-tag>
              <a-descriptions :column="3" size="small" bordered>
                <a-descriptions-item label="通过">{{ selectedNode.selfCheck.passed }}</a-descriptions-item>
                <a-descriptions-item label="告警">{{ selectedNode.selfCheck.warning }}</a-descriptions-item>
                <a-descriptions-item label="失败">{{ selectedNode.selfCheck.failed }}</a-descriptions-item>
              </a-descriptions>
              <a-alert
                v-if="selectedNode.selfCheck.summary.includes('业务执行层未激活')"
                type="warning"
                show-icon
                message="业务执行层未激活"
                description="当前未加载业务路由，因此不会执行 A 网 HTTP 落地。"
              />
              <a-typography-text type="secondary">{{ selectedNode.selfCheck.summary }}</a-typography-text>
              <a-typography-text type="secondary">检查时间：{{ selectedNode.selfCheck.checkedAt }}</a-typography-text>
            </a-space>
          </a-card>
        </a-col>

        <a-col :xs="24" :lg="12">
          <a-card size="small" title="一键链路测试">
            <a-space direction="vertical" size="middle" style="width: 100%">
              <a-alert type="info" show-icon message="链路测试仅做最小探测，不写入真实业务数据；HTTP 使用 mock/downstream 健康探针。" />
              <a-space>
                <a-button type="primary" :loading="runningLinkTest" @click="runLinkTest">执行链路测试</a-button>
                <a-tag :color="linkTestTagColor">{{ latestLinkTest?.status ?? '未执行' }}</a-tag>
              </a-space>
              <a-empty v-if="!latestLinkTest" description="暂无链路测试记录" />
              <template v-else>
                <a-descriptions bordered size="small" :column="2">
                  <a-descriptions-item label="结论">{{ latestLinkTest.status }}</a-descriptions-item>
                  <a-descriptions-item label="总耗时">{{ latestLinkTest.duration_ms }} ms</a-descriptions-item>
                  <a-descriptions-item label="request_id">{{ latestLinkTest.request_id }}</a-descriptions-item>
                  <a-descriptions-item label="trace_id">{{ latestLinkTest.trace_id }}</a-descriptions-item>
                  <a-descriptions-item label="mock 目标" :span="2">{{ latestLinkTest.mock_target }}</a-descriptions-item>
                </a-descriptions>
                <a-list size="small" bordered :data-source="latestLinkTest.items">
                  <template #renderItem="{ item }">
                    <a-list-item>
                      <a-space direction="vertical" size="small" style="width: 100%">
                        <a-space>
                          <a-tag :color="item.passed ? 'success' : 'error'">{{ item.status }}</a-tag>
                          <span>{{ item.name }}</span>
                          <a-typography-text type="secondary">{{ item.duration_ms }} ms</a-typography-text>
                        </a-space>
                        <a-typography-text type="secondary">{{ item.detail }}</a-typography-text>
                      </a-space>
                    </a-list-item>
                  </template>
                </a-list>
              </template>
            </a-space>
          </a-card>
        </a-col>

        <a-col :xs="24" :lg="12">
          <a-card size="small" title="导出诊断入口">
            <a-alert
              type="info"
              show-icon
              message="导出内容含 transport 配置、连接统计、端口池、transport 错误、task failure、rate limit 与 profile 入口信息，适合排障打包。"
            />
            <a-space direction="vertical" size="middle" style="width: 100%; margin-top: 12px">
              <div class="diagnostic-toolbar">
                <a-space wrap>
                  <a-typography-text>目标节点</a-typography-text>
                  <a-select v-model:value="selectedNodeId" style="min-width: 180px">
                    <a-select-option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.id }}</a-select-option>
                  </a-select>
                  <a-input v-model:value="diagRequestId" allow-clear placeholder="request_id（可选）" style="width: 180px" />
                  <a-input v-model:value="diagTraceId" allow-clear placeholder="trace_id（可选）" style="width: 180px" />
                  <a-button type="primary" :loading="creating" @click="startExport">导出诊断包</a-button>
                </a-space>
                <a-typography-text type="secondary">文件命名：{{ fileNameRule }}</a-typography-text>
              </div>

              <a-empty v-if="!job" description="尚未发起导出任务" />

              <template v-else>
                <a-descriptions bordered size="small" :column="2">
                  <a-descriptions-item label="任务编号">{{ job.jobId }}</a-descriptions-item>
                  <a-descriptions-item label="当前状态">{{ statusTextMap[job.status] }}</a-descriptions-item>
                  <a-descriptions-item label="目标节点">{{ job.nodeId }}</a-descriptions-item>
                  <a-descriptions-item label="输出文件">{{ job.fileName }}</a-descriptions-item>
                </a-descriptions>

                <a-progress :percent="job.progress" :status="job.status === 'failed' ? 'exception' : undefined" />

                <a-list size="small" bordered :data-source="job.sections">
                  <template #renderItem="{ item }">
                    <a-list-item>
                      <a-space>
                        <status-pill :value="item.done ? 'online' : 'degraded'" kind="online" />
                        <span>{{ item.label }}</span>
                      </a-space>
                    </a-list-item>
                  </template>
                </a-list>

                <a-alert v-if="job.errorMessage" type="error" show-icon :message="job.errorMessage" />

                <a-space>
                  <a-button v-if="job.status === 'failed'" @click="retryExport">重试导出</a-button>
                  <a-button type="primary" :disabled="job.status !== 'succeeded'" @click="downloadResult">下载诊断包</a-button>
                  <a-button :disabled="!polling || job.status === 'succeeded' || job.status === 'failed'" @click="refreshJob">
                    刷新状态
                  </a-button>
                </a-space>
              </template>
            </a-space>
          </a-card>
        </a-col>
      </a-row>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { message } from 'ant-design-vue'
import { computed, onBeforeUnmount, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import StatusPill from '../components/StatusPill.vue'
import type { DiagnosticExportJob, NodeOpsSnapshot, OpsLinkTestReport } from '../types/gateway'

const nodes = ref<NodeOpsSnapshot[]>([
  {
    id: 'gateway-a-01',
    status: 'online',
    cpu: 38,
    memory: 52,
    backlog: 21,
    concurrency: 140,
    portBindings: [
      { service: 'SIP', protocol: 'UDP', bindAddress: '0.0.0.0:5060', status: 'bound', updatedAt: '2026-03-12 14:30:11' },
      { service: 'RTP', protocol: 'UDP', bindAddress: '0.0.0.0:20000-20999', status: 'bound', updatedAt: '2026-03-12 14:30:11' }
    ],
    bindingFailures: [{ id: 'evt-001', occurredAt: '2026-03-12 07:14:03', service: 'RTP', reason: '端口 20032 被占用，已自动回收后重试成功。' }],
    selfCheck: { status: 'pass', checkedAt: '2026-03-12 14:29:45', passed: 18, warning: 0, failed: 0, summary: '网络、存储、任务队列与路由模板均通过。' }
  },
  {
    id: 'gateway-a-02',
    status: 'degraded',
    cpu: 84,
    memory: 76,
    backlog: 189,
    concurrency: 96,
    portBindings: [
      { service: 'SIP', protocol: 'UDP', bindAddress: '0.0.0.0:5060', status: 'degraded', updatedAt: '2026-03-12 14:30:02' },
      { service: 'RTP', protocol: 'UDP', bindAddress: '0.0.0.0:20000-20999', status: 'bound', updatedAt: '2026-03-12 14:30:02' }
    ],
    bindingFailures: [
      { id: 'evt-002', occurredAt: '2026-03-12 13:47:12', service: 'SIP', reason: '监听端口瞬时抖动，重绑耗时 3.2s。' },
      { id: 'evt-003', occurredAt: '2026-03-12 11:07:54', service: 'RTP', reason: '端口池耗尽触发限速，回收后恢复。' }
    ],
    selfCheck: { status: 'warn', checkedAt: '2026-03-12 14:29:45', passed: 15, warning: 2, failed: 1, summary: '协议层可启动、业务执行层未激活：当前未加载业务路由，A 网 HTTP 落地不会执行。' }
  },
  {
    id: 'gateway-b-01',
    status: 'online',
    cpu: 41,
    memory: 48,
    backlog: 15,
    concurrency: 112,
    portBindings: [
      { service: 'SIP', protocol: 'TCP', bindAddress: '0.0.0.0:5061', status: 'bound', updatedAt: '2026-03-12 14:30:35' },
      { service: 'RTP', protocol: 'UDP', bindAddress: '0.0.0.0:21000-21999', status: 'bound', updatedAt: '2026-03-12 14:30:35' }
    ],
    bindingFailures: [{ id: 'evt-004', occurredAt: '2026-03-11 23:58:10', service: 'SIP', reason: '配置回滚后重新绑定一次，结果正常。' }],
    selfCheck: { status: 'pass', checkedAt: '2026-03-12 14:29:45', passed: 18, warning: 0, failed: 0, summary: '全部检查项通过，节点运行稳定。' }
  },
  {
    id: 'gateway-b-02',
    status: 'offline',
    cpu: 0,
    memory: 0,
    backlog: 0,
    concurrency: 0,
    portBindings: [
      { service: 'SIP', protocol: 'UDP', bindAddress: '0.0.0.0:5060', status: 'unbound', updatedAt: '2026-03-12 14:20:06' },
      { service: 'RTP', protocol: 'UDP', bindAddress: '0.0.0.0:20000-20999', status: 'unbound', updatedAt: '2026-03-12 14:20:06' }
    ],
    bindingFailures: [{ id: 'evt-005', occurredAt: '2026-03-12 14:20:06', service: 'SIP', reason: '节点离线，无法完成绑定。' }],
    selfCheck: { status: 'fail', checkedAt: '2026-03-12 14:20:06', passed: 3, warning: 1, failed: 8, summary: '节点不可达，请优先恢复主机可用性。' }
  }
])

const selectedNodeId = ref(nodes.value[0]?.id ?? '')
const creating = ref(false)
const diagRequestId = ref('')
const diagTraceId = ref('')
const job = ref<DiagnosticExportJob | null>(null)
const runningLinkTest = ref(false)
const latestLinkTest = ref<OpsLinkTestReport | null>(null)
const polling = ref(false)
const pollTimer = ref<number | null>(null)

const selectedNode = computed(() => nodes.value.find((item) => item.id === selectedNodeId.value) ?? null)
const linkTestTagColor = computed(() => {
  if (!latestLinkTest.value) return 'default'
  return latestLinkTest.value.passed ? 'success' : 'error'
})

const fileNameRule = 'diag_{nodeId}_{YYYYMMDDTHHmmssZ}[_req_{request_id}][_trace_{trace_id}]_{jobId}(.zip)，其中 ID 仅保留字母数字/下划线'

const statusTextMap: Record<DiagnosticExportJob['status'], string> = {
  pending: '排队中',
  collecting: '正在采集信息',
  packaging: '正在打包',
  succeeded: '导出成功',
  failed: '导出失败'
}

const portStatusLabel = (status: 'bound' | 'unbound' | 'degraded') => {
  if (status === 'bound') return '已绑定'
  if (status === 'degraded') return '绑定抖动'
  return '未绑定'
}

const portStatusColor = (status: 'bound' | 'unbound' | 'degraded') => {
  if (status === 'bound') return 'success'
  if (status === 'degraded') return 'warning'
  return 'error'
}

const selfCheckLabel = (status: 'pass' | 'warn' | 'fail') => {
  if (status === 'pass') return '自检通过'
  if (status === 'warn') return '自检告警'
  return '自检失败'
}

const selfCheckColor = (status: 'pass' | 'warn' | 'fail') => {
  if (status === 'pass') return 'success'
  if (status === 'warn') return 'warning'
  return 'error'
}

const refresh = () => {
  nodes.value = nodes.value.map((node) => ({
    ...node,
    cpu: Math.max(0, Math.min(95, node.cpu + (Math.random() > 0.5 ? 3 : -3))),
    memory: Math.max(0, Math.min(95, node.memory + (Math.random() > 0.5 ? 2 : -2))),
    backlog: Math.max(0, node.backlog + (Math.random() > 0.5 ? 8 : -6)),
    concurrency: Math.max(0, node.concurrency + (Math.random() > 0.5 ? 5 : -4))
  }))
}

const runLinkTest = async () => {
  runningLinkTest.value = true
  try {
    latestLinkTest.value = await gatewayApi.runLinkTest()
    message.success(`链路测试完成：${latestLinkTest.value.status}`)
  } catch (error) {
    message.error(error instanceof Error ? error.message : '链路测试失败')
  } finally {
    runningLinkTest.value = false
  }
}

const loadLatestLinkTest = async () => {
  try {
    latestLinkTest.value = await gatewayApi.fetchLatestLinkTest()
  } catch {
    latestLinkTest.value = null
  }
}

const stopPolling = () => {
  if (pollTimer.value) {
    window.clearInterval(pollTimer.value)
    pollTimer.value = null
  }
  polling.value = false
}

const refreshJob = async () => {
  if (!job.value) return
  try {
    const latest = await gatewayApi.fetchDiagnosticExport(job.value.jobId)
    job.value = latest
    if (latest.status === 'succeeded' || latest.status === 'failed') {
      stopPolling()
    }
  } catch (error) {
    stopPolling()
    message.error(error instanceof Error ? error.message : '刷新任务状态失败，请稍后重试')
  }
}

const startPolling = () => {
  stopPolling()
  polling.value = true
  pollTimer.value = window.setInterval(() => {
    void refreshJob()
  }, 2000)
}

const startExport = async () => {
  if (!selectedNodeId.value) {
    message.warning('请先选择一个节点')
    return
  }
  creating.value = true
  try {
    job.value = await gatewayApi.createDiagnosticExport({
      nodeId: selectedNodeId.value,
      requestId: diagRequestId.value.trim() || undefined,
      traceId: diagTraceId.value.trim() || undefined
    })
    startPolling()
    void refreshJob()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '创建导出任务失败')
  } finally {
    creating.value = false
  }
}

const retryExport = async () => {
  if (!job.value) return
  try {
    job.value = await gatewayApi.retryDiagnosticExport(job.value.jobId)
    startPolling()
    message.success('已重新发起导出任务')
  } catch (error) {
    message.error(error instanceof Error ? error.message : '重试失败，请稍后再试')
  }
}

const downloadResult = () => {
  if (!job.value?.downloadUrl) {
    message.warning('当前没有可下载文件，请等待导出完成')
    return
  }
  const link = document.createElement('a')
  link.href = job.value.downloadUrl
  link.download = job.value.fileName
  link.click()
}

onBeforeUnmount(() => {
  stopPolling()
})

void loadLatestLinkTest()
</script>

<style scoped>
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.node-selected {
  border: 1px solid #91caff;
}

.diagnostic-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
</style>
