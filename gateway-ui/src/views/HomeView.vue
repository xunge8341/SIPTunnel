<template>
  <a-layout style="min-height: 100vh; padding: 24px">
    <a-card title="隧道网关管理界面" style="max-width: 720px; margin: 0 auto">
      <a-typography-paragraph>
        Monorepo 初始化完成，前端与后端均可独立启动。
      </a-typography-paragraph>
      <a-descriptions bordered :column="1">
        <a-descriptions-item label="Gateway 状态">{{ health.status }}</a-descriptions-item>
        <a-descriptions-item label="后端健康检查">GET /healthz</a-descriptions-item>
      </a-descriptions>
    </a-card>
  </a-layout>
</template>

<script setup lang="ts">
import { useHealthStore } from '../stores/health'

const health = useHealthStore()
</script>
