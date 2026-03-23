<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header
      title="告警与保护"
      sub-title="与总览监控、访问日志共用“近 1 小时访问日志”分析口径。"
    >
      <template #extra>
        <a-space>
          <a-button @click="load">刷新</a-button>
          <a-button :loading="recovering" @click="recoverCircuit"
            >熔断恢复</a-button
          >
          <a-button v-if="!editing" type="primary" @click="editing = true"
            >编辑策略</a-button
          >
          <template v-else>
            <a-button @click="cancelEdit">取消编辑</a-button>
            <a-button type="primary" :loading="saving" @click="saveAll"
              >保存保护策略</a-button
            >
          </template>
        </a-space>
      </template>
    </a-page-header>
    <a-alert v-if="notice" type="success" :message="notice" show-icon />
    <actionable-notice
      v-if="error"
      type="error"
      title="保护状态加载失败"
      :summary="error"
      suggestion="先确认 /metrics、/api/protection/state、/api/system/resource-usage 与访问日志统计是否可达，再检查保护策略是否被误改。"
      detail="本页现在同时展示运行态计数、热点目标、临时限制和系统资源；如果页面为空，通常不是 UI 样式问题，而是后端运行态或接口调用失败。"
    />
    <a-spin
      :spinning="loading || saving || recovering || restricting || removing"
    >
      <a-empty v-if="!state" description="暂无保护状态" />
      <template v-else>
        <a-row :gutter="[16, 16]" align="top">
          <a-col :xs="24" :xl="10">
            <a-card class="full-card" title="当前触发状态" :bordered="false">
              <div class="page-section-hint" style="margin-bottom: 12px">
                本页显示运行态命中计数与热点对象，不再只展示静态配置值。
              </div>
              <a-descriptions :column="1" size="small">
                <a-descriptions-item label="统计口径">{{
                  state.analysisWindow || '近 1 小时'
                }}</a-descriptions-item>
                <a-descriptions-item label="当前命中">{{
                  state.currentTriggered.join('、') || '未触发'
                }}</a-descriptions-item>
                <a-descriptions-item label="限流状态">{{
                  state.rateLimitStatus || '正常'
                }}</a-descriptions-item>
                <a-descriptions-item label="熔断状态">{{
                  state.circuitBreakerStatus || '关闭'
                }}</a-descriptions-item>
                <a-descriptions-item label="保护状态">{{
                  state.protectionStatus || '未触发保护'
                }}</a-descriptions-item>
                <a-descriptions-item label="近 1 小时失败数">{{
                  state.recentFailureCount ?? 0
                }}</a-descriptions-item>
                <a-descriptions-item label="近 1 小时慢请求">{{
                  state.recentSlowRequestCount ?? 0
                }}</a-descriptions-item>
                <a-descriptions-item label="当前活跃请求">{{
                  state.currentActiveRequests ?? 0
                }}</a-descriptions-item>
                <a-descriptions-item label="限流命中累计">{{
                  state.rateLimitHitsTotal ?? 0
                }}</a-descriptions-item>
                <a-descriptions-item label="并发拒绝累计">{{
                  state.concurrentRejectsTotal ?? 0
                }}</a-descriptions-item>
                <a-descriptions-item label="允许通过累计">{{
                  state.allowedRequestsTotal ?? 0
                }}</a-descriptions-item>
                <a-descriptions-item label="熔断打开数">{{
                  state.circuitOpenCount ?? 0
                }}</a-descriptions-item>
                <a-descriptions-item label="半开观察数">{{
                  state.circuitHalfOpenCount ?? 0
                }}</a-descriptions-item>
                <a-descriptions-item label="熔断当前相位">{{
                  state.circuitActiveState || 'closed'
                }}</a-descriptions-item>
                <a-descriptions-item label="最近触发类型">{{
                  state.lastTriggeredType || '暂无'
                }}</a-descriptions-item>
                <a-descriptions-item label="最近触发时间">{{
                  state.lastTriggeredTime || '暂无'
                }}</a-descriptions-item>
                <a-descriptions-item label="最近触发对象">{{
                  state.lastTriggeredTarget || '暂无'
                }}</a-descriptions-item>
                <a-descriptions-item label="熔断恢复时间">{{
                  state.circuitLastOpenUntil || '暂无'
                }}</a-descriptions-item>
                <a-descriptions-item label="熔断最近原因">{{
                  state.circuitLastOpenReason || '暂无'
                }}</a-descriptions-item>
              </a-descriptions>
              <a-divider orientation="left">熔断窗口</a-divider>
              <a-empty
                v-if="!state.circuitEntries?.length"
                description="暂无熔断对象"
              />
              <a-list
                v-else
                size="small"
                :data-source="state.circuitEntries"
                style="margin-bottom: 12px"
              >
                <template #renderItem="{ item }">
                  <a-list-item
                    >{{ item.key }} · {{ item.state }} ·
                    {{
                      item.open_until || item.last_cause || '观察中'
                    }}</a-list-item
                  >
                </template>
              </a-list>
              <a-divider orientation="left">热点对象</a-divider>
              <a-row :gutter="12">
                <a-col
                  v-for="scope in state.scopes || []"
                  :key="scope.scope"
                  :xs="24"
                  :md="12"
                  :xl="24"
                >
                  <a-card size="small" class="scope-card">
                    <div class="hot-target-title">{{ scope.label }}</div>
                    <a-descriptions :column="1" size="small">
                      <a-descriptions-item label="RPS">{{
                        scope.rps
                      }}</a-descriptions-item>
                      <a-descriptions-item label="Burst">{{
                        scope.burst
                      }}</a-descriptions-item>
                      <a-descriptions-item label="最大并发">{{
                        scope.max_concurrent
                      }}</a-descriptions-item>
                      <a-descriptions-item label="当前活跃">{{
                        scope.active_requests
                      }}</a-descriptions-item>
                      <a-descriptions-item label="限流命中">{{
                        scope.rate_limit_hits_total
                      }}</a-descriptions-item>
                      <a-descriptions-item label="并发拒绝">{{
                        scope.concurrent_rejects_total
                      }}</a-descriptions-item>
                    </a-descriptions>
                    <template v-if="scope.scope !== 'global'">
                      <a-divider orientation="left">热点目标快捷限制</a-divider>
                      <a-empty
                        v-if="scopeHotTargets(scope).length === 0"
                        description="暂无热点目标"
                      />
                      <a-list
                        v-else
                        size="small"
                        :data-source="scopeHotTargets(scope)"
                      >
                        <template #renderItem="{ item }">
                          <a-list-item>
                            <a-space
                              style="
                                width: 100%;
                                justify-content: space-between;
                              "
                            >
                              <div>
                                <div
                                  class="mono-inline-code wrap-break-anywhere"
                                >
                                  {{ item.target }}
                                </div>
                                <div class="list-hint">
                                  {{ item.sourceLabel }} · 命中 {{ item.count }}
                                </div>
                              </div>
                              <a-space>
                                <a-button
                                  size="small"
                                  @click="
                                    quickRestrict(scope.scope, item.target, 10)
                                  "
                                  >限 10 分钟</a-button
                                >
                                <a-button
                                  size="small"
                                  danger
                                  ghost
                                  @click="
                                    quickRestrict(scope.scope, item.target, 30)
                                  "
                                  >限 30 分钟</a-button
                                >
                              </a-space>
                            </a-space>
                          </a-list-item>
                        </template>
                      </a-list>
                    </template>
                  </a-card>
                </a-col>
              </a-row>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="14">
            <a-card class="full-card" title="保护策略" :bordered="false">
              <a-alert
                type="info"
                show-icon
                message="建议配合 /metrics 与 deploy/observability/prometheus/alerts.yaml 一起使用；策略调优后先观察 5~10 分钟命中率再放量。"
                style="margin-bottom: 16px"
              />
              <a-form layout="vertical">
                <a-row :gutter="16">
                  <a-col :xs="24" :md="12">
                    <a-form-item label="告警规则 1"
                      ><a-input v-model:value="alertRule1" :disabled="!editing"
                    /></a-form-item>
                  </a-col>
                  <a-col :xs="24" :md="12">
                    <a-form-item label="告警规则 2"
                      ><a-input v-model:value="alertRule2" :disabled="!editing"
                    /></a-form-item>
                  </a-col>
                  <a-col :xs="24" :md="8"
                    ><a-form-item label="每秒请求数（RPS）"
                      ><a-input-number
                        v-model:value="rps"
                        :disabled="!editing"
                        :min="1"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                  <a-col :xs="24" :md="8"
                    ><a-form-item label="突发值（Burst）"
                      ><a-input-number
                        v-model:value="burst"
                        :disabled="!editing"
                        :min="1"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                  <a-col :xs="24" :md="8"
                    ><a-form-item label="最大并发数"
                      ><a-input-number
                        v-model:value="maxConcurrent"
                        :disabled="!editing"
                        :min="1"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                  <a-col :xs="24" :md="12"
                    ><a-form-item label="熔断规则 1"
                      ><a-input
                        v-model:value="circuitRule1"
                        :disabled="!editing" /></a-form-item
                  ></a-col>
                  <a-col :xs="24" :md="12"
                    ><a-form-item label="熔断规则 2"
                      ><a-input
                        v-model:value="circuitRule2"
                        :disabled="!editing" /></a-form-item
                  ></a-col>
                  <a-col :xs="24" :md="12"
                    ><a-form-item label="失败阈值"
                      ><a-input-number
                        v-model:value="failureThreshold"
                        :disabled="!editing"
                        :min="1"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                  <a-col :xs="24" :md="12"
                    ><a-form-item label="恢复窗口（秒）"
                      ><a-input-number
                        v-model:value="recoveryWindowSec"
                        :disabled="!editing"
                        :min="5"
                        style="width: 100%" /></a-form-item
                  ></a-col>
                </a-row>
              </a-form>
            </a-card>
          </a-col>
        </a-row>

        <a-row :gutter="[16, 16]" align="top">
          <a-col :xs="24" :xl="10">
            <a-card
              class="full-card"
              title="系统资源与带宽收口"
              :bordered="false"
            >
              <a-alert
                type="info"
                show-icon
                message="从运维视角统一查看 CPU、内存、活跃连接、RTP 端口池，以及当前生效的大文件带宽与热点窗口（MB/Mbps 口径）。"
                style="margin-bottom: 16px"
              />
              <a-empty v-if="!resourceUsage" description="暂无资源视图" />
              <a-descriptions v-else :column="1" size="small">
                <a-descriptions-item label="采集时间">{{
                  resourceUsage.captured_at || '-'
                }}</a-descriptions-item>
                <a-descriptions-item label="运行结论"
                  ><a-tag
                    :color="resourceStatusTagColor(resourceUsage.status_color)"
                    >{{ resourceUsage.status_summary || '未评估' }}</a-tag
                  ></a-descriptions-item
                >
                <a-descriptions-item label="推荐档位">{{
                  resourceUsage.recommended_profile || '平衡模式'
                }}</a-descriptions-item>
                <a-descriptions-item label="运行时已应用档位"
                  >{{
                    resourceUsage.runtime_profile_applied ||
                    resourceUsage.recommended_profile ||
                    '平衡模式'
                  }}<span v-if="resourceUsage.runtime_profile_changed"
                    >（本轮已切换）</span
                  ></a-descriptions-item
                >
                <a-descriptions-item label="CPU / GOMAXPROCS"
                  >{{ resourceUsage.cpu_cores }} /
                  {{ resourceUsage.gomaxprocs }}</a-descriptions-item
                >
                <a-descriptions-item label="Goroutines">{{
                  resourceUsage.goroutines
                }}</a-descriptions-item>
                <a-descriptions-item label="堆内存 / Sys"
                  >{{ formatMiB(resourceUsage.heap_alloc_bytes) }} /
                  {{
                    formatMiB(resourceUsage.heap_sys_bytes)
                  }}</a-descriptions-item
                >
                <a-descriptions-item label="堆空闲 / 栈内存"
                  >{{ formatMiB(resourceUsage.heap_idle_bytes) }} /
                  {{
                    formatMiB(resourceUsage.stack_inuse_bytes)
                  }}</a-descriptions-item
                >
                <a-descriptions-item label="最近 GC">{{
                  resourceUsage.last_gc_time || '暂无'
                }}</a-descriptions-item>
                <a-descriptions-item label="当前活跃请求"
                  >{{ resourceUsage.active_requests }}（{{
                    resourceUsage.active_request_usage_percent ?? 0
                  }}%）</a-descriptions-item
                >
                <a-descriptions-item label="SIP 当前连接">{{
                  resourceUsage.sip_connections
                }}</a-descriptions-item>
                <a-descriptions-item label="RTP 活跃传输">{{
                  resourceUsage.rtp_active_transfers
                }}</a-descriptions-item>
                <a-descriptions-item label="RTP 端口池"
                  >{{ resourceUsage.rtp_port_pool_used }} /
                  {{ resourceUsage.rtp_port_pool_total }}（{{
                    resourceUsage.rtp_port_pool_usage_percent ?? 0
                  }}%）</a-descriptions-item
                >
                <a-descriptions-item label="理论稳定 RTP 并发">{{
                  resourceUsage.theoretical_rtp_transfer_limit ?? 0
                }}</a-descriptions-item>
                <a-descriptions-item label="建议文件并发 / 建议总并发"
                  >{{
                    resourceUsage.recommended_file_transfer_max_concurrent ?? 0
                  }}
                  /
                  {{
                    resourceUsage.recommended_max_concurrent ?? 0
                  }}</a-descriptions-item
                >
                <a-descriptions-item label="建议限流 RPS / Burst"
                  >{{ resourceUsage.recommended_rate_limit_rps ?? 0 }} /
                  {{
                    resourceUsage.recommended_rate_limit_burst ?? 0
                  }}</a-descriptions-item
                >
                <a-descriptions-item label="大文件总带宽上限">{{
                  formatMbps(resourceUsage.configured_generic_download_mbps)
                }}</a-descriptions-item>
                <a-descriptions-item label="单传输保底带宽">{{
                  formatMbps(resourceUsage.configured_generic_per_transfer_mbps)
                }}</a-descriptions-item>
                <a-descriptions-item label="热点缓存 / 热点窗口"
                  >{{
                    formatMBValue(
                      resourceUsage.configured_adaptive_hot_cache_mb
                    )
                  }}
                  /
                  {{
                    formatMBValue(
                      resourceUsage.configured_adaptive_hot_window_mb
                    )
                  }}</a-descriptions-item
                >
                <a-descriptions-item label="RTP jitter/loss / pending"
                  >{{ resourceUsage.observed_jitter_loss_events ?? 0 }} /
                  {{ resourceUsage.observed_gap_timeouts ?? 0 }} /
                  {{
                    resourceUsage.observed_peak_pending ?? 0
                  }}</a-descriptions-item
                >
                <a-descriptions-item label="writer block / circuit open"
                  >{{ resourceUsage.observed_max_writer_block_ms ?? 0 }} ms /
                  {{
                    resourceUsage.observed_circuit_open_count ?? 0
                  }}</a-descriptions-item
                >
                <a-descriptions-item label="运行时写回说明"
                  ><span class="mono-inline-code wrap-break-anywhere">{{
                    resourceUsage.runtime_profile_reason || '未写回'
                  }}</span></a-descriptions-item
                >
                <a-descriptions-item label="建议动作">{{
                  resourceUsage.suggested_actions?.join('；') ||
                  '先观察运行结论后再放量。'
                }}</a-descriptions-item>
              </a-descriptions>
            </a-card>
          </a-col>
          <a-col :xs="24" :xl="14">
            <a-card
              class="full-card"
              title="临时限制 / 黑名单"
              :bordered="false"
            >
              <a-alert
                type="warning"
                show-icon
                message="用于异常来源 IP 或热点资源的临时封禁。建议先限 10~30 分钟观察，避免直接永久拉黑。"
                style="margin-bottom: 16px"
              />
              <a-form layout="vertical">
                <a-row :gutter="16">
                  <a-col :xs="24" :md="6">
                    <a-form-item label="限制范围">
                      <a-select
                        v-model:value="restrictionScope"
                        :options="restrictionScopeOptions"
                      />
                    </a-form-item>
                  </a-col>
                  <a-col :xs="24" :md="8">
                    <a-form-item label="目标">
                      <a-input
                        v-model:value="restrictionTarget"
                        placeholder="来源 IP 或映射 ID"
                      />
                    </a-form-item>
                  </a-col>
                  <a-col :xs="24" :md="4">
                    <a-form-item label="分钟">
                      <a-input-number
                        v-model:value="restrictionMinutes"
                        :min="1"
                        :max="1440"
                        style="width: 100%"
                      />
                    </a-form-item>
                  </a-col>
                  <a-col :xs="24" :md="6">
                    <a-form-item label="原因">
                      <a-input
                        v-model:value="restrictionReason"
                        placeholder="例如：热点异常、压测限流"
                      />
                    </a-form-item>
                  </a-col>
                </a-row>
                <a-space>
                  <a-button
                    type="primary"
                    :loading="restricting"
                    :disabled="!restrictionTarget.trim()"
                    @click="submitRestriction"
                    >添加临时限制</a-button
                  >
                  <a-button @click="resetRestrictionForm">清空</a-button>
                </a-space>
              </a-form>
              <a-divider orientation="left">当前限制项</a-divider>
              <a-empty
                v-if="!state.restrictions?.length"
                description="暂无临时限制"
              />
              <a-list v-else size="small" :data-source="state.restrictions">
                <template #renderItem="{ item }">
                  <a-list-item>
                    <a-space
                      style="width: 100%; justify-content: space-between"
                      align="start"
                    >
                      <div>
                        <div>
                          <a-tag :color="item.active ? 'orange' : 'default'">{{
                            item.scope === 'source'
                              ? '来源 IP'
                              : item.scope === 'mapping'
                                ? '映射资源'
                                : item.scope
                          }}</a-tag>
                          <a-tag :color="item.auto ? 'blue' : 'default'">{{
                            item.auto ? '自动' : '人工'
                          }}</a-tag>
                          <span class="mono-inline-code wrap-break-anywhere">{{
                            item.target
                          }}</span>
                        </div>
                        <div class="list-hint">
                          原因：{{ item.reason || '临时限制'
                          }}{{ item.trigger ? ` · 触发=${item.trigger}` : '' }}
                        </div>
                        <div class="list-hint">
                          创建：{{ item.created_at || '-' }}；到期：{{
                            item.expires_at || '-'
                          }}；{{
                            item.auto_release ? '到期自动恢复' : '人工解除'
                          }}
                        </div>
                      </div>
                      <a-space>
                        <a-tag>{{ restrictionRemainingText(item) }}</a-tag>
                        <a-button
                          size="small"
                          danger
                          ghost
                          :loading="
                            removingKey === `${item.scope}|${item.target}`
                          "
                          @click="removeRestriction(item)"
                          >解除</a-button
                        >
                      </a-space>
                    </a-space>
                  </a-list-item>
                </template>
              </a-list>
            </a-card>
          </a-col>
        </a-row>
      </template>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { gatewayApi } from '../api/gateway'
