<template>
  <a-layout style="min-height: 100vh; padding: 24px">
    <a-card title="HTTP 映射隧道网关管理界面" style="max-width: 720px; margin: 0 auto">
      <a-typography-paragraph>
        当前主线为“HTTP 映射隧道模式”：围绕本端节点/对端节点、网络模式、能力矩阵与隧道映射进行运维。
      </a-typography-paragraph>
      <a-typography-paragraph type="secondary">
        历史术语 route/api_code/template 仅用于兼容，不作为默认产品表达。
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
