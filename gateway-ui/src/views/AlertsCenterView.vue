<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="告警中心">
      <a-tabs v-model:activeKey="activeTab">
        <a-tab-pane key="active" tab="活跃告警" />
        <a-tab-pane key="recovered" tab="已恢复告警" />
      </a-tabs>

      <a-table :columns="columns" :data-source="currentList" row-key="id" :pagination="false">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'severity'">
            <status-pill :value="record.severity" kind="severity" />
          </template>
          <template v-if="column.key === 'action'">
            <a-button type="link" @click="selectAlert(record)">查看时间线</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-card v-if="selectedAlert.id" :title="`${selectedAlert.id} 时间线`">
      <a-timeline>
        <a-timeline-item v-for="node in selectedAlert.timeline" :key="node.time" :color="node.color">
          <a-space direction="vertical" size="small">
            <a-typography-text strong>{{ node.time }}</a-typography-text>
            <a-typography-text>{{ node.text }}</a-typography-text>
          </a-space>
        </a-timeline-item>
      </a-timeline>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import StatusPill from '../components/StatusPill.vue'

type Severity = 'critical' | 'high' | 'medium' | 'low'

interface AlertItem {
  id: string
  title: string
  severity: Severity
  source: string
  triggeredAt: string
  recoveredAt?: string
  timeline: Array<{ time: string; text: string; color: string }>
}

const activeTab = ref<'active' | 'recovered'>('active')

const activeAlerts = ref<AlertItem[]>([
  {
    id: 'ALT-1001',
    title: 'gateway-a-02 CPU 持续高于 80%',
    severity: 'high',
    source: 'node-status',
    triggeredAt: '2026-03-12 10:30:00',
    timeline: [
      { time: '10:30:00', text: '首次触发阈值告警', color: 'red' },
      { time: '10:32:20', text: '自动扩容失败，等待人工处理', color: 'orange' }
    ]
  },
  {
    id: 'ALT-1002',
    title: 'ORDER_SYNC 命中限流阈值',
    severity: 'medium',
    source: 'rate-limit',
    triggeredAt: '2026-03-12 10:45:00',
    timeline: [
      { time: '10:45:00', text: '5 分钟内触发 34 次限流', color: 'gold' },
      { time: '10:46:10', text: '已通知值班人员', color: 'blue' }
    ]
  }
])

const recoveredAlerts = ref<AlertItem[]>([
  {
    id: 'ALT-0908',
    title: 'route-config 下发失败率升高',
    severity: 'critical',
    source: 'router',
    triggeredAt: '2026-03-12 08:10:00',
    recoveredAt: '2026-03-12 08:19:30',
    timeline: [
      { time: '08:10:00', text: '连续 3 次配置下发失败', color: 'red' },
      { time: '08:15:00', text: '回滚上一版本路由模板', color: 'orange' },
      { time: '08:19:30', text: '恢复成功', color: 'green' }
    ]
  }
])

const columns = [
  { title: '告警ID', dataIndex: 'id', key: 'id' },
  { title: '告警内容', dataIndex: 'title', key: 'title' },
  { title: '级别', key: 'severity' },
  { title: '来源', dataIndex: 'source', key: 'source' },
  { title: '触发时间', dataIndex: 'triggeredAt', key: 'triggeredAt' },
  { title: '恢复时间', dataIndex: 'recoveredAt', key: 'recoveredAt' },
  { title: '操作', key: 'action' }
]

const currentList = computed(() => (activeTab.value === 'active' ? activeAlerts.value : recoveredAlerts.value))

const selectedAlert = reactive<AlertItem>({
  id: '',
  title: '',
  severity: 'low',
  source: '',
  triggeredAt: '',
  timeline: []
})

const selectAlert = (record: AlertItem) => {
  Object.assign(selectedAlert, record)
}
</script>
