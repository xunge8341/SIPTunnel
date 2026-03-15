<template>
  <a-space direction="vertical" style="width: 100%" size="large">
    <a-card :bordered="false">
      <a-page-header title="运维审计" sub-title="聚焦谁在什么时候改了什么，以及执行结果。" />
    </a-card>
    <a-card :bordered="false">
      <a-table :columns="columns" :data-source="rows" row-key="id">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'result'">
            <a-tag :color="record.result === '成功' ? 'green' : 'red'">{{ record.result }}</a-tag>
          </template>
          <template v-else-if="column.key === 'action'">
            <a-button type="link" @click="openDetail(record)">查看详情</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-drawer v-model:open="detailOpen" title="审计详情" :width="700">
      <a-descriptions bordered :column="1">
        <a-descriptions-item label="时间">{{ current?.time }}</a-descriptions-item>
        <a-descriptions-item label="操作类型">{{ current?.type }}</a-descriptions-item>
        <a-descriptions-item label="操作对象">{{ current?.target }}</a-descriptions-item>
        <a-descriptions-item label="操作人">{{ current?.operator }}</a-descriptions-item>
        <a-descriptions-item label="request_id">req-audit-{{ current?.id }}</a-descriptions-item>
        <a-descriptions-item label="trace_id">trace-audit-{{ current?.id }}</a-descriptions-item>
        <a-descriptions-item label="原始 payload">{"enabled": true, "timeout": 5000, "scope": "payment"}</a-descriptions-item>
        <a-descriptions-item label="变更前后对比">变更前：timeout=3000；变更后：timeout=5000。</a-descriptions-item>
        <a-descriptions-item label="调试字段">审批单号 OA-2026-0315-009；来源终端 10.2.8.3。</a-descriptions-item>
      </a-descriptions>
    </a-drawer>
  </a-space>
</template>

<script setup lang="ts">
import { ref } from 'vue'

type AuditRow = {
  id: number
  time: string
  type: string
  target: string
  operator: string
  result: '成功' | '失败'
  summary: string
}

const columns = [
  { title: '时间', dataIndex: 'time', key: 'time' },
  { title: '操作类型', dataIndex: 'type', key: 'type' },
  { title: '操作对象', dataIndex: 'target', key: 'target' },
  { title: '操作人', dataIndex: 'operator', key: 'operator' },
  { title: '结果', dataIndex: 'result', key: 'result' },
  { title: '摘要', dataIndex: 'summary', key: 'summary' },
  { title: '操作', key: 'action' }
]

const rows = ref<AuditRow[]>([
  { id: 1, time: '2026-03-15 11:20:11', type: '修改映射', target: '支付回调', operator: 'ops.zhang', result: '成功', summary: '调整响应超时为 8s。' },
  { id: 2, time: '2026-03-15 10:42:57', type: '批量停用', target: '账单类映射组', operator: 'ops.li', result: '成功', summary: '停用 3 条故障映射。' },
  { id: 3, time: '2026-03-15 10:18:32', type: '导入配置', target: '节点与隧道', operator: 'ops.wang', result: '失败', summary: '签名校验不通过。' }
])

const detailOpen = ref(false)
const current = ref<AuditRow>()
const openDetail = (row: AuditRow) => {
  current.value = row
  detailOpen.value = true
}
</script>
