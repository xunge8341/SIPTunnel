<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xl="16">
        <a-card title="运行总览" :bordered="false">
          <a-row :gutter="[12, 12]">
            <a-col :xs="24" :sm="12" :lg="8" v-for="item in keyStatusCards" :key="item.title">
              <a-card size="small" class="status-card" :bordered="false">
                <a-statistic :title="item.title" :value="item.value" :suffix="item.suffix" />
              </a-card>
            </a-col>
          </a-row>
        </a-card>
      </a-col>
      <a-col :xs="24" :xl="8">
        <a-card title="成功与传输质量" :bordered="false">
          <a-row :gutter="[12, 12]">
            <a-col :span="12">
              <a-statistic title="成功率" :value="dashboard.metrics.successRate" suffix="%" :precision="2" />
            </a-col>
            <a-col :span="12">
              <a-statistic title="失败率" :value="dashboard.metrics.failureRate" suffix="%" :precision="2" />
            </a-col>
            <a-col :span="12">
              <a-statistic title="当前并发" :value="dashboard.metrics.concurrency" />
            </a-col>
            <a-col :span="12">
              <a-statistic title="RTP 丢片率" :value="dashboard.metrics.rtpLossRate" suffix="%" :precision="2" />
            </a-col>
          </a-row>
        </a-card>
      </a-col>
    </a-row>

    <a-card title="系统状态（HTTP 映射隧道）" :bordered="false">
      <a-row :gutter="[12, 12]">
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="隧道链路状态" :value="tunnelStatusText(systemStatus.tunnel_status)" />
        </a-col>
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="注册状态" :value="registrationStatusText(systemStatus.registration_status)" />
        </a-col>
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="心跳状态" :value="heartbeatStatusText(systemStatus.heartbeat_status)" />
        </a-col>
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="网络模式（全局）" :value="networkModeProfile?.shortLabel ?? systemStatus.network_mode" />
        </a-col>
      </a-row>
      <a-row :gutter="[12, 12]" style="margin-top: 8px">
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="最近注册时间" :value="formatDateTime(systemStatus.last_register_time)" />
        </a-col>
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="最近心跳时间" :value="formatDateTime(systemStatus.last_heartbeat_time)" />
        </a-col>
        <a-col :xs="24" :sm="12" :lg="6">
          <a-statistic title="隧道映射总数 / 异常数" :value="mappingCountText" />
        </a-col>
      </a-row>
      <a-alert
        type="error"
        show-icon
        style="margin-top: 12px"
        message="最近异常原因"
        :description="latestAbnormalReason"
      />
      <a-alert
        v-if="systemStatus.peer_binding_error"
        type="error"
        show-icon
        :message="systemStatus.peer_binding_error"
        style="margin-top: 12px"
      />
      <a-descriptions bordered size="small" :column="1" style="margin-top: 12px">
        <a-descriptions-item label="本端 / 对端角色">{{ networkModeProfile?.flowLabel ?? '-' }}</a-descriptions-item>
        <a-descriptions-item label="对端节点">{{ systemStatus.bound_peer?.peer_name || '未绑定' }}</a-descriptions-item>
      </a-descriptions>
      <a-space style="margin-top: 12px">
        <a-typography-title :level="5" style="margin: 0">能力矩阵</a-typography-title>
        <a-tooltip>
          <template #title>能力矩阵由网络模式全局决定，用于约束本端入口与对端目标的可用 HTTP 映射能力。</template>
          <a-typography-text type="secondary">ⓘ</a-typography-text>
        </a-tooltip>
      </a-space>
      <a-descriptions :column="1" size="small" bordered>
        <a-descriptions-item label="支持小请求">{{ yesNo(systemStatus.capability.supports_small_request_body) }}</a-descriptions-item>
        <a-descriptions-item label="支持大响应">{{ yesNo(systemStatus.capability.supports_large_response_body) }}</a-descriptions-item>
        <a-descriptions-item label="支持流式响应">{{ yesNo(systemStatus.capability.supports_streaming_response) }}</a-descriptions-item>
        <a-descriptions-item label="支持大文件上传">{{ yesNo(systemStatus.capability.supports_large_file_upload) }}</a-descriptions-item>
        <a-descriptions-item label="支持HTTP双向">{{ yesNo(systemStatus.capability.supports_bidirectional_http_tunnel) }}</a-descriptions-item>
      </a-descriptions>
    </a-card>


    <a-card title="运维决策视图" :bordered="false">
      <a-row :gutter="12">
        <a-col :xs="24" :lg="8">
          <a-statistic title="当前限流状态" :value="opsSummary.rate_limit_status" />
          <a-statistic title="当前熔断状态" :value="opsSummary.circuit_breaker_state" />
          <a-statistic title="资源保护状态" :value="opsSummary.protection_status" />
        </a-col>
        <a-col :xs="24" :lg="16">
          <a-row :gutter="12">
            <a-col :span="12"><a-card size="small" title="热点映射 TopN"><a-list size="small" :data-source="opsSummary.top_mappings"><template #renderItem="{item}"><a-list-item>{{ item.name }}（{{ item.count }}）</a-list-item></template></a-list></a-card></a-col>
            <a-col :span="12"><a-card size="small" title="来源 IP TopN"><a-list size="small" :data-source="opsSummary.top_source_ips"><template #renderItem="{item}"><a-list-item>{{ item.name }}（{{ item.count }}）</a-list-item></template></a-list></a-card></a-col>
          </a-row>
          <a-row :gutter="12" style="margin-top: 12px">
            <a-col :span="12"><a-card size="small" title="失败最多映射 TopN"><a-list size="small" :data-source="opsSummary.top_failed_mappings"><template #renderItem="{item}"><a-list-item>{{ item.name }}（{{ item.count }}）</a-list-item></template></a-list></a-card></a-col>
            <a-col :span="12"><a-card size="small" title="失败最多来源 IP TopN"><a-list size="small" :data-source="opsSummary.top_failed_source_ips"><template #renderItem="{item}"><a-list-item>{{ item.name }}（{{ item.count }}）</a-list-item></template></a-list></a-card></a-col>
          </a-row>
        </a-col>
      </a-row>
    </a-card>

    <a-card :bordered="false">
      <template #title>
        <a-space>
          <span>界面与服务部署模式</span>
          <a-tooltip>
            <template #title>
              <div>嵌入式模式：界面与服务同进程部署，适用于内网一体化发布与低运维复杂度场景。</div>
              <div>独立模式：界面独立部署并反向代理服务，适用于前后端独立扩缩容与跨域接入场景。</div>
            </template>
            <a-tag color="blue">说明</a-tag>
          </a-tooltip>
        </a-space>
      </template>
      <a-descriptions :column="1" size="small" bordered>
        <a-descriptions-item label="界面部署方式">
          <a-tag :color="deploymentMode.uiMode === 'embedded' ? 'success' : 'processing'">
            {{ deploymentMode.uiMode === 'embedded' ? '嵌入式' : '独立式' }}
          </a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="界面地址">{{ deploymentMode.uiUrl }}</a-descriptions-item>
        <a-descriptions-item label="服务地址">{{ deploymentMode.apiUrl }}</a-descriptions-item>
        <a-descriptions-item label="配置文件路径">{{ deploymentMode.configPath }}</a-descriptions-item>
        <a-descriptions-item label="配置来源">{{ deploymentMode.configSource }}</a-descriptions-item>
      </a-descriptions>
    </a-card>



    <a-card :bordered="false">
      <template #title>
        <a-space>
          <span>全局传输策略（只读）</span>
          <a-tooltip>
            <template #title>transport 来源于网络模式全局推导；不是逐条隧道映射可编辑项。旧 route/api_code 文案仅作兼容提示。</template>
            <a-typography-text type="secondary">ⓘ</a-typography-text>
          </a-tooltip>
        </a-space>
      </template>
      <a-descriptions :column="1" size="small" bordered>
        <a-descriptions-item label="请求元数据 transport（全局推导）">{{ startupSummary.transport_plan.request_meta_transport }}</a-descriptions-item>
        <a-descriptions-item label="请求体 transport（全局推导）">{{ startupSummary.transport_plan.request_body_transport }}</a-descriptions-item>
        <a-descriptions-item label="响应元数据 transport（全局推导）">{{ startupSummary.transport_plan.response_meta_transport }}</a-descriptions-item>
        <a-descriptions-item label="响应体 transport（全局推导）">{{ startupSummary.transport_plan.response_body_transport }}</a-descriptions-item>
        <a-descriptions-item label="request_body_size_limit">{{ startupSummary.transport_plan.request_body_size_limit }}</a-descriptions-item>
        <a-descriptions-item label="response_body_size_limit">{{ startupSummary.transport_plan.response_body_size_limit }}</a-descriptions-item>
      </a-descriptions>
      <a-typography-title :level="5" style="margin-top: 12px">说明</a-typography-title>
      <a-list size="small" bordered :data-source="startupSummary.transport_plan.notes">
        <template #renderItem="{ item }">
          <a-list-item>{{ item }}</a-list-item>
        </template>
      </a-list>
      <a-typography-title :level="5" style="margin-top: 12px">告警</a-typography-title>
      <a-list size="small" bordered :data-source="startupSummary.transport_plan.warnings">
        <template #renderItem="{ item }">
          <a-list-item>{{ item }}</a-list-item>
        </template>
      </a-list>
    </a-card>

    <a-card
      v-if="startupSummary.business_execution.state === 'protocol_only'"
      :bordered="false"
    >
      <a-alert
        type="warning"
        show-icon
        message="当前未加载 HTTP 隧道映射"
        description="系统当前为“协议层可启动、业务执行层未激活”状态，因此不会执行 A 网 HTTP 落地。请加载最小隧道映射配置（旧 httpinvoke route 为兼容格式）后重启并复核。"
      />
      <a-typography-paragraph style="margin-top: 8px; margin-bottom: 0">
        当前未加载 HTTP 隧道映射，因此不会执行 A 网 HTTP 落地。
      </a-typography-paragraph>
    </a-card>

    <a-card title="最近任务趋势图">
      <div class="chart-wrap">
        <svg viewBox="0 0 760 220" role="img" aria-label="任务趋势图">
          <polyline :points="totalLine" class="line-total" />
          <polyline :points="successLine" class="line-success" />
          <polyline :points="failedLine" class="line-failed" />
          <g v-for="point in chartPoints" :key="point.time">
            <circle :cx="point.x" :cy="point.totalY" r="3" class="dot-total" />
            <text :x="point.x" y="210" text-anchor="middle" class="axis-label">{{ point.time }}</text>
          </g>
        </svg>
      </div>
      <a-space>
        <a-tag color="processing">总任务</a-tag>
        <a-tag color="success">成功</a-tag>
        <a-tag color="error">失败</a-tag>
      </a-space>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { DashboardPayload, DashboardOpsSummaryPayload, DeploymentModePayload, StartupSummaryPayload, SystemStatusPayload } from '../types/gateway'