import ActionableNotice from '../components/ActionableNotice.vue'
import type {
  AlertProtectionState,
  ProtectionRestrictionSnapshot,
  ProtectionScopeSnapshot,
  ProtectionTargetStat,
  SystemResourceUsage
} from '../types/gateway'

type RestrictionScope = 'source' | 'mapping'

const loading = ref(false)
const saving = ref(false)
const recovering = ref(false)
const editing = ref(false)
const restricting = ref(false)
const removing = ref(false)
const removingKey = ref('')
const error = ref('')
const notice = ref('')
const state = ref<AlertProtectionState>()
const resourceUsage = ref<SystemResourceUsage>()
const alertRule1 = ref('')
const alertRule2 = ref('')
const circuitRule1 = ref('')
const circuitRule2 = ref('')
const rps = ref<number | null>(null)
const burst = ref<number | null>(null)
const maxConcurrent = ref<number | null>(null)
const failureThreshold = ref<number | null>(null)
const recoveryWindowSec = ref<number | null>(null)
const restrictionScope = ref<RestrictionScope>('source')
const restrictionTarget = ref('')
const restrictionMinutes = ref<number>(10)
const restrictionReason = ref('热点异常临时限制')

const restrictionScopeOptions = [
  { label: '来源 IP', value: 'source' },
  { label: '映射资源', value: 'mapping' }
]

