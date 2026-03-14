<template>
  <a-layout style="min-height: 100vh; padding: 24px">
    <a-typography-title :level="3">隧道网关运维后台</a-typography-title>
    <a-row :gutter="16">
      <a-col :span="8" v-for="item in cards" :key="item.title">
        <a-card>
          <a-statistic :title="item.title" :value="item.value" />
        </a-card>
      </a-col>
    </a-row>
    <a-alert style="margin-top: 16px" message="告警中心：当前无 P1 告警" type="success" show-icon />
    <a-alert style="margin-top: 16px" message="链路追踪：SIP->RTP->HTTP 全链路可观测（占位）" type="info" show-icon />
  </a-layout>
</template>

<script setup lang="ts">
const cards = [
  { title: 'RTP 吞吐 (MB/s)', value: 128 },
  { title: 'SIP 成功率 (%)', value: 99.95 },
  { title: '重试队列长度', value: 3 }
]
</script>