import { getNetworkModeProfile } from '../utils/networkMode'

const deploymentMode = ref<DeploymentModePayload>({
  uiMode: 'embedded',
  uiUrl: '-',
  apiUrl: '-',
  configPath: '-',
  configSource: '-'
})


const startupSummary = ref<StartupSummaryPayload>({
  node_id: '-',
  network_mode: '-',
  bound_peer: undefined,
  peer_binding_error: '',
  capability: {
    supports_large_request_body: false,
    supports_large_response_body: false,
    supports_streaming_response: false,
    supports_bidirectional_http_tunnel: false,
    supports_transparent_proxy: false
  },
  capability_summary: {
    supported: [],
    unsupported: [],
    items: []
  },
  config_path: '-',
  config_source: '-',
  ui_mode: 'embedded',
  ui_url: '-',
  api_url: '-',
  transport_plan: {
    request_meta_transport: '-',
    request_body_transport: '-',
    response_meta_transport: '-',
    response_body_transport: '-',
    request_body_size_limit: 0,
    response_body_size_limit: 0,
    notes: [],
    warnings: []
  },
  business_execution: {
    state: 'active',
    route_count: 1,
    message: '-',
    impact: '-'
  },
  self_check_summary: {
    generated_at: '-',
    overall: 'info',
    info: 0,
    warn: 0,
    error: 0
  }
})


