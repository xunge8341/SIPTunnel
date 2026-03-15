<template>
  <a-card title="节点与隧道统一配置" :bordered="false">
    <a-tabs>
      <a-tab-pane key="node" tab="节点配置（本端/对端）">
        <NodeConfigView />
      </a-tab-pane>
      <a-tab-pane key="tunnel" tab="隧道能力（网络模式/SIP/RTP）">
        <TunnelConfigView />
      </a-tab-pane>
      <a-tab-pane key="network" tab="网络高级参数">
        <NetworkConfigView />
      </a-tab-pane>
    </a-tabs>
  </a-card>
</template>

<script setup lang="ts">
import NodeConfigView from './NodeConfigView.vue'
import TunnelConfigView from './TunnelConfigView.vue'
import NetworkConfigView from './NetworkConfigView.vue'
</script>
