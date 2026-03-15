<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-card :bordered="false">
      <a-page-header title="节点与隧道" sub-title="统一工作流：先看节点状态，再配置能力、安全与高级参数。">
        <template #extra>
          <a-space>
            <a-button>刷新回读</a-button>
            <a-button>校验配置</a-button>
            <a-button type="primary">保存并应用</a-button>
          </a-space>
        </template>
      </a-page-header>
      <a-row :gutter="[12, 12]">
        <a-col v-for="item in headerCards" :key="item.title" :xs="24" :lg="6">
          <a-card size="small">
            <a-statistic :title="item.title" :value="item.value" />
            <a-typography-text type="secondary">{{ item.hint }}</a-typography-text>
          </a-card>
        </a-col>
      </a-row>
    </a-card>

    <a-card title="A. 节点基础信息" :bordered="false">
      <a-form layout="vertical">
        <a-row :gutter="16">
          <a-col :xs="24" :xl="12" v-for="field in basicFields" :key="field.label">
            <a-form-item :label="field.label" :extra="field.extra">
              <a-input v-model:value="field.value" />
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-card>

    <a-card title="B. 网络能力与隧道能力" :bordered="false">
      <a-row :gutter="16">
        <a-col :xs="24" :xl="10">
          <a-form layout="vertical">
            <a-form-item label="网络能力模式" extra="决定 SIP/RTP 以及请求响应承载策略。">
              <a-select v-model:value="capabilityMode" :options="modeOptions" />
            </a-form-item>
            <a-form-item label="能力摘要" extra="常用能力默认开启，异常能力需按网络质量评估。">
              <a-textarea :value="capabilitySummary" :rows="4" readonly />
            </a-form-item>
          </a-form>
        </a-col>
        <a-col :xs="24" :xl="14">
          <a-table :columns="capabilityColumns" :data-source="capabilityMatrix" :pagination="false" row-key="name" size="small">
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'support'">
                <a-tag :color="record.support ? 'green' : 'red'">{{ record.support ? '支持' : '不支持' }}</a-tag>
              </template>
            </template>
          </a-table>
        </a-col>
      </a-row>
    </a-card>

    <a-card title="C. 安全与加密" :bordered="false">
      <a-form layout="vertical">
        <a-row :gutter="16">
          <a-col :xs="24" :xl="8">
            <a-form-item label="节点认证方式" extra="建议生产环境使用签名策略，避免仅 Token。">
              <a-select v-model:value="security.auth" :options="authOptions" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :xl="8">
            <a-form-item label="Token / 签名策略" extra="当前默认 HMAC-SHA256，保留国密升级位。">
              <a-select v-model:value="security.signature" :options="signOptions" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :xl="8">
            <a-form-item label="传输加密开关" extra="关闭会降低跨网链路安全性。">
              <a-switch v-model:checked="security.encrypted" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :xl="8">
            <a-form-item label="加密算法" extra="AES/SM4 属于节点通信配置，集中在本页维护。">
              <a-select v-model:value="security.algorithm" :options="algorithmOptions" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :xl="8">
            <a-form-item label="密钥来源" extra="建议接入密钥管理系统，避免明文落盘。">
              <a-select v-model:value="security.keySource" :options="keySourceOptions" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :xl="8">
            <a-form-item label="生效范围" extra="可限制在指定映射组，降低改动面。">
              <a-select v-model:value="security.scope" :options="scopeOptions" />
            </a-form-item>
          </a-col>
        </a-row>
      </a-form>
    </a-card>

    <a-collapse>
      <a-collapse-panel key="advanced" header="D. 高级设置（默认折叠）">
        <a-form layout="vertical">
          <a-row :gutter="16">
            <a-col :xs="24" :md="12" :xl="8" v-for="item in advancedFields" :key="item.label">
              <a-form-item :label="item.label" :extra="item.extra">
                <a-input-number v-model:value="item.value" style="width: 100%" :min="item.min" />
              </a-form-item>
            </a-col>
          </a-row>
        </a-form>
      </a-collapse-panel>
    </a-collapse>
  </a-space>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const headerCards = [
  { title: '本端节点摘要', value: 'A 网关（在线）', hint: '监听 0.0.0.0:5060，近 1h 心跳稳定。' },
  { title: '对端节点摘要', value: 'B 网关（在线）', hint: '信令 RTT 42ms，最近无重连。' },
  { title: '当前网络能力模式', value: '增强传输模式', hint: '支持大请求/大响应/流式响应。' },
  { title: '当前加密状态', value: '已启用（AES-256）', hint: '主链路已加密，密钥来源 KMS。' }
]

