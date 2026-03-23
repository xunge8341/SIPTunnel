<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="总览监控" sub-title="统一查看近 1 小时访问趋势、热点、保护状态与待处理事项。">
      <template #extra>
        <a-space>
          <a-tag color="blue">统计口径：近 1 小时访问日志</a-tag>
          <a-button @click="load">刷新</a-button>
          <a-button type="primary" @click="goToAccessLogs">查看失败日志</a-button>
        </a-space>
      </template>
    </a-page-header>

    <a-alert v-if="error" type="error" :message="error" show-icon />

    <a-spin :spinning="loading">
      <a-empty v-if="!summary" description="暂无总览数据" />
      <template v-else>
        <a-row :gutter="[16, 16]" class="dashboard-section">
          <a-col v-for="card in summaryCards" :key="card.title" :xs="24" :sm="12" :xl="8">
            <a-card :bordered="false" class="dashboard-card">
              <a-statistic :title="card.title" :value="card.value" />
              <div class="card-hint">{{ card.hint }}</div>
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="[16, 16]" class="dashboard-section">
          <a-col :span="24">
            <a-card title="访问趋势" :bordered="false" class="dashboard-card trend-card">
              <div class="trend-toolbar">
                <a-radio-group v-model:value="rangeKey" button-style="solid" @change="loadTrends">
                  <a-radio-button value="1h">近 1 小时</a-radio-button>
                  <a-radio-button value="6h">近 6 小时</a-radio-button>
                  <a-radio-button value="24h">近 24 小时</a-radio-button>
                  <a-radio-button value="7d">近 7 天</a-radio-button>
                </a-radio-group>
                <a-select v-model:value="granularity" style="width: 120px" @change="loadTrends">
                  <a-select-option value="5m">5 分钟</a-select-option>
                  <a-select-option value="15m">15 分钟</a-select-option>
                  <a-select-option value="1h">1 小时</a-select-option>
                  <a-select-option value="1d">1 天</a-select-option>
                </a-select>
              </div>
              <div v-if="trendSeries.points.length" class="trend-chart-wrap">
                <div class="trend-chart">
                  <svg :viewBox="`0 0 ${chartWidth} ${chartHeight}`" preserveAspectRatio="none">
                    <line v-for="y in gridLines" :key="`grid-${y}`" :x1="paddingLeft" :x2="chartWidth - paddingRight" :y1="y" :y2="y" class="grid-line" />
                    <polyline :points="totalPolyline" class="line-total" fill="none" />
                    <polyline :points="failedPolyline" class="line-failed" fill="none" />
                  </svg>
                </div>
                <div class="trend-legend">
                  <span><i class="legend-dot total"></i>总请求</span>
                  <span><i class="legend-dot failed"></i>失败请求</span>
                  <span><i class="legend-dot slow"></i>慢请求：{{ totalSlow }}</span>
                </div>
                <div class="trend-axis">
                  <span v-for="point in xTicks" :key="point.bucket">{{ point.label }}</span>
                </div>
              </div>
              <a-empty v-else description="当前时间范围内暂无趋势数据" />
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="[16, 16]" align="stretch" class="dashboard-section">
          <a-col :xs="24" :xl="12">
            <a-card title="热点与异常映射" :bordered="false" class="dashboard-card full-card">
              <a-row :gutter="16">
                <a-col :span="12">
                  <div class="section-title">访问最频繁</div>
                  <a-list :data-source="ops?.hotMappings || []" size="small" :locale="emptyLocale">
                    <template #renderItem="{ item }">
                      <a-list-item>
                        <a-space class="list-line">
                          <span>{{ item.name }}</span>
                          <a-tag color="blue">{{ item.count }}</a-tag>
                        </a-space>
                      </a-list-item>
                    </template>
                  </a-list>
                </a-col>
                <a-col :span="12">
                  <div class="section-title">失败最多</div>
                  <a-list :data-source="ops?.topFailureMappings || []" size="small" :locale="emptyLocale">
                    <template #renderItem="{ item }">
                      <a-list-item>
                        <a-space class="list-line">
                          <span>{{ item.name }}</span>
                          <a-tag color="red">{{ item.count }}</a-tag>
                        </a-space>
                      </a-list-item>
                    </template>
                  </a-list>
                </a-col>
              </a-row>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="12">
            <a-card title="来源 IP 风险" :bordered="false" class="dashboard-card full-card">
              <a-row :gutter="16">
                <a-col :span="12">
                  <div class="section-title">访问最频繁</div>
                  <a-list :data-source="ops?.hotSourceIPs || []" size="small" :locale="emptyLocale">
                    <template #renderItem="{ item }">
                      <a-list-item>
                        <a-space class="list-line">
                          <span>{{ item.name }}</span>
                          <a-tag>{{ item.count }}</a-tag>
                        </a-space>
                      </a-list-item>
                    </template>
                  </a-list>
                </a-col>
                <a-col :span="12">
                  <div class="section-title">失败最多</div>
                  <a-list :data-source="ops?.topFailureIPs || []" size="small" :locale="emptyLocale">
                    <template #renderItem="{ item }">
                      <a-list-item>
                        <a-space class="list-line">
                          <span>{{ item.name }}</span>
                          <a-tag color="red">{{ item.count }}</a-tag>
                        </a-space>
                      </a-list-item>
                    </template>
                  </a-list>
                </a-col>
              </a-row>
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="[16, 16]" align="stretch" class="dashboard-section">
          <a-col :xs="24" :xl="14">
            <a-card title="保护状态" :bordered="false" class="dashboard-card full-card">
              <a-descriptions :column="1" size="small">
                <a-descriptions-item label="限流状态">{{ formatRateState(summary.rateLimitState) }}</a-descriptions-item>
                <a-descriptions-item label="熔断状态">{{ formatCircuitState(summary.circuitBreakerState) }}</a-descriptions-item>
                <a-descriptions-item label="建议操作">{{ actionAdvice }}</a-descriptions-item>
              </a-descriptions>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="10">
            <a-card title="快速操作" :bordered="false" class="dashboard-card full-card">
              <a-space direction="vertical" style="width: 100%">
                <a-button block @click="go('/tunnel-mappings')">进入隧道映射</a-button>
                <a-button block @click="go('/access-logs')">查看访问日志</a-button>
                <a-button block @click="go('/diagnostics-loadtest')">进入诊断与压测</a-button>
                <a-button block @click="go('/system-settings')">进入系统设置</a-button>
              </a-space>
            </a-card>
          </a-col>
        </a-row>
      </template>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { gatewayApi } from '../api/gateway'
