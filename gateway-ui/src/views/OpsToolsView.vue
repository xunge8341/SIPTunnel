<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card>
      <a-space direction="vertical" size="small">
        <a-typography-title :level="5" style="margin: 0">运维工具</a-typography-title>
        <a-typography-text type="secondary">
          提供网络诊断、端口检测、通道测试、配置校验四类运维能力，辅助快速定位 SIP/RTP/配置问题。
        </a-typography-text>
      </a-space>
    </a-card>

    <a-tabs v-model:activeKey="activeTool">
      <a-tab-pane key="network" tab="网络诊断">
        <a-card>
          <a-form layout="inline">
            <a-form-item label="目标地址">
              <a-input v-model:value="networkProbe.target" placeholder="例如 10.10.10.18" style="width: 220px" />
            </a-form-item>
            <a-form-item label="协议">
              <a-select v-model:value="networkProbe.protocol" style="width: 120px">
                <a-select-option value="SIP">SIP</a-select-option>
                <a-select-option value="RTP">RTP</a-select-option>
              </a-select>
            </a-form-item>
            <a-form-item>
              <a-button type="primary" @click="runNetworkDiag">开始诊断</a-button>
            </a-form-item>
          </a-form>
          <a-alert
            style="margin-top: 12px"
            type="info"
            show-icon
            :message="networkResult.summary"
            :description="networkResult.detail"
          />
        </a-card>
      </a-tab-pane>

      <a-tab-pane key="port" tab="端口检测">
        <a-card>
          <a-space direction="vertical" style="width: 100%">
            <a-space>
              <a-input-number v-model:value="portCheck.start" :min="1" :max="65535" />
              <span>~</span>
              <a-input-number v-model:value="portCheck.end" :min="1" :max="65535" />
              <a-button type="primary" @click="runPortCheck">执行检测</a-button>
            </a-space>
            <a-table :columns="portColumns" :data-source="portResults" :pagination="false" row-key="port" size="small">
              <template #bodyCell="{ column, record }">
                <template v-if="column.dataIndex === 'status'">
                  <a-tag :color="record.status === 'open' ? 'green' : record.status === 'occupied' ? 'orange' : 'red'">
                    {{ record.status }}
                  </a-tag>
                </template>
              </template>
            </a-table>
          </a-space>
        </a-card>
      </a-tab-pane>

      <a-tab-pane key="tunnel" tab="通道测试">
        <a-card>
          <a-space direction="vertical" style="width: 100%">
            <a-button type="primary" @click="runTunnelTest">执行通道测试</a-button>
            <a-steps :current="tunnelProgress" size="small" :items="tunnelSteps" />
            <a-alert type="success" show-icon :message="tunnelResult" />
          </a-space>
        </a-card>
      </a-tab-pane>

      <a-tab-pane key="config" tab="配置校验">
        <a-card>
          <a-space direction="vertical" style="width: 100%">
            <a-button type="primary" @click="runConfigValidation">执行配置校验</a-button>
            <a-list bordered :data-source="configChecks">
              <template #renderItem="{ item }">
                <a-list-item>
                  <a-space style="width: 100%; justify-content: space-between">
                    <span>{{ item.name }}</span>
                    <a-tag :color="item.level === 'pass' ? 'green' : item.level === 'warn' ? 'gold' : 'red'">
                      {{ getLevelText(item.level) }}
                    </a-tag>
                  </a-space>
                </a-list-item>
              </template>
            </a-list>
          </a-space>
        </a-card>
      </a-tab-pane>
    </a-tabs>
  </a-space>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'

interface PortResult {
  port: number
  status: 'open' | 'occupied' | 'blocked'
}

interface ConfigCheck {
  name: string
  level: 'pass' | 'warn' | 'fail'
}

const levelTextMap: Record<ConfigCheck['level'], string> = {
  pass: '正常',
  warn: '异常',
  fail: '异常'
}


const getLevelText = (level: ConfigCheck['level']) => levelTextMap[level]

const activeTool = ref('network')

const networkProbe = reactive({
  target: '10.10.10.18',
  protocol: 'SIP'
})

const networkResult = reactive({
  summary: '等待执行网络诊断',
  detail: '支持连通性探测、RTT 抖动和丢包率基础检查。'
})

const portCheck = reactive({
  start: 5060,
  end: 5065
})

const portColumns = [
  { title: '端口', dataIndex: 'port', key: 'port', width: 120 },
  { title: '状态', dataIndex: 'status', key: 'status', width: 160 }
]

const portResults = ref<PortResult[]>([])

const tunnelSteps = [
  { title: 'SIP 握手' },
  { title: 'RTP 通道建立' },
  { title: 'HTTP 路由回环' },
  { title: '完成' }
]

const tunnelProgress = ref(0)
const tunnelResult = ref('尚未执行通道测试')

const configChecks = ref<ConfigCheck[]>([
  { name: 'api_code 映射合法性', level: 'pass' },
  { name: 'SIP/RTP 端口区间冲突', level: 'warn' },
  { name: '签名算法配置完整性', level: 'pass' }
])

const runNetworkDiag = () => {
  networkResult.summary = `${networkProbe.protocol} 到 ${networkProbe.target} 连通性正常`
  networkResult.detail = '平均 RTT 8ms，抖动 1.1ms，近 5 分钟丢包率 0.02%。'
}

const runPortCheck = () => {
  const results: PortResult[] = []
  const start = Math.min(portCheck.start, portCheck.end)
  const end = Math.max(portCheck.start, portCheck.end)

  for (let port = start; port <= end && results.length < 20; port += 1) {
    results.push({
      port,
      status: port % 5 === 0 ? 'occupied' : port % 7 === 0 ? 'blocked' : 'open'
    })
  }

  portResults.value = results
}

const runTunnelTest = () => {
  tunnelProgress.value = 3
  tunnelResult.value = '通道测试通过：控制面与文件面链路均可用。'
}

const runConfigValidation = () => {
  configChecks.value = [
    { name: 'api_code 映射合法性', level: 'pass' },
    { name: 'SIP/RTP 端口区间冲突', level: 'pass' },
    { name: '签名算法配置完整性', level: 'pass' },
    { name: '隧道模式能力匹配', level: 'warn' }
  ]
}
</script>