const syncForm = (payload: AlertProtectionState) => {
  alertRule1.value = payload.alertRules?.[0] ?? ''
  alertRule2.value = payload.alertRules?.[1] ?? ''
  circuitRule1.value = payload.circuitBreakerRules?.[0] ?? ''
  circuitRule2.value = payload.circuitBreakerRules?.[1] ?? ''
  const parsedRps = payload.rateLimitRules?.find((item) =>
    item.startsWith('RPS=')
  )
  const parsedBurst = payload.rateLimitRules?.find((item) =>
    item.startsWith('Burst=')
  )
  const parsedMax = payload.rateLimitRules?.find((item) =>
    item.startsWith('MaxConcurrent=')
  )
  rps.value =
    payload.rps ?? (parsedRps ? Number(parsedRps.split('=')[1]) : null)
  burst.value =
    payload.burst ?? (parsedBurst ? Number(parsedBurst.split('=')[1]) : null)
  maxConcurrent.value =
    payload.maxConcurrent ??
    (parsedMax ? Number(parsedMax.split('=')[1]) : null)
  failureThreshold.value = payload.failureThreshold ?? null
  recoveryWindowSec.value = payload.recoveryWindowSec ?? null
}

const formatMiB = (bytes?: number) =>
  `${((bytes ?? 0) / (1024 * 1024)).toFixed(1)} MB`