import type { DashboardOpsSummary, DashboardSummary, DashboardTrendSeries } from '../types/gateway'

const router = useRouter()
const loading = ref(false)
const error = ref('')
const summary = ref<DashboardSummary>()
const ops = ref<DashboardOpsSummary>()
const trendSeries = ref<DashboardTrendSeries>({ range: '24h', granularity: '1h', points: [] })
const rangeKey = ref<'1h' | '6h' | '24h' | '7d'>('24h')
const granularity = ref<'5m' | '15m' | '1h' | '1d'>('1h')

const emptyLocale = { emptyText: '暂无数据' }
const chartWidth = 900
const chartHeight = 260
const paddingLeft = 32
const paddingRight = 12
const paddingTop = 16
const paddingBottom = 28

const translateHealth = (value: string) => value === 'healthy' ? '健康' : value === 'degraded' ? '降级' : value || '未知'
const formatRateState = (value: string) => value === 'normal' ? '正常' : value === 'enabled' ? '已启用' : value || '正常'
const formatCircuitState = (value: string) => value === 'closed' ? '关闭' : value === 'open' ? '保护中' : value || '关闭'

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    const [summaryResp, opsResp, trendResp] = await Promise.all([
      gatewayApi.fetchDashboardSummary(),
      gatewayApi.fetchDashboardOpsSummary(),
      gatewayApi.fetchDashboardTrends(rangeKey.value, granularity.value)
    ])
    summary.value = summaryResp
    ops.value = opsResp
    trendSeries.value = trendResp
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载总览数据失败'
  } finally {
    loading.value = false
  }
}

const loadTrends = async () => {
  try {
    trendSeries.value = await gatewayApi.fetchDashboardTrends(rangeKey.value, granularity.value)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载趋势失败'
  }
}

