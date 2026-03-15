<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-typography-title :level="4" style="margin: 0">System Settings</a-typography-title>
    <a-typography-paragraph type="secondary" style="margin: 0">
      集中管理运维配置、审计、诊断与治理能力。
    </a-typography-paragraph>

    <a-row :gutter="16">
      <a-col :span="12" v-for="item in items" :key="item.title">
        <a-card :title="item.title" :bordered="true">
          <a-typography-paragraph type="secondary">{{ item.desc }}</a-typography-paragraph>
          <router-link :to="item.path">进入</router-link>
        </a-card>
      </a-col>
    </a-row>
  </a-space>
</template>

<script setup lang="ts">
const items = [
  { title: 'Audit Logs', desc: '查询控制面与执行面的审计事件。', path: '/audit-logs' },
  { title: 'Ops Tools', desc: '执行网络诊断、链路测试与配置校验。', path: '/ops-tools' },
  { title: 'Config Governance', desc: '快照、对比与回滚配置。', path: '/config-governance' },
  { title: 'Config Transfer', desc: '导入导出 JSON 配置模板。', path: '/config-transfer' },
  { title: 'Rate Limits', desc: '统一维护限流与并发保护参数。', path: '/rate-limits' }
]
</script>
