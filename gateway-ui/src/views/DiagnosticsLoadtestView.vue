<template>
  <a-space direction="vertical" style="width: 100%" size="large">
    <a-page-header title="诊断与压测" sub-title="接入真实诊断链路、诊断包导出与压测任务列表，并基于压测结果输出保护策略建设值。" />
    <a-alert v-if="notice" type="success" :message="notice" show-icon />
    <a-alert v-if="error" type="error" :message="error" show-icon />

    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xl="12">
        <a-card title="诊断" :bordered="false">
          <a-space direction="vertical" style="width: 100%">
            <a-space wrap>
              <a-button type="primary" :loading="testing" @click="runLinkTest">本端节点连通性测试</a-button>
              <a-button :loading="testing" @click="runPeerLinkTest">对端节点连通性测试</a-button>
              <a-button @click="notice = '请在隧道映射页使用单条测试入口。'">单条映射测试</a-button>
              <a-button @click="notice = '请在隧道映射页使用全部测试入口。'">全部映射测试</a-button>
              <a-button :loading="exporting" @click="runExport">导出诊断包</a-button>
            </a-space>

            <a-spin :spinning="loading">
              <a-empty v-if="!report" description="暂无最近诊断记录" />
              <template v-else>
                <a-descriptions bordered :column="1" size="small">
                  <a-descriptions-item label="最近测试时间">{{ formatDateTimeText(report.checked_at) }}</a-descriptions-item>
                  <a-descriptions-item label="整体结果">{{ report.status === 'passed' ? '通过' : '失败' }}</a-descriptions-item>
                  <a-descriptions-item label="耗时">{{ report.duration_ms }} ms</a-descriptions-item>
                  <a-descriptions-item label="request_id">{{ report.request_id }}</a-descriptions-item>
                  <a-descriptions-item label="trace_id">{{ report.trace_id }}</a-descriptions-item>
                </a-descriptions>
                <a-list header="诊断步骤" bordered size="small" :data-source="report.items" style="margin-top: 16px">
                  <template #renderItem="{ item }">
                    <a-list-item>
                      <a-space style="width: 100%; justify-content: space-between">
                        <span>{{ item.name || '未命名步骤' }}：{{ item.detail }}</span>
                        <a-tag :color="item.status === 'passed' ? 'green' : 'red'">{{ item.status === 'passed' ? '通过' : '失败' }}</a-tag>
                      </a-space>
                    </a-list-item>
                  </template>
                </a-list>
              </template>
            </a-spin>

            <a-card v-if="exportData" size="small" title="最近诊断包导出结果">
              <a-descriptions :column="1" size="small" bordered>
                <a-descriptions-item label="生成时间">{{ formatDateTimeText(exportData.generated_at) }}</a-descriptions-item>
                <a-descriptions-item label="输出目录">{{ exportData.output_dir }}</a-descriptions-item>
                <a-descriptions-item label="文件名">{{ exportData.file_name }}</a-descriptions-item>
                <a-descriptions-item label="关联 request_id">{{ exportData.request_id || '无' }}</a-descriptions-item>
                <a-descriptions-item label="关联 trace_id">{{ exportData.trace_id || '无' }}</a-descriptions-item>
              </a-descriptions>
            </a-card>
          </a-space>
        </a-card>
      </a-col>

      <a-col :xs="24" :xl="12">
        <a-card title="压测" :bordered="false">
          <a-form layout="vertical">
            <a-form-item label="HTTP 目标地址">
              <a-input v-model:value="httpUrl" placeholder="例如 http://127.0.0.1:18080/healthz" />
            </a-form-item>
            <a-form-item label="网关基地址">
              <a-input v-model:value="gatewayBaseUrl" placeholder="例如 http://127.0.0.1:8080" />
            </a-form-item>
            <a-row :gutter="16">
              <a-col :span="8"><a-form-item label="并发数"><a-input-number v-model:value="loadtestConcurrency" :min="1" style="width: 100%" /></a-form-item></a-col>
              <a-col :span="8"><a-form-item label="QPS"><a-input-number v-model:value="loadtestQps" :min="0" style="width: 100%" /></a-form-item></a-col>
              <a-col :span="8"><a-form-item label="持续时间（秒）"><a-input-number v-model:value="loadtestDurationSec" :min="1" style="width: 100%" /></a-form-item></a-col>
            </a-row>
            <a-form-item label="结果输出目录"><a-input v-model:value="outputDir" placeholder="默认写入 ./data/final/loadtest" /></a-form-item>
            <a-button type="primary" :loading="startingLoadtest" @click="runLoadtest">发起压测</a-button>
          </a-form>
          <a-divider />
          <a-card v-if="latestCapacitySuggestion" size="small" title="保护策略建设值" style="margin-bottom: 16px">
            <a-descriptions :column="1" size="small" bordered>
              <a-descriptions-item label="命令并发建议">{{ latestCapacitySuggestion.recommended_command_max_concurrent ?? '-' }}</a-descriptions-item>
              <a-descriptions-item label="文件传输并发建议">{{ latestCapacitySuggestion.recommended_file_transfer_max_concurrent ?? '-' }}</a-descriptions-item>
              <a-descriptions-item label="RTP 端口池建议">{{ latestCapacitySuggestion.recommended_rtp_port_pool_size ?? '-' }}</a-descriptions-item>
              <a-descriptions-item label="最大连接数建议">{{ latestCapacitySuggestion.recommended_max_connections ?? '-' }}</a-descriptions-item>
              <a-descriptions-item label="限流 RPS 建议">{{ latestCapacitySuggestion.recommended_rate_limit_rps ?? '-' }}</a-descriptions-item>
              <a-descriptions-item label="限流 Burst 建议">{{ latestCapacitySuggestion.recommended_rate_limit_burst ?? '-' }}</a-descriptions-item>
              <a-descriptions-item label="建设依据">{{ capacityBasisText }}</a-descriptions-item>
            </a-descriptions>
          </a-card>
          <a-list header="压测任务" bordered :data-source="jobs" :locale="{ emptyText: '暂无压测任务' }">
            <template #renderItem="{ item }">
              <a-list-item>
                <a-space direction="vertical" style="width: 100%">
                  <a-space style="justify-content: space-between; width: 100%">
                    <span>{{ item.job_id }}</span>
                    <a-tag :color="item.status === 'succeeded' ? 'green' : item.status === 'failed' ? 'red' : 'blue'">{{ item.status === 'succeeded' ? '成功' : item.status === 'failed' ? '失败' : item.status === 'running' ? '运行中' : item.status }}</a-tag>
                  </a-space>
                  <span>创建时间 {{ formatDateTimeText(item.created_at) }} / 更新时间 {{ formatDateTimeText(item.updated_at) }}</span>
                  <span>并发 {{ item.concurrency }} / QPS {{ item.qps }} / 持续 {{ item.duration_sec }} 秒</span>
                  <span v-if="item.error_message">错误：{{ item.error_message }}</span>
                </a-space>
              </a-list-item>
            </template>
          </a-list>
        </a-card>
      </a-col>
    </a-row>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import { formatDateTimeText } from '../utils/date'