const systemStatus = ref<SystemStatusPayload>({
  tunnel_status: 'disconnected',
  connection_reason: '-',
  network_mode: '-',
  registration_status: 'unregistered',
  heartbeat_status: 'unknown',
  last_register_time: '',
  last_heartbeat_time: '',
  mapping_total: 0,
  mapping_abnormal_total: 0,
  latest_mapping_error_reason: '',
  bound_peer: undefined,
  peer_binding_error: '',
  capability: {
    supports_small_request_body: false,
    supports_large_response_body: false,
    supports_streaming_response: false,
    supports_large_file_upload: false,
    supports_bidirectional_http_tunnel: false
  }
})


const networkModeProfile = computed(() => getNetworkModeProfile(systemStatus.value.network_mode))
const yesNo = (v: boolean) => (v ? '是' : '否')
const formatDateTime = (value?: string) => {
  if (!value) return '-'
  return value.replace('T', ' ').replace('Z', '')
}
const tunnelStatusText = (status: string) => {
  if (status === 'connected') return '已连接'
  if (status === 'disconnected') return '未连接'
  return '异常'
}
const registrationStatusText = (status?: string) => {
  if (status === 'registered') return '正常'
  return '异常'
}
const heartbeatStatusText = (status?: string) => {
  if (status === 'healthy') return '正常'
  return '异常'
}

const mappingCountText = computed(() => {
  const total = systemStatus.value.mapping_total ?? 0
  const abnormal = systemStatus.value.mapping_abnormal_total ?? 0
  return `${total} / ${abnormal}`
})

