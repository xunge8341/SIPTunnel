<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card>
      <div class="toolbar">
        <a-typography-text>节点健康监控（每 30s 刷新）</a-typography-text>
        <a-button @click="refresh">手动刷新</a-button>
      </div>
    </a-card>

    <a-row :gutter="16">
      <a-col v-for="node in nodes" :key="node.id" :xs="24" :md="12">
        <a-card :title="node.id">
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

    <a-card title="运维工具：一键诊断导出">
      <a-space direction="vertical" size="middle" style="width: 100%">
        <a-alert
          type="info"
          show-icon
          message="导出内容含配置快照、节点状态、失败任务、日志索引、告警摘要，适合排障时直接打包传递。"
        />

        <div class="diagnostic-toolbar">
          <a-space wrap>
            <a-typography-text>目标节点</a-typography-text>
            <a-select v-model:value="selectedNodeId" style="min-width: 180px">
              <a-select-option v-for="node in nodes" :key="node.id" :value="node.id">{{ node.id }}</a-select-option>
            </a-select>
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
  </a-space>
</template>

<script setup lang="ts">
import { message } from 'ant-design-vue'
import { onBeforeUnmount, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import StatusPill from '../components/StatusPill.vue'
import type { DiagnosticExportJob } from '../types/gateway'

interface NodeStatus {
  id: string
  status: 'online' | 'offline' | 'degraded'
  cpu: number
  memory: number
  backlog: number
  concurrency: number
}

const nodes = ref<NodeStatus[]>([
  { id: 'gateway-a-01', status: 'online', cpu: 38, memory: 52, backlog: 21, concurrency: 140 },
  { id: 'gateway-a-02', status: 'degraded', cpu: 84, memory: 76, backlog: 189, concurrency: 96 },
  { id: 'gateway-b-01', status: 'online', cpu: 41, memory: 48, backlog: 15, concurrency: 112 },
  { id: 'gateway-b-02', status: 'offline', cpu: 0, memory: 0, backlog: 0, concurrency: 0 }
])

const selectedNodeId = ref(nodes.value[0]?.id ?? '')
const creating = ref(false)
const job = ref<DiagnosticExportJob | null>(null)
const polling = ref(false)
const pollTimer = ref<number | null>(null)

const fileNameRule = 'diag_{nodeId}_{YYYYMMDDTHHmmss}_{jobId}.zip'

const statusTextMap: Record<DiagnosticExportJob['status'], string> = {
  pending: '排队中',
  collecting: '正在采集信息',
  packaging: '正在打包',
  succeeded: '导出成功',
  failed: '导出失败'
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
    job.value = await gatewayApi.createDiagnosticExport({ nodeId: selectedNodeId.value })
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
</script>

<style scoped>
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.diagnostic-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
</style>