const basicFields = ref([
  { label: '本端名称', value: 'A-生产节点', extra: '用于运维标识，不要填写内部 ID。' },
  { label: '本端监听地址', value: '0.0.0.0', extra: '建议仅绑定必要网卡。' },
  { label: '本端监听端口', value: '5060', extra: '默认 SIP 控制面端口。' },
  { label: '本端入口', value: 'http://10.1.2.10:8080', extra: '运维常用入口地址。' },
  { label: '对端名称', value: 'B-业务节点', extra: '与对端运维约定一致。' },
  { label: '对端地址', value: '10.9.0.15', extra: '建议填写固定地址。' },
  { label: '对端信令地址', value: 'sip:10.9.0.15:5060', extra: '用于 SIP 控制连接。' },
  { label: '健康检查配置', value: '每 15 秒，超时 3 秒，失败 3 次告警', extra: '用于快速定位链路可用性。' }
])

const capabilityMode = ref('enhanced')
const modeOptions = [
  { label: '基础模式', value: 'basic' },
  { label: '增强传输模式', value: 'enhanced' },
  { label: '高可靠模式', value: 'reliable' }
]
const capabilitySummary = 'SIP 控制稳定；RTP 支持单向大载荷；支持大请求/大响应；支持流式响应。'
const capabilityColumns = [
  { title: '能力项', dataIndex: 'name', key: 'name' },
  { title: '说明', dataIndex: 'desc', key: 'desc' },
  { title: '支持情况', dataIndex: 'support', key: 'support' }
]
const capabilityMatrix = [
  { name: 'SIP 能力', desc: '控制面状态同步与命令下发', support: true },
  { name: 'RTP 能力', desc: '文件面单向大载荷传输', support: true },
  { name: '大请求支持', desc: '请求体超阈值自动走 RTP', support: true },
  { name: '大响应支持', desc: '响应体超阈值自动走 RTP', support: true },
  { name: '流式支持', desc: '支持持续响应流转发', support: false }
]

const security = ref({ auth: 'signature', signature: 'hmac', encrypted: true, algorithm: 'aes', keySource: 'kms', scope: 'all' })
const authOptions = [
  { label: 'Token + 签名', value: 'signature' },
  { label: '仅 Token（不推荐）', value: 'token' }
]
const signOptions = [
  { label: 'HMAC-SHA256（当前）', value: 'hmac' },
  { label: 'SM3/SM4 预留策略', value: 'sm' }
]
const algorithmOptions = [
  { label: 'AES-256-GCM', value: 'aes' },
  { label: 'SM4-GCM', value: 'sm4' }
]
const keySourceOptions = [
  { label: 'KMS 密钥管理', value: 'kms' },
  { label: '本地密钥文件', value: 'file' }
]
const scopeOptions = [
  { label: '全量隧道映射', value: 'all' },
  { label: '指定映射组', value: 'group' }
]

const advancedFields = ref([
  { label: '心跳间隔（秒）', value: 15, min: 1, extra: '链路稳定建议 10-20 秒。' },
  { label: '注册重试次数', value: 3, min: 0, extra: '异常网络建议不少于 3 次。' },
  { label: '会话上限', value: 500, min: 10, extra: '超过上限会触发保护动作。' },
  { label: '请求超时（毫秒）', value: 5000, min: 100, extra: '按业务 SLA 设置。' },
  { label: '响应超时（毫秒）', value: 8000, min: 100, extra: '建议大于请求超时。' },
  { label: '网络抖动容忍（毫秒）', value: 150, min: 0, extra: '用于 RTP 重组调优。' }
])
</script>