const formatMBValue = (mb?: number) => `${Number(mb ?? 0).toFixed(1)} MB`
const formatMbps = (mbps?: number) => `${Number(mbps ?? 0).toFixed(1)} Mbps`
const resourceStatusTagColor = (value?: string) => {
  if (value === 'green') return 'green'
  if (value === 'yellow') return 'orange'
  if (value === 'red') return 'red'
  return 'default'
}

const scopeHotTargets = (scope: ProtectionScopeSnapshot) => {
  const merged = new Map<
    string,
    { target: string; count: number; sourceLabel: string }
  >()
  const addItems = (
    items: ProtectionTargetStat[] | undefined,
    label: string
  ) => {
    for (const item of items ?? []) {
      const existing = merged.get(item.target)
      if (!existing || item.count > existing.count) {
        merged.set(item.target, {
          target: item.target,
          count: item.count,
          sourceLabel: label
        })
      }
    }
  }
  addItems(scope.top_rate_limit_targets, '限流命中')
  addItems(scope.top_concurrent_targets, '并发拒绝')
  return Array.from(merged.values())
    .sort((a, b) => b.count - a.count)
    .slice(0, 4)
}

const restrictionRemainingText = (item: ProtectionRestrictionSnapshot) => {
  if (!item.active || !item.expires_at) return '已失效'
  const remaining = new Date(item.expires_at).getTime() - Date.now()
  if (Number.isNaN(remaining)) return '有效'
  if (remaining <= 0) return '已到期'
  return `剩余 ${Math.max(1, Math.ceil(remaining / 60000))} 分钟`
}

