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

    <a-row :gutter="16">
      <a-col :span="12">
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
          </a-form>
        </a-card>
      </a-col>

      <a-col :span="12">
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
                <a-input-number
                  v-model:value="draft.rtp.portRangeStart"
                  :min="1"
                  :max="65535"
                  :disabled="!isEditable"
                  style="width: 45%"
                />
                <span>-</span>
                <a-input-number
                  v-model:value="draft.rtp.portRangeEnd"
                  :min="1"
                  :max="65535"
                  :disabled="!isEditable"
                  style="width: 45%"
                />
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
              <a-input-number
                v-model:value="draft.rtp.maxConcurrentTransfers"
                :min="1"
                :max="20000"
                :disabled="!isEditable"
                style="width: 100%"
              />
            </a-form-item>
          </a-form>
        </a-card>
      </a-col>
    </a-row>

    <a-card title="端口池状态">
      <a-row :gutter="12">
        <a-col :span="8"><a-statistic title="可用端口总数" :value="config.portPool.totalAvailablePorts" /></a-col>
        <a-col :span="8"><a-statistic title="已占用端口数" :value="config.portPool.occupiedPorts" /></a-col>
        <a-col :span="8"><a-statistic title="活跃传输数" :value="config.portPool.activeTransfers" /></a-col>
      </a-row>
      <a-divider />
      <a-descriptions :column="1" size="small" bordered>
        <a-descriptions-item label="当前 RTP 端口范围">{{ formatPortRange(draft.rtp.portRangeStart, draft.rtp.portRangeEnd) }}</a-descriptions-item>
      </a-descriptions>
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
import { onMounted, reactive, ref } from 'vue'
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

const config = reactive<NetworkConfigPayload>({
  sip: {
    listenIp: '',
    listenPort: 5060,
    protocol: 'UDP',
    advertisedAddress: '',
    domain: ''
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
    activeTransfers: 0
  }
})

const draft = reactive<UpdateNetworkConfigPayload>({
  sip: {
    listenIp: '',
    listenPort: 5060,
    protocol: 'UDP',
    advertisedAddress: '',
    domain: ''
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
