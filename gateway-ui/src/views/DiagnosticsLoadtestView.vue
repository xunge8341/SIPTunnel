<template>
  <a-card title="诊断与压测">
    <a-tabs>
      <a-tab-pane key="diag" tab="链路诊断">
        <OpsToolsView />
      </a-tab-pane>
      <a-tab-pane key="cmd" tab="命令任务历史">
        <CommandTasksView />
      </a-tab-pane>
      <a-tab-pane key="file" tab="文件任务历史">
        <FileTasksView />
      </a-tab-pane>
    </a-tabs>
  </a-card>
</template>

<script setup lang="ts">
import OpsToolsView from './OpsToolsView.vue'
import CommandTasksView from './CommandTasksView.vue'
import FileTasksView from './FileTasksView.vue'
</script>