const resetRestrictionForm = () => {
  restrictionScope.value = 'source'
  restrictionTarget.value = ''
  restrictionMinutes.value = 10
  restrictionReason.value = '热点异常临时限制'
}

const load = async () => {
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    const [protectionState, usage] = await Promise.all([
      gatewayApi.fetchProtectionState(),
      gatewayApi.fetchSystemResourceUsage()
    ])
    state.value = protectionState
    resourceUsage.value = usage
    syncForm(protectionState)
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载告警与保护状态失败'
  } finally {
    loading.value = false
  }
}

const cancelEdit = async () => {
  editing.value = false
  await load()
}

const recoverCircuit = async () => {
  recovering.value = true
  error.value = ''
  notice.value = ''
  try {
    const resp = await gatewayApi.recoverProtectionCircuit()
    if (resp.state) {
      state.value = resp.state
      syncForm(resp.state)
    } else {
      await load()
    }
    notice.value = `已执行熔断恢复，清理对象数：${resp.removed ?? 0}`
  } catch (e) {
    error.value = e instanceof Error ? e.message : '熔断恢复失败'
  } finally {
    recovering.value = false
  }
}

const saveAll = async () => {
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    state.value = await gatewayApi.updateProtectionState({
      alertRules: [alertRule1.value, alertRule2.value].filter(Boolean),
      circuitBreakerRules: [circuitRule1.value, circuitRule2.value].filter(
        Boolean
      ),
      rps: rps.value ?? undefined,
      burst: burst.value ?? undefined,
      maxConcurrent: maxConcurrent.value ?? undefined,
      failureThreshold: failureThreshold.value ?? undefined,
      recoveryWindowSec: recoveryWindowSec.value ?? undefined
    })
    syncForm(state.value)
    editing.value = false
    notice.value = '保护策略已保存并重新回读。'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '保存保护策略失败'
  } finally {
    saving.value = false
  }
}

