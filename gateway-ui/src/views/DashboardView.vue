<template>
  <a-row :gutter="16">
    <a-col :span="8" v-for="item in metrics" :key="item.title">
      <a-card>
        <a-statistic :title="item.title" :value="item.value" />
      </a-card>
    </a-col>
  </a-row>
  <a-card style="margin-top: 16px" title="运行概览">
    <a-typography-paragraph>
      SIPTunnel 管理后台基础骨架已完成，可从左侧菜单进入各业务模块。
    </a-typography-paragraph>
    <a-button type="primary" @click="sendReadyMessage">触发全局消息</a-button>
  </a-card>
</template>

<script setup lang="ts">
import { useUiStore } from '../stores/ui'

const uiStore = useUiStore()

const metrics = [
  { title: 'SIP 成功率(%)', value: 99.98 },
  { title: 'RTP 吞吐(MB/s)', value: 126 },
  { title: '活跃节点数', value: 12 }
]

const sendReadyMessage = () => {
  uiStore.notify({
    type: 'success',
    content: '全局消息通道已就绪。'
  })
}
</script>
