<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card title="基础信息">
      <a-descriptions :column="3" bordered size="small">
        <a-descriptions-item label="任务ID">{{ detail.id }}</a-descriptions-item>
        <a-descriptions-item label="任务类型">{{ detail.taskKind }}</a-descriptions-item>
        <a-descriptions-item label="状态">
          <a-tag color="processing">{{ detail.status }}</a-tag>
        </a-descriptions-item>
        <a-descriptions-item label="request_id">{{ detail.requestId }}</a-descriptions-item>
        <a-descriptions-item label="trace_id">{{ detail.traceId }}</a-descriptions-item>
        <a-descriptions-item label="节点">{{ detail.nodeId }}</a-descriptions-item>
      </a-descriptions>
    </a-card>

    <a-card title="状态流转时间线">
      <a-timeline>
        <a-timeline-item v-for="item in detail.timeline" :key="item.stage" :color="timelineColor[item.status]">
          <strong>{{ item.stage }}</strong> - {{ item.time }} - {{ item.detail }}（{{ item.operator }}）
        </a-timeline-item>
      </a-timeline>
    </a-card>

    <a-card title="SIP 事件">
      <a-table :columns="sipColumns" :data-source="detail.sipEvents" :pagination="false" row-key="time" size="small" />
    </a-card>

    <a-card title="RTP 分片统计">
      <a-row :gutter="16">
        <a-col :span="6"><a-statistic title="总分片" :value="detail.rtpStats.totalShards" /></a-col>
        <a-col :span="6"><a-statistic title="已接收" :value="detail.rtpStats.receivedShards" /></a-col>
        <a-col :span="6"><a-statistic title="缺失" :value="detail.rtpStats.missingShards" /></a-col>
        <a-col :span="6"><a-statistic title="重传分片" :value="detail.rtpStats.retransmittedShards" /></a-col>
      </a-row>
    </a-card>

    <a-card title="HTTP 执行结果">
      <a-descriptions :column="2" bordered size="small">
        <a-descriptions-item label="api_code">{{ detail.httpResult.apiCode }}</a-descriptions-item>
        <a-descriptions-item label="URL">{{ detail.httpResult.url }}</a-descriptions-item>
        <a-descriptions-item label="Method">{{ detail.httpResult.method }}</a-descriptions-item>
        <a-descriptions-item label="状态码">{{ detail.httpResult.statusCode }}</a-descriptions-item>
        <a-descriptions-item label="耗时">{{ detail.httpResult.durationMs }}ms</a-descriptions-item>
        <a-descriptions-item label="响应片段">
          <a-typography-text code>{{ detail.httpResult.responseSnippet }}</a-typography-text>
        </a-descriptions-item>
      </a-descriptions>
    </a-card>

    <a-card title="审计记录片段">
      <a-list :data-source="detail.auditSnippets" bordered>
        <template #renderItem="{ item }">
          <a-list-item>
            <a-space direction="vertical" size="small">
              <a-typography-text strong>{{ item.action }}（{{ item.actor }}）</a-typography-text>
              <a-typography-text type="secondary">{{ item.time }}</a-typography-text>
              <span>{{ item.summary }}</span>
            </a-space>
          </a-list-item>
        </template>
      </a-list>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { gatewayApi } from '../api/gateway'
import type { TaskDetail } from '../types/gateway'

const route = useRoute()

const detail = ref<TaskDetail>({
  id: '',
  taskKind: 'command',
  requestId: '',
  traceId: '',
  status: 'pending',
  nodeId: '',
  createdAt: '',
  updatedAt: '',
  timeline: [],
  sipEvents: [],
  rtpStats: { totalShards: 0, receivedShards: 0, missingShards: 0, retransmittedShards: 0, bitrateMbps: 0 },
  httpResult: { apiCode: '', url: '', method: '', statusCode: 0, durationMs: 0, responseSnippet: '' },
  auditSnippets: []
})

const timelineColor = {
  done: 'green',
  processing: 'blue',
  wait: 'gray'
}

const sipColumns = [
  { title: '时间', dataIndex: 'time', key: 'time' },
  { title: '方法', dataIndex: 'method', key: 'method' },
  { title: '状态码', dataIndex: 'code', key: 'code' },
  { title: '摘要', dataIndex: 'summary', key: 'summary' }
]

onMounted(async () => {
  const id = String(route.params.id)
  const taskKind = route.params.taskKind === 'file' ? 'file' : 'command'
  detail.value = await gatewayApi.fetchTaskDetail(id, taskKind)
})
</script>
