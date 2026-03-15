<template>
  <a-space direction="vertical" style="width:100%">
    <a-card title="告警/限流/熔断统一配置">
      <a-typography-paragraph type="secondary">在同一页面配置规则并查看当前命中和保护状态，保存后回读真实值。</a-typography-paragraph>
      <RateLimitPoliciesView />
      <a-divider />
      <AlertsCenterView />
      <a-divider />
      <a-descriptions bordered :column="1" size="small" title="当前保护状态（实时）">
        <a-descriptions-item label="限流状态">{{ opsSummary.rate_limit_status }}</a-descriptions-item>
        <a-descriptions-item label="熔断状态">{{ opsSummary.circuit_breaker_state }}</a-descriptions-item>
        <a-descriptions-item label="资源保护状态">{{ opsSummary.protection_status }}</a-descriptions-item>
      </a-descriptions>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import AlertsCenterView from './AlertsCenterView.vue'
import RateLimitPoliciesView from './RateLimitPoliciesView.vue'
import { gatewayApi } from '../api/gateway'

const opsSummary = reactive({ rate_limit_status: '-', circuit_breaker_state: '-', protection_status: '-' })
onMounted(async () => Object.assign(opsSummary, await gatewayApi.fetchDashboardOpsSummary()))
</script>