const latestAbnormalReason = computed(() => {
  if (systemStatus.value.latest_mapping_error_reason) return systemStatus.value.latest_mapping_error_reason
  if (systemStatus.value.peer_binding_error) return systemStatus.value.peer_binding_error
  if (systemStatus.value.connection_reason) return systemStatus.value.connection_reason
  return '当前未发现异常原因'
})

const loadSystemStatus = async () => {
  systemStatus.value = await gatewayApi.fetchSystemStatus()
}

let statusPollingTimer: ReturnType<typeof setInterval> | undefined

const opsSummary = ref<DashboardOpsSummaryPayload>({
  top_mappings: [],
  top_source_ips: [],
  top_failed_mappings: [],
  top_failed_source_ips: [],
  rate_limit_status: '-',
  circuit_breaker_state: '-',
  protection_status: '-'
})

const dashboard = ref<DashboardPayload>({
  metrics: {
    successRate: 0,
    failureRate: 0,
    concurrency: 0,
    rtpLossRate: 0,
    rateLimitHits: 0,
    sipProtocol: 'UDP',
    sipListenPort: 5060,
    rtpProtocol: 'UDP',
    rtpPortRange: '-',
    activeSessions: 0,
    activeTransfers: 0,
    currentConnections: 0,
    failedTasks1h: 0,
    transportErrors1h: 0,
    rateLimitHits1h: 0
  },
  recentTrends: []
})

onMounted(async () => {
  const [dashboardPayload, deploymentPayload, startupPayload, opsSummaryPayload] = await Promise.all([
    gatewayApi.fetchDashboard(),
    gatewayApi.fetchDeploymentMode(),
    gatewayApi.fetchStartupSummary(),
    gatewayApi.fetchDashboardOpsSummary()
  ])
  dashboard.value = dashboardPayload
  deploymentMode.value = deploymentPayload
  startupSummary.value = startupPayload
  opsSummary.value = opsSummaryPayload
  await loadSystemStatus()
  statusPollingTimer = setInterval(() => {
    void loadSystemStatus()
  }, 5000)
})

onUnmounted(() => {
  if (statusPollingTimer) {
    clearInterval(statusPollingTimer)
  }
})

const keyStatusCards = computed<Array<{ title: string; value: string | number; suffix?: string }>>(() => [
  { title: '当前 SIP 协议', value: dashboard.value.metrics.sipProtocol },
  { title: '当前 RTP 协议', value: dashboard.value.metrics.rtpProtocol },
  { title: '当前连接数', value: dashboard.value.metrics.currentConnections },
  { title: '当前活跃传输数', value: dashboard.value.metrics.activeTransfers },
  { title: '最近 1h 失败任务', value: dashboard.value.metrics.failedTasks1h },
  { title: '最近 1 小时传输异常', value: dashboard.value.metrics.transportErrors1h },
  { title: '最近 1h 限流命中', value: dashboard.value.metrics.rateLimitHits1h },
  { title: 'RTP 端口范围', value: dashboard.value.metrics.rtpPortRange }
])

const chartPoints = computed(() => {
  const trend = dashboard.value.recentTrends
  if (!trend.length) return []
  const max = Math.max(...trend.map((t) => t.total))
  return trend.map((t, index) => {
    const x = 40 + index * (680 / Math.max(trend.length - 1, 1))
    const totalY = 180 - (t.total / max) * 140
    const successY = 180 - (t.success / max) * 140
    const failedY = 180 - (t.failed / max) * 140
    return { ...t, x, totalY, successY, failedY }
  })
})

const totalLine = computed(() => chartPoints.value.map((p) => `${p.x},${p.totalY}`).join(' '))
const successLine = computed(() => chartPoints.value.map((p) => `${p.x},${p.successY}`).join(' '))
const failedLine = computed(() => chartPoints.value.map((p) => `${p.x},${p.failedY}`).join(' '))
</script>

<style scoped>
.chart-wrap {
  width: 100%;
  overflow-x: auto;
  margin-bottom: 12px;
}

.status-card {
  background: #fafafa;
}

svg {
  width: 100%;
  min-width: 760px;
  height: 240px;
  background: #fafafa;
  border: 1px solid #f0f0f0;
  border-radius: 6px;
}

.line-total,
.line-success,
.line-failed {
  fill: none;
  stroke-width: 2;
}
.line-total {
  stroke: #1677ff;
}
.line-success {
  stroke: #52c41a;
}
.line-failed {
  stroke: #ff4d4f;
}
.dot-total {
  fill: #1677ff;
}
.axis-label {
  fill: #8c8c8c;
  font-size: 12px;
}
</style>
