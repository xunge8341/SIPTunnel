<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-row :gutter="16">
      <a-col :xs="24" :sm="12" :lg="8" :xl="4" v-for="item in metricCards" :key="item.title">
        <a-card>
          <a-statistic :title="item.title" :value="item.value" :suffix="item.suffix" :precision="item.precision" />
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
    rateLimitHits: 0
  },
  recentTrends: []
})

onMounted(async () => {
  dashboard.value = await gatewayApi.fetchDashboard()
})

const metricCards = computed(() => [
  { title: '成功率', value: dashboard.value.metrics.successRate, suffix: '%', precision: 2 },
  { title: '失败率', value: dashboard.value.metrics.failureRate, suffix: '%', precision: 2 },
  { title: '当前并发', value: dashboard.value.metrics.concurrency, suffix: '', precision: 0 },
  { title: 'RTP 丢片率', value: dashboard.value.metrics.rtpLossRate, suffix: '%', precision: 2 },
  { title: '限流命中次数', value: dashboard.value.metrics.rateLimitHits, suffix: '次', precision: 0 }
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
