<template>
  <a-card :title="title" :bordered="false">
    <a-typography-paragraph>{{ description }}</a-typography-paragraph>
    <a-alert type="info" show-icon message="页面占位" description="该模块正在开发中，后续将接入真实业务能力。" />
  </a-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const title = computed(() => (route.meta.title as string) ?? '未命名页面')
const description = computed(
  () =>
    (route.meta.description as string) ??
    '用于承载 SIP 控制面、RTP 文件面与 HTTP 执行链路相关的管理能力。'
)
</script>
