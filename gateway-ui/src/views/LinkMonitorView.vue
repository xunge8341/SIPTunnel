<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="链路监控" sub-title="这里只放注册状态、GB/T 28181 运行态和会话链路；资源定义与映射配置已经拆到其他页面。">
      <template #extra>
        <a-space>
          <a-button @click="load">刷新</a-button>
          <a-button :loading="actionLoading" @click="runAction('register_now')">手动注册</a-button>
          <a-button :loading="actionLoading" @click="runAction('reregister')">重新注册</a-button>
          <a-button :loading="actionLoading" @click="runAction('heartbeat_once')">发送心跳</a-button>
        </a-space>
      </template>
    </a-page-header>

    <a-alert v-if="notice" type="success" :message="notice" show-icon />
    <actionable-notice
      v-if="error"
      type="error"
      title="链路监控加载失败"
      :summary="error"
      suggestion="先确认 /healthz、/readyz 与 /api/selfcheck 是否可访问，再复核对端与端口配置。"
      detail="如为短时网络抖动，建议先查看‘告警与保护’页的最近触发对象和熔断原因。"
    />

    <a-spin :spinning="loading || actionLoading">
      <a-row :gutter="16" style="margin-bottom: 16px">
        <a-col :xs="24" :sm="12" :xl="6"><a-card><a-statistic title="已注册对端" :value="peerRows.length" /></a-card></a-col>
        <a-col :xs="24" :sm="12" :xl="6"><a-card><a-statistic title="待完成会话" :value="pendingRows.length" /></a-card></a-col>
        <a-col :xs="24" :sm="12" :xl="6"><a-card><a-statistic title="入站会话" :value="inboundRows.length" /></a-card></a-col>
        <a-col :xs="24" :sm="12" :xl="6"><a-card><a-statistic title="连续心跳超时" :value="state?.session.consecutive_heartbeat_timeout ?? 0" /></a-card></a-col>
      </a-row>

      <actionable-notice
        :type="state?.ready_status === 'ready' ? 'success' : 'warning'"
        title="链路就绪摘要"
        :summary="`进程存活=${state?.live_status || '-'}；就绪状态=${state?.ready_status || '-'}`"
        :suggestion="state?.ready_status === 'ready' ? '当前可以接新流量；变更注册、心跳或映射配置后仍建议观察 1~2 个周期。' : '先处理就绪原因中的第一条，再重复检查 /readyz 与本页阶段状态。'"
        :detail="(state?.readiness_reasons || []).join('；') || '本地监听、自检与映射运行态均满足就绪条件。'"
        style="margin-bottom: 16px"
      >
        <template #actions>
          <a-button size="small" @click="go('/nodes-tunnels')">打开节点与级联</a-button>
          <a-button size="small" @click="go('/alerts-protection')">查看保护状态</a-button>
        </template>
      </actionable-notice>

      <actionable-notice
        v-if="sipPortWarning"
        type="warning"
        title="SIP 发送链路提示"
        :summary="sipPortWarning"
        suggestion="重点核对本端监听端口、对端信令端口、SIP transport 和网络策略是否完全一致。"
        detail="这类问题多数发生在发送前编码、端口复用或 UDP 超时阶段，不一定是对端主动拒绝。"
        style="margin-bottom: 16px"
      >
        <template #actions>
          <a-button size="small" @click="go('/nodes-tunnels')">检查节点配置</a-button>
        </template>
      </actionable-notice>

      <a-row :gutter="16" align="top">
        <a-col :xs="24" :md="12" :xl="10">
          <a-card title="节点连接状态" :bordered="false" style="height: 100%">
            <a-descriptions :column="1" bordered size="small">
              <a-descriptions-item label="注册状态">{{ statusLabel(state?.session.registration_status, 'registration') }}</a-descriptions-item>
              <a-descriptions-item label="心跳状态">{{ statusLabel(state?.session.heartbeat_status, 'heartbeat') }}</a-descriptions-item>
              <a-descriptions-item label="当前阶段">{{ phaseLabel(state?.session.phase) }}</a-descriptions-item>
              <a-descriptions-item label="阶段更新时间">{{ formatTime(state?.session.phase_updated_at) }}</a-descriptions-item>
              <a-descriptions-item label="最近注册时间">{{ formatTime(state?.session.last_register_time) }}</a-descriptions-item>
              <a-descriptions-item label="最近心跳时间">{{ formatTime(state?.session.last_heartbeat_time) }}</a-descriptions-item>
              <a-descriptions-item label="最近失败原因">{{ state?.session.last_failure_reason || '-' }}</a-descriptions-item>
              <a-descriptions-item label="下一次重试">{{ formatTime(state?.session.next_retry_time) }}</a-descriptions-item>
              <a-descriptions-item label="Catalog 续订周期">{{ state?.config.catalog_subscribe_expires_sec || 0 }} 秒</a-descriptions-item>
              <a-descriptions-item label="快照时间">{{ formatTime(state?.updated_at) }}</a-descriptions-item>
            </a-descriptions>
          </a-card>
        </a-col>
        <a-col :xs="24" :md="12" :xl="14">
          <a-card title="已注册对端" :bordered="false" style="height: 100%">
            <a-table size="small" :data-source="peerRows" :columns="peerColumns" row-key="device_id" :pagination="false" :locale="emptyLocale">
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'auth_required'">
                  <a-tag :color="record.auth_required ? 'orange' : 'default'">{{ record.auth_required ? '需要鉴权' : '无需鉴权' }}</a-tag>
                </template>
              </template>
            </a-table>
          </a-card>
        </a-col>
      </a-row>

      <a-row :gutter="16" style="margin-top: 16px">
        <a-col :xs="24" :md="12" :xl="12">
          <a-card title="待完成会话" :bordered="false">
            <a-table size="small" :data-source="pendingRows" :columns="pendingColumns" row-key="call_id" :pagination="false" :locale="emptyLocale" />
          </a-card>
        </a-col>
        <a-col :xs="24" :md="12" :xl="12">
          <a-card title="入站会话" :bordered="false">
            <a-table size="small" :data-source="inboundRows" :columns="inboundColumns" row-key="call_id" :pagination="false" :locale="emptyLocale" />
          </a-card>
        </a-col>
      </a-row>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { gatewayApi } from '../api/gateway'