import type { DiagnosticExportData, LoadtestJob, OpsLinkTestReport } from '../types/gateway'

const loading = ref(false)
const testing = ref(false)
const exporting = ref(false)
const startingLoadtest = ref(false)
const error = ref('')
const notice = ref('')
const report = ref<OpsLinkTestReport>()
const exportData = ref<DiagnosticExportData>()
const jobs = ref<LoadtestJob[]>([])
const httpUrl = ref('http://127.0.0.1:18080/healthz')
const gatewayBaseUrl = ref('http://127.0.0.1:8080')
const loadtestConcurrency = ref<number | null>(50)
const loadtestQps = ref<number | null>(100)
const loadtestDurationSec = ref<number | null>(30)
const outputDir = ref('')
const workspace = ref<any>()

const latestCapacitySuggestion = computed<Record<string, any> | undefined>(() => jobs.value.find((item) => item.status === 'succeeded' && item.capacity_suggestion)?.capacity_suggestion as Record<string, any> | undefined)
const capacityBasisText = computed(() => Array.isArray(latestCapacitySuggestion.value?.basis) ? latestCapacitySuggestion.value?.basis.join('；') : latestCapacitySuggestion.value?.note || '暂无')

const loadLatest = async () => {
  loading.value = true
  error.value = ''
  try {
    const [linkReport, loadtestItems, workspaceResp] = await Promise.all([
      gatewayApi.fetchLatestLinkTest().catch(() => undefined),
      gatewayApi.fetchLoadtestJobs().catch(() => []),
      gatewayApi.fetchNodeTunnelWorkspace().catch(() => undefined)
    ])
    report.value = linkReport
    jobs.value = loadtestItems
    workspace.value = workspaceResp
    if (workspaceResp?.peerNode?.node_ip && workspaceResp?.peerNode?.signaling_port) {
      gatewayBaseUrl.value = gatewayBaseUrl.value || 'http://127.0.0.1:8080'
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载诊断与压测数据失败'
  } finally {
    loading.value = false
  }
}

const runLinkTest = async () => {
  testing.value = true
  error.value = ''
  notice.value = ''
  try {
    report.value = await gatewayApi.runLinkTest()
    notice.value = `本端节点连通性测试已完成：${report.value.status === 'passed' ? '通过' : '失败'}。`
  } catch (e) {
    error.value = e instanceof Error ? e.message : '执行连通性测试失败'
  } finally {
    testing.value = false
  }
}


const runPeerLinkTest = async () => {
  testing.value = true
  error.value = ''
  notice.value = ''
  try {
    report.value = await gatewayApi.runLinkTest({ target: 'peer' } as any)
    notice.value = `对端节点连通性测试已完成：${report.value.status === 'passed' ? '通过' : '失败'}。`
  } catch (e) {
    error.value = e instanceof Error ? e.message : '执行对端节点连通性测试失败'
  } finally {
    testing.value = false
  }
}

const runExport = async () => {
  exporting.value = true
  error.value = ''
  notice.value = ''
  try {
    exportData.value = await gatewayApi.exportDiagnostics(report.value?.request_id, report.value?.trace_id)
    notice.value = '诊断包已导出并生成最近结果摘要。'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '导出诊断包失败'
  } finally {
    exporting.value = false
  }
}

const runLoadtest = async () => {
  startingLoadtest.value = true
  error.value = ''
  notice.value = ''
  try {
    const job = await gatewayApi.startLoadtest({
      http_url: httpUrl.value,
      gateway_base_url: gatewayBaseUrl.value,
      concurrency: loadtestConcurrency.value ?? undefined,
      qps: loadtestQps.value ?? undefined,
      duration_sec: loadtestDurationSec.value ?? undefined,
      output_dir: outputDir.value || undefined
    })
    notice.value = `压测任务已创建：${job.job_id}`
    jobs.value = await gatewayApi.fetchLoadtestJobs()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '创建压测任务失败'
  } finally {
    startingLoadtest.value = false
  }
}

onMounted(loadLatest)
</script>
