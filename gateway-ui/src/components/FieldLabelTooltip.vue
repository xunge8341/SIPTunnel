<template>
  <a-space :size="4">
    <span>{{ label }}</span>
    <a-tooltip :title="tooltip">
      <a-typography-text type="secondary" style="cursor: help">ⓘ</a-typography-text>
    </a-tooltip>
  </a-space>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  tooltip: string
}>()
</script>