const summaryCards = computed(() => {
  if (!summary.value) return []
  return [
    { title: '系统健康', value: translateHealth(summary.value.systemHealth), hint: '根据近 1 小时访问日志与隧道状态综合判断。' },
    { title: '当前连接数', value: summary.value.activeConnections, hint: '来自网关当前连接统计。' },
    { title: '映射总数', value: summary.value.mappingTotal, hint: `异常映射 ${summary.value.mappingErrorCount} 条。` },
    { title: '近 1 小时失败数', value: summary.value.recentFailureCount, hint: '与访问日志摘要和告警触发共用同一口径。' },
    { title: '限流状态', value: formatRateState(summary.value.rateLimitState), hint: '请结合告警与保护页进一步查看。' },
    { title: '熔断状态', value: formatCircuitState(summary.value.circuitBreakerState), hint: '若异常升高，建议先下钻失败日志。' }
  ]
})

const actionAdvice = computed(() => {
  if (!summary.value) return '暂无建议'
  if (summary.value.recentFailureCount > 0) return '先查看失败日志，再检查映射测试与节点配置。'
  if (summary.value.mappingErrorCount > 0) return '优先处理异常映射，并复核对端节点连通性。'
  return '当前整体稳定，可重点关注热点映射与来源 IP。'
})

const totalSlow = computed(() => trendSeries.value.points.reduce((sum, point) => sum + point.slow, 0))
const yMax = computed(() => Math.max(1, ...trendSeries.value.points.map((point) => Math.max(point.total, point.failed))))
const usableWidth = computed(() => chartWidth - paddingLeft - paddingRight)
const usableHeight = computed(() => chartHeight - paddingTop - paddingBottom)
const gridLines = computed(() => [0, 0.25, 0.5, 0.75, 1].map((ratio) => paddingTop + usableHeight.value * ratio))
const pointX = (index: number) => {
  if (trendSeries.value.points.length <= 1) return paddingLeft
  return paddingLeft + (usableWidth.value * index) / (trendSeries.value.points.length - 1)
}
const pointY = (value: number) => paddingTop + usableHeight.value * (1 - value / yMax.value)
const makePolyline = (selector: (item: DashboardTrendSeries['points'][number]) => number) => trendSeries.value.points.map((point, index) => `${pointX(index)},${pointY(selector(point))}`).join(' ')
const totalPolyline = computed(() => makePolyline((item) => item.total))
const failedPolyline = computed(() => makePolyline((item) => item.failed))
const xTicks = computed(() => {
  if (trendSeries.value.points.length <= 6) return trendSeries.value.points
  const last = trendSeries.value.points.length - 1
  const picked = new Set<number>([0, Math.floor(last / 4), Math.floor(last / 2), Math.floor((last * 3) / 4), last])
  return trendSeries.value.points.filter((_, index) => picked.has(index))
})

const go = (path: string) => router.push(path)
const goToAccessLogs = () => router.push('/access-logs')

onMounted(load)
</script>

<style scoped>
.dashboard-section {
  margin-bottom: 8px;
}
.trend-card {
  display: block;
}
.trend-card :deep(.ant-card-head) {
  border-bottom: 1px solid #f0f0f0;
}
.trend-card :deep(.ant-card-body) {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.dashboard-card {
  height: 100%;
  overflow: hidden;
}
.full-card {
  min-height: 100%;
}
.card-hint {
  margin-top: 8px;
  color: rgba(0, 0, 0, 0.45);
  min-height: 40px;
}
.section-title {
  margin-bottom: 8px;
  font-weight: 600;
}
.list-line {
  width: 100%;
  justify-content: space-between;
}
.trend-card :deep(.ant-card-body) {
  padding-top: 20px;
}
.trend-toolbar {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}
.trend-chart-wrap {
  width: 100%;
  overflow: hidden;
  border-radius: 12px;
  background: #fafafa;
  padding: 12px 12px 8px;
}
.trend-chart {
  width: 100%;
  height: 260px;
}
.trend-chart svg {
  width: 100%;
  height: 100%;
  display: block;
}
.grid-line {
  stroke: rgba(0, 0, 0, 0.08);
  stroke-width: 1;
}
.line-total {
  stroke: #1677ff;
  stroke-width: 3;
}
.line-failed {
  stroke: #ff4d4f;
  stroke-width: 3;
}
.trend-legend {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  margin-top: 8px;
}
.legend-dot {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  margin-right: 6px;
}
.legend-dot.total { background: #1677ff; }
.legend-dot.failed { background: #ff4d4f; }
.legend-dot.slow { background: #faad14; }
.trend-axis {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(80px, 1fr));
  gap: 8px;
  color: rgba(0, 0, 0, 0.45);
  margin-top: 12px;
}
</style>
