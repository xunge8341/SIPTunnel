<template>
  <a-space direction="vertical" style="width: 100%" size="large">
    <a-card :bordered="false">
      <a-page-header title="诊断与压测" sub-title="左侧做诊断，右侧做压测；页面内可直接完成动作。" />
    </a-card>

    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xl="12">
        <a-card title="诊断" :bordered="false">
          <a-space direction="vertical" style="width: 100%">
            <a-space wrap>
              <a-button type="primary">节点连通性测试</a-button>
              <a-button>单条映射测试</a-button>
              <a-button>全部映射测试</a-button>
              <a-button>诊断包导出</a-button>
            </a-space>
            <a-list bordered size="small" header="最近诊断记录" :data-source="diagnostics">
              <template #renderItem="{ item }">
                <a-list-item>
                  <a-space style="width: 100%; justify-content: space-between">
                    <span>{{ item.time }} - {{ item.name }}</span>
                    <a-tag :color="item.success ? 'green' : 'red'">{{ item.success ? '通过' : '失败' }}</a-tag>
                  </a-space>
                </a-list-item>
              </template>
            </a-list>
          </a-space>
        </a-card>
      </a-col>

      <a-col :xs="24" :xl="12">
        <a-card title="压测" :bordered="false">
          <a-form layout="vertical">
            <a-form-item label="发起压测" extra="输入并发和持续时间后可直接执行。">
              <a-space>
                <a-input placeholder="并发数，例如 200" style="width: 180px" />
                <a-input placeholder="持续时间，例如 10m" style="width: 180px" />
                <a-button type="primary">开始压测</a-button>
              </a-space>
            </a-form-item>
          </a-form>
          <a-descriptions bordered :column="1" size="small" title="压测结果摘要">
            <a-descriptions-item label="压测历史">最近 3 次：2 次通过、1 次容量告警。</a-descriptions-item>
            <a-descriptions-item label="容量建议">建议并发上限 260；超过后失败率显著上升。</a-descriptions-item>
            <a-descriptions-item label="联动建议">并发 > 220 时开启更严格限流，并缩短熔断恢复窗口。</a-descriptions-item>
          </a-descriptions>
        </a-card>
      </a-col>
    </a-row>
  </a-space>
</template>

<script setup lang="ts">
const diagnostics = [
  { time: '11:40', name: '节点 A -> 节点 B 连通性', success: true },
  { time: '11:35', name: '支付回调映射测试', success: false },
  { time: '11:20', name: '全部映射批量测试', success: true }
]
</script>
