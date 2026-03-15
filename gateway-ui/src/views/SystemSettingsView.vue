<template>
  <a-card title="系统设置">
    <a-typography-paragraph type="secondary">
      集中管理配置治理、导入导出与保留策略。保存后将立即落库并可刷新回读。
    </a-typography-paragraph>
    <a-tabs>
      <a-tab-pane key="governance" tab="配置治理">
        <ConfigGovernanceView />
      </a-tab-pane>
      <a-tab-pane key="transfer" tab="配置导入导出">
        <ConfigTransferView />
      </a-tab-pane>
    </a-tabs>
  </a-card>
</template>

<script setup lang="ts">
import ConfigGovernanceView from './ConfigGovernanceView.vue'
import ConfigTransferView from './ConfigTransferView.vue'
</script>
