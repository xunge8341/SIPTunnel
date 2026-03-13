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
import { computed, onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import type { DashboardPayload } from '../types/gateway'

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
  dashboard.value = await gatewayApi.fetchDashboard()
})

const keyStatusCards = computed<Array<{ title: string; value: string | number; suffix?: string }>>(() => [
  { title: '当前 SIP transport', value: dashboard.value.metrics.sipProtocol },
  { title: '当前 RTP transport', value: dashboard.value.metrics.rtpProtocol },
  { title: '当前连接数', value: dashboard.value.metrics.currentConnections },
  { title: '当前活跃传输数', value: dashboard.value.metrics.activeTransfers },
  { title: '最近 1h 失败任务', value: dashboard.value.metrics.failedTasks1h },
  { title: '最近 1h transport error', value: dashboard.value.metrics.transportErrors1h },
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
