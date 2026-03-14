<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card>
      <a-space style="width: 100%; justify-content: space-between" align="center">
        <a-space>
          <a-typography-title :level="5" style="margin: 0">网络配置</a-typography-title>
          <a-tag :color="isEditable ? 'orange' : 'blue'">{{ isEditable ? '可编辑模式' : '只读模式' }}</a-tag>
        </a-space>
        <a-space>
          <a-button v-if="!isEditable" type="primary" @click="isEditable = true">进入编辑模式</a-button>
          <template v-else>
            <a-button @click="resetDraft">取消修改</a-button>
            <a-button type="primary" :loading="saving" @click="submit">保存配置</a-button>
          </template>
        </a-space>
      </a-space>
      <a-alert
        style="margin-top: 12px"
        type="warning"
        show-icon
        message="高风险配置变更需二次确认"
        description="修改 SIP 协议/端口、RTP 协议/端口范围或并发阈值可能导致连接短时中断。"
      />
    </a-card>

    <a-card title="运行态网络状态">
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :md="12" :xl="6">
          <a-statistic title="SIP TCP/UDP 当前模式" :value="`${config.sip.protocol} / ${config.sip.listenPort}`" />
        </a-col>
        <a-col :xs="24" :md="12" :xl="6">
          <a-statistic title="RTP UDP/TCP 当前模式" :value="`${config.rtp.protocol} / ${formatPortRange(config.rtp.portRangeStart, config.rtp.portRangeEnd)}`" />
        </a-col>
        <a-col :xs="24" :md="12" :xl="6">
          <a-statistic title="端口池使用率" :value="config.portPool.usageRate" suffix="%" :precision="2" />
        </a-col>
        <a-col :xs="24" :md="12" :xl="6">
          <a-progress :percent="config.portPool.usageRate" :status="portPoolStatus" />
        </a-col>
      </a-row>
    </a-card>

    <a-card title="SIP 配置">
      <a-form layout="vertical">
        <a-form-item>
          <template #label>
            <field-label-tooltip label="监听 IP" tooltip="SIP 信令监听网卡地址，建议与服务绑定地址保持一致。" />
          </template>
          <a-input v-model:value="draft.sip.listenIp" :disabled="!isEditable" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="监听端口" tooltip="SIP 服务监听端口，变更后需确保上下游防火墙同步放行。" />
          </template>
          <a-input-number v-model:value="draft.sip.listenPort" :min="1" :max="65535" :disabled="!isEditable" style="width: 100%" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="协议" tooltip="SIP 传输协议，通常为 UDP，切换 TCP 需上下游同时支持。" />
          </template>
          <a-radio-group v-model:value="draft.sip.protocol" :disabled="!isEditable" button-style="solid">
            <a-radio-button value="UDP">UDP</a-radio-button>
            <a-radio-button value="TCP">TCP</a-radio-button>
          </a-radio-group>
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="对外通告地址" tooltip="写入 SIP/SDP 给外部节点回连，通常为公网或跨网可达地址。" />
          </template>
          <a-input v-model:value="draft.sip.advertisedAddress" :disabled="!isEditable" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="域" tooltip="SIP 域用于路由与身份标识，应保持与注册域一致。" />
          </template>
          <a-input v-model:value="draft.sip.domain" :disabled="!isEditable" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="TCP Keepalive" tooltip="启用后可加速识别僵死连接。" />
          </template>
          <a-switch v-model:checked="draft.sip.tcpKeepaliveEnabled" :disabled="!isEditable" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="Keepalive 间隔(ms)" tooltip="TCP keepalive 探测间隔。" />
          </template>
          <a-input-number v-model:value="draft.sip.tcpKeepaliveIntervalMs" :min="1000" :disabled="!isEditable" style="width: 100%" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="读缓冲区(bytes)" tooltip="单连接读取缓冲区大小。" />
          </template>
          <a-input-number v-model:value="draft.sip.tcpReadBufferBytes" :min="1024" :disabled="!isEditable" style="width: 100%" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="写缓冲区(bytes)" tooltip="单连接写入缓冲区大小。" />
          </template>
          <a-input-number v-model:value="draft.sip.tcpWriteBufferBytes" :min="1024" :disabled="!isEditable" style="width: 100%" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="最大连接数" tooltip="SIP TCP 服务端最大并发连接。" />
          </template>
          <a-input-number v-model:value="draft.sip.maxConnections" :min="1" :disabled="!isEditable" style="width: 100%" />
        </a-form-item>
      </a-form>
    </a-card>

    <a-card title="RTP 配置">
      <a-form layout="vertical">
        <a-form-item>
          <template #label>
            <field-label-tooltip label="监听 IP" tooltip="RTP 收流监听网卡地址，应确保与数据面网络可达。" />
          </template>
          <a-input v-model:value="draft.rtp.listenIp" :disabled="!isEditable" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="端口范围" tooltip="RTP 端口池，建议预留连续区间并在网络策略中整体放行。" />
          </template>
          <a-space style="width: 100%">
            <a-input-number v-model:value="draft.rtp.portRangeStart" :min="1" :max="65535" :disabled="!isEditable" style="width: 45%" />
            <span>~</span>
            <a-input-number v-model:value="draft.rtp.portRangeEnd" :min="1" :max="65535" :disabled="!isEditable" style="width: 45%" />
          </a-space>
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="协议" tooltip="RTP/文件传输协议，默认 UDP，切换 TCP 可能增大链路延迟。" />
          </template>
          <a-radio-group v-model:value="draft.rtp.protocol" :disabled="!isEditable" button-style="solid">
            <a-radio-button value="UDP">UDP</a-radio-button>
            <a-radio-button value="TCP">TCP</a-radio-button>
          </a-radio-group>
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="对外通告地址" tooltip="对端用于建立 RTP 回传的地址，建议配置可达的 FQDN 或静态 IP。" />
          </template>
          <a-input v-model:value="draft.rtp.advertisedAddress" :disabled="!isEditable" />
        </a-form-item>
        <a-form-item>
          <template #label>
            <field-label-tooltip label="最大并发传输数" tooltip="限制同时进行的 RTP 传输会话，超过阈值后新任务排队。" />
          </template>
          <a-input-number v-model:value="draft.rtp.maxConcurrentTransfers" :min="1" :max="20000" :disabled="!isEditable" style="width: 100%" />
        </a-form-item>
      </a-form>
    </a-card>

    <a-row :gutter="16">
      <a-col :xs="24" :xl="14">
        <a-card title="连接错误事件表">
          <a-table :data-source="config.connectionErrors" :pagination="{ pageSize: 5 }" size="small" row-key="id">
            <a-table-column title="时间" data-index="occurredAt" key="occurredAt" width="170" />
            <a-table-column title="transport" key="transport" width="120">
              <template #default="{ record }">
                <a-tag :color="record.transport === 'SIP' ? 'blue' : 'purple'">{{ record.transport }} {{ record.protocol }}</a-tag>
              </template>
            </a-table-column>
            <a-table-column title="节点" data-index="nodeId" key="nodeId" width="140" />
            <a-table-column title="错误码" data-index="errorCode" key="errorCode" width="150" />
            <a-table-column title="原因" data-index="reason" key="reason" />
          </a-table>
        </a-card>
      </a-col>
      <a-col :xs="24" :xl="10">
        <a-card title="自检结果面板">
          <a-space direction="vertical" size="small" style="width: 100%; margin-bottom: 12px">
            <a-typography-text type="secondary">按级别筛选（支持多选）</a-typography-text>
            <a-checkbox-group v-model:value="selectedSelfCheckLevels" :options="selfCheckLevelOptions" />
          </a-space>
          <a-list :data-source="filteredSelfCheckItems" size="small" bordered>
            <template #renderItem="{ item }">
              <a-list-item>
                <a-space direction="vertical" size="small" style="width: 100%">
                  <a-space>
                    <a-typography-text strong>{{ item.name }}</a-typography-text>
                    <a-tag :color="selfCheckTagColor(item.level)">{{ selfCheckTagText(item.level) }}</a-tag>
                  </a-space>
                  <a-typography-text type="secondary">{{ item.message }}</a-typography-text>
                  <a-typography-text>建议：{{ item.suggestion }}</a-typography-text>
                  <a-typography-text>动作：{{ item.action_hint }}</a-typography-text>
                  <a :href="item.doc_link" target="_blank" rel="noopener noreferrer" v-if="item.doc_link">查看文档</a>
                </a-space>
              </a-list-item>
            </template>
          </a-list>
        </a-card>
      </a-col>
    </a-row>

    <a-card title="链路测试结果展示（只读）">
      <a-alert type="info" show-icon message="该区域仅展示最近链路测试结果，前端不触发真实压测。" style="margin-bottom: 12px" />
      <a-table :data-source="config.linkTests" :pagination="false" row-key="id" size="small">
        <a-table-column title="场景" data-index="scene" key="scene" />
        <a-table-column title="状态" key="status" width="120">
          <template #default="{ record }">
            <a-tag :color="linkTestTagColor(record.status)">{{ linkTestTagText(record.status) }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column title="平均时延(ms)" data-index="avgLatencyMs" key="avgLatencyMs" width="140" />
        <a-table-column title="丢包率(%)" data-index="packetLossRate" key="packetLossRate" width="120" />
        <a-table-column title="吞吐(Mbps)" data-index="throughputMbps" key="throughputMbps" width="130" />
        <a-table-column title="执行时间" data-index="executedAt" key="executedAt" width="170" />
      </a-table>
    </a-card>

    <a-modal
      v-model:open="riskVisible"
      title="高风险配置确认"
      ok-text="确认变更"
      cancel-text="返回检查"
      :confirm-loading="saving"
      @ok="confirmSave"
    >
      <a-alert
        type="error"
        show-icon
        message="以下字段属于高风险项"
        description="请确认变更窗口、上下游兼容性和回滚预案后再提交。"
        style="margin-bottom: 12px"
      />
      <a-list bordered :data-source="highRiskChanges" size="small">
        <template #renderItem="{ item }">
          <a-list-item>
            <a-space direction="vertical" size="2">
              <strong>{{ item.field }}: {{ item.before }} → {{ item.after }}</strong>
              <a-typography-text type="secondary">{{ item.risk }}</a-typography-text>
            </a-space>
          </a-list-item>
        </template>
      </a-list>
    </a-modal>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import type { HighRiskChange } from '../utils/networkConfig'
import { detectHighRiskChanges, formatPortRange } from '../utils/networkConfig'
import FieldLabelTooltip from '../components/FieldLabelTooltip.vue'
import type { NetworkConfigPayload, UpdateNetworkConfigPayload } from '../types/gateway'

const isEditable = ref(false)
const saving = ref(false)
const riskVisible = ref(false)
const highRiskChanges = ref<HighRiskChange[]>([])
const selectedSelfCheckLevels = ref<Array<'info' | 'warn' | 'error'>>(['info', 'warn', 'error'])
const selfCheckLevelOptions = [
  { label: '信息', value: 'info' },
  { label: '告警', value: 'warn' },
  { label: '错误', value: 'error' }
] as const

const config = reactive<NetworkConfigPayload>({
  sip: {
    listenIp: '',
    listenPort: 5060,
    protocol: 'UDP',
    advertisedAddress: '',
    domain: '',
    tcpKeepaliveEnabled: true,
    tcpKeepaliveIntervalMs: 30000,
    tcpReadBufferBytes: 65536,
    tcpWriteBufferBytes: 65536,
    maxConnections: 2048
  },
  rtp: {
    listenIp: '',
    portRangeStart: 20000,
    portRangeEnd: 20999,
    protocol: 'UDP',
    advertisedAddress: '',
    maxConcurrentTransfers: 100
  },
  portPool: {
    totalAvailablePorts: 0,
    occupiedPorts: 0,
    activeTransfers: 0,
    usageRate: 0
  },
  connectionErrors: [],
  selfCheckItems: [],
  linkTests: []
})

const draft = reactive<UpdateNetworkConfigPayload>({
  sip: {
    listenIp: '',
    listenPort: 5060,
    protocol: 'UDP',
    advertisedAddress: '',
    domain: '',
    tcpKeepaliveEnabled: true,
    tcpKeepaliveIntervalMs: 30000,
    tcpReadBufferBytes: 65536,
    tcpWriteBufferBytes: 65536,
    maxConnections: 2048
  },
  rtp: {
    listenIp: '',
    portRangeStart: 20000,
    portRangeEnd: 20999,
    protocol: 'UDP',
    advertisedAddress: '',
    maxConcurrentTransfers: 100
  }
})

const portPoolStatus = computed(() => {
  if (config.portPool.usageRate >= 85) return 'exception'
  if (config.portPool.usageRate >= 70) return 'active'
  return 'normal'
})

const selfCheckTagColor = (level: 'info' | 'warn' | 'error') => {
  if (level === 'info') return 'success'
  if (level === 'warn') return 'warning'
  return 'error'
}

const selfCheckTagText = (level: 'info' | 'warn' | 'error') => {
  if (level === 'info') return '通过'
  if (level === 'warn') return '告警'
  return '失败'
}

const filteredSelfCheckItems = computed(() => {
  const selected = new Set(selectedSelfCheckLevels.value)
  return config.selfCheckItems.filter((item) => selected.has(item.level))
})

const linkTestTagColor = (status: 'pass' | 'warn' | 'fail') => {
  if (status === 'pass') return 'success'
  if (status === 'warn') return 'warning'
  return 'error'
}

const linkTestTagText = (status: 'pass' | 'warn' | 'fail') => {
  if (status === 'pass') return '通过'
  if (status === 'warn') return '告警'
  return '失败'
}

const loadConfig = async () => {
  const data = await gatewayApi.fetchNetworkConfig()
  Object.assign(config, data)
  resetDraft()
}

const resetDraft = () => {
  Object.assign(draft.sip, config.sip)
  Object.assign(draft.rtp, config.rtp)
  isEditable.value = false
}

const persist = async () => {
  saving.value = true
  try {
    const saved = await gatewayApi.updateNetworkConfig({ sip: { ...draft.sip }, rtp: { ...draft.rtp } })
    Object.assign(config, saved)
    resetDraft()
    riskVisible.value = false
    message.success('网络配置已保存')
  } finally {
    saving.value = false
  }
}

const submit = async () => {
  if (draft.rtp.portRangeStart > draft.rtp.portRangeEnd) {
    message.error('RTP 端口范围起始值不能大于结束值')
    return
  }

  highRiskChanges.value = detectHighRiskChanges(config, draft)
  if (highRiskChanges.value.length > 0) {
    riskVisible.value = true
    return
  }

  await persist()
}

const confirmSave = async () => {
  await persist()
}

onMounted(loadConfig)
</script>