import { formatDateTimeText } from '../utils/date'
import ActionableNotice from '../components/ActionableNotice.vue'
import type { LinkMonitorPayload, TunnelSessionActionPayload } from '../types/gateway'

const router = useRouter()
const loading = ref(false)
const actionLoading = ref(false)
const error = ref('')
const notice = ref('')
const state = ref<LinkMonitorPayload>()

const emptyLocale = { emptyText: '暂无数据' }
const peerColumns = [
  { title: '级联对端编码', dataIndex: 'device_id', key: 'device_id' },
  { title: '远端地址', dataIndex: 'remote_addr', key: 'remote_addr' },
  { title: '订阅到期', dataIndex: 'subscription_expires_at', key: 'subscription_expires_at' },
  { title: '鉴权', dataIndex: 'auth_required', key: 'auth_required', width: 110 },
  { title: '最近错误', dataIndex: 'last_error', key: 'last_error' }
]
const pendingColumns = [
  { title: 'Call-ID', dataIndex: 'call_id', key: 'call_id' },
  { title: '资源编码', dataIndex: 'device_id', key: 'device_id' },
  { title: '映射 ID', dataIndex: 'mapping_id', key: 'mapping_id' },
  { title: '承载', dataIndex: 'response_mode', key: 'response_mode' },
  { title: '阶段', dataIndex: 'stage', key: 'stage' },
  { title: '阶段更新时间', dataIndex: 'last_stage_at', key: 'last_stage_at' },
  { title: '最近错误', dataIndex: 'last_error', key: 'last_error' }
]
const inboundColumns = [
  { title: 'Call-ID', dataIndex: 'call_id', key: 'call_id' },
  { title: '资源编码', dataIndex: 'device_id', key: 'device_id' },
  { title: '回调地址', dataIndex: 'callback_addr', key: 'callback_addr' },
  { title: 'RTP 远端', dataIndex: 'remote_rtp_ip', key: 'remote_rtp_ip' },
  { title: '阶段', dataIndex: 'stage', key: 'stage' },
  { title: '阶段更新时间', dataIndex: 'last_stage_at', key: 'last_stage_at' },
  { title: '最近错误', dataIndex: 'last_error', key: 'last_error' }
]

