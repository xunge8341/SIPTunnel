<template>
  <a-alert :type="type" show-icon :message="title">
    <template #description>
      <a-space direction="vertical" size="small" style="width: 100%">
        <div><strong>结论：</strong>{{ summary }}</div>
        <div v-if="suggestion"><strong>建议：</strong>{{ suggestion }}</div>
        <div v-if="detail" class="actionable-detail"><strong>详情：</strong>{{ detail }}</div>
        <a-space v-if="$slots.actions || docLink" wrap>
          <slot name="actions" />
          <a v-if="docLink" :href="docLink" target="_blank" rel="noopener noreferrer">查看文档</a>
        </a-space>
      </a-space>
    </template>
  </a-alert>
</template>

<script setup lang="ts">
defineProps<{
  type?: 'success' | 'info' | 'warning' | 'error'
  title: string
  summary: string
  suggestion?: string
  detail?: string
  docLink?: string
}>()
</script>

<style scoped>
.actionable-detail {
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