const submitRestriction = async () => {
  if (!restrictionTarget.value.trim()) {
    error.value = '请先填写需要限制的来源 IP 或映射 ID'
    return
  }
  restricting.value = true
  error.value = ''
  notice.value = ''
  try {
    const resp = await gatewayApi.upsertProtectionRestriction({
      scope: restrictionScope.value,
      target: restrictionTarget.value.trim(),
      minutes: restrictionMinutes.value,
      reason: restrictionReason.value.trim() || '热点异常临时限制'
    })
    if (resp.state) {
      state.value = resp.state
      syncForm(resp.state)
    } else {
      await load()
    }
    notice.value = `已新增临时限制：${restrictionTarget.value.trim()}，持续 ${restrictionMinutes.value} 分钟。`
    resetRestrictionForm()
  } catch (e) {
    error.value = e instanceof Error ? e.message : '新增临时限制失败'
  } finally {
    restricting.value = false
  }
}

const quickRestrict = async (scope: string, target: string, minutes = 10) => {
  restrictionScope.value = (
    scope === 'mapping' ? 'mapping' : 'source'
  ) satisfies RestrictionScope
  restrictionTarget.value = target
  restrictionMinutes.value = minutes
  restrictionReason.value =
    scope === 'mapping' ? '热点资源临时限制' : '热点来源 IP 临时限制'
  await submitRestriction()
}

const removeRestriction = async (item: ProtectionRestrictionSnapshot) => {
  removing.value = true
  removingKey.value = `${item.scope}|${item.target}`
  error.value = ''
  notice.value = ''
  try {
    const resp = await gatewayApi.removeProtectionRestriction(
      item.scope,
      item.target
    )
    if (resp.state) {
      state.value = resp.state
      syncForm(resp.state)
    } else {
      await load()
    }
    notice.value = `已解除临时限制：${item.target}`
  } catch (e) {
    error.value = e instanceof Error ? e.message : '解除临时限制失败'
  } finally {
    removing.value = false
    removingKey.value = ''
  }
}

onMounted(load)
</script>

<style scoped>
.full-card {
  height: 100%;
}
.hot-target-block {
  min-height: 180px;
}
.hot-target-title {
  font-weight: 600;
  margin-bottom: 8px;
}
.scope-card {
  margin-bottom: 12px;
}
.list-hint {
  color: rgba(0, 0, 0, 0.45);
  font-size: 12px;
}
</style>