const formatTime = (value?: string | null) => formatDateTimeText(value, '-')

const peerRows = computed(() => (state.value?.gb28181?.peers ?? []).map((item) => ({ ...item, subscription_expires_at: formatTime(item.subscription_expires_at) })))
const pendingRows = computed(() => (state.value?.gb28181?.pending_sessions ?? []).map((item) => ({ ...item, last_stage_at: formatTime(item.last_stage_at) })))
const inboundRows = computed(() => (state.value?.gb28181?.inbound_sessions ?? []).map((item) => ({ ...item, last_stage_at: formatTime(item.last_stage_at) })))
const sipPortWarning = computed(() => {
  const reason = String(state.value?.session?.last_failure_reason || '').toLowerCase()
  if (reason.includes('parse outgoing sip payload')) {
    return 'REGISTER 在本端发送前就失败了：当前错误发生在本端 SIP 报文编码/发送链路，而不是对端返回。请优先检查本端发送路径、SIP 监听端口复用与请求编码。'
  }
  if (reason.includes('read udp') && reason.includes('i/o timeout')) {
    return '最近的 REGISTER 超时看起来是 SIP 信令没有走在两端约定的 SIP 端口上。请重点检查：本级域 SIP 监听端口、级联对端信令端口、SIP transport 是否一致，以及防火墙是否放行。'
  }
  return ''
})

const phaseLabel = (raw?: string) => {
  const value = String(raw || '').toLowerCase()
  if (value === 'initializing') return '初始化'
  if (value === 'waiting_peer') return '等待对端'
  if (value === 'registering') return '注册中'
  if (value === 'auth_challenge') return '鉴权挑战'
  if (value === 'registered') return '已注册'
  if (value === 'heartbeat_ready') return '待心跳'
  if (value === 'heartbeat_due') return '触发心跳'
  if (value === 'heartbeat_healthy') return '心跳正常'
  if (value === 'retry_wait') return '等待重试'
  return raw || '-'
}

const statusLabel = (raw: string | undefined, kind: 'registration' | 'heartbeat') => {
  const value = String(raw || '').toLowerCase()
  if (kind === 'registration') {
    if (value === 'registered') return '已注册'
    if (value === 'registering') return '注册中'
    if (value === 'failed') return '注册失败'
    if (value === 'waiting_peer') return '等待对端发起'
    if (value === 'unregistered') return '未注册'
  }
  if (kind === 'heartbeat') {
    if (value === 'healthy') return '正常'
    if (value === 'timeout') return '超时'
    if (value === 'unknown') return '未知'
  }
  return raw || '-'
}

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    state.value = await gatewayApi.fetchLinkMonitor()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载链路监控失败'
  } finally {
    loading.value = false
  }
}

const actionText = (action: TunnelSessionActionPayload['action']) => {
  if (action === 'register_now') return '手动注册'
  if (action === 'reregister') return '重新注册'
  if (action === 'heartbeat_once') return '发送心跳'
  return action
}

const go = (path: string) => router.push(path)


const runAction = async (action: TunnelSessionActionPayload['action']) => {
  actionLoading.value = true
  error.value = ''
  try {
    await gatewayApi.triggerTunnelSessionAction({ action })
    notice.value = `已执行操作：${actionText(action)}`
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '执行链路动作失败'
  } finally {
    actionLoading.value = false
  }
}

onMounted(load)
</script>
