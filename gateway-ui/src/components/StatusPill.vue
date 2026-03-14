<template>
  <a-tag :color="resolvedColor">{{ resolvedLabel }}</a-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    value: string
    kind?: 'status' | 'severity' | 'online'
  }>(),
  {
    kind: 'status'
  }
)

const statusMap: Record<string, { label: string; color: string }> = {
  pending: { label: '待执行', color: 'default' },
  running: { label: '执行中', color: 'processing' },
  success: { label: '成功', color: 'success' },
  failed: { label: '失败', color: 'error' },
  partial_success: { label: '部分成功', color: 'warning' }
}

const severityMap: Record<string, { label: string; color: string }> = {
  critical: { label: '严重', color: 'error' },
  high: { label: '高', color: 'volcano' },
  medium: { label: '中', color: 'gold' },
  low: { label: '低', color: 'blue' }
}

const onlineMap: Record<string, { label: string; color: string }> = {
  online: { label: '已连接', color: 'success' },
  offline: { label: '未连接', color: 'default' },
  degraded: { label: '异常', color: 'warning' }
}

const currentMap = computed(() => {
  if (props.kind === 'severity') return severityMap
  if (props.kind === 'online') return onlineMap
  return statusMap
})

const resolvedLabel = computed(() => currentMap.value[props.value]?.label ?? props.value)
const resolvedColor = computed(() => currentMap.value[props.value]?.color ?? 'default')
</script>
