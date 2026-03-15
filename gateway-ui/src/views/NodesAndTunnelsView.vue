<template>
  <a-space direction="vertical" style="width: 100%" size="middle">
    <a-card title="节点与隧道统一配置">
      <a-alert type="info" show-icon message="本页整合本端、对端、网络模式、SIP/RTP、会话参数、安全与高级设置，保存后自动回读。" />
    </a-card>

    <a-card title="本端节点与对端节点">
      <NodeConfigView />
    </a-card>

    <a-card title="网络能力模式 / SIP / RTP / 隧道会话参数">
      <TunnelConfigView />
    </a-card>

    <a-card title="安全与认证（含 AES / SM4）">
      <SecurityCenterView />
    </a-card>

    <a-card title="高级设置">
      <NetworkConfigView />
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import NodeConfigView from './NodeConfigView.vue'
import TunnelConfigView from './TunnelConfigView.vue'
import NetworkConfigView from './NetworkConfigView.vue'
import SecurityCenterView from './SecurityCenterView.vue'
</script>
