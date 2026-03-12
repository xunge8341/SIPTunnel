<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card>
      <div class="toolbar">
        <a-typography-text>节点健康监控（每 30s 刷新）</a-typography-text>
        <a-button @click="refresh">手动刷新</a-button>
      </div>
    </a-card>

    <a-row :gutter="16">
      <a-col v-for="node in nodes" :key="node.id" :xs="24" :md="12">
        <a-card :title="node.id">
          <a-descriptions :column="2" size="small">
            <a-descriptions-item label="在线状态"><status-pill :value="node.status" kind="online" /></a-descriptions-item>
            <a-descriptions-item label="当前并发">{{ node.concurrency }}</a-descriptions-item>
          </a-descriptions>
          <a-typography-text type="secondary">CPU / 内存</a-typography-text>
          <a-progress :percent="node.cpu" size="small" :status="node.cpu > 80 ? 'exception' : 'active'" />
          <a-progress :percent="node.memory" size="small" :status="node.memory > 80 ? 'exception' : 'active'" />
          <a-typography-text type="secondary">队列积压</a-typography-text>
          <a-statistic :value="node.backlog" suffix="条" :value-style="{ fontSize: '20px' }" />
        </a-card>
      </a-col>
    </a-row>
  </a-space>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import StatusPill from '../components/StatusPill.vue'

interface NodeStatus {
  id: string
  status: 'online' | 'offline' | 'degraded'
  cpu: number
  memory: number
  backlog: number
  concurrency: number
}

const nodes = ref<NodeStatus[]>([
  { id: 'gateway-a-01', status: 'online', cpu: 38, memory: 52, backlog: 21, concurrency: 140 },
  { id: 'gateway-a-02', status: 'degraded', cpu: 84, memory: 76, backlog: 189, concurrency: 96 },
  { id: 'gateway-b-01', status: 'online', cpu: 41, memory: 48, backlog: 15, concurrency: 112 },
  { id: 'gateway-b-02', status: 'offline', cpu: 0, memory: 0, backlog: 0, concurrency: 0 }
])

const refresh = () => {
  nodes.value = nodes.value.map((node) => ({
    ...node,
    cpu: Math.max(0, Math.min(95, node.cpu + (Math.random() > 0.5 ? 3 : -3))),
    memory: Math.max(0, Math.min(95, node.memory + (Math.random() > 0.5 ? 2 : -2))),
    backlog: Math.max(0, node.backlog + (Math.random() > 0.5 ? 8 : -6)),
    concurrency: Math.max(0, node.concurrency + (Math.random() > 0.5 ? 5 : -4))
  }))
}
</script>

<style scoped>
.toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
