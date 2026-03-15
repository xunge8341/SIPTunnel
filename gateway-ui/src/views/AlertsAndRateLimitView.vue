<template>
  <a-card title="告警与限流">
    <a-tabs>
      <a-tab-pane key="alert" tab="告警中心">
        <AlertsCenterView />
      </a-tab-pane>
      <a-tab-pane key="rate" tab="限流策略">
        <RateLimitPoliciesView />
      </a-tab-pane>
    </a-tabs>
  </a-card>
</template>

<script setup lang="ts">
import AlertsCenterView from './AlertsCenterView.vue'
import RateLimitPoliciesView from './RateLimitPoliciesView.vue'
</script>
