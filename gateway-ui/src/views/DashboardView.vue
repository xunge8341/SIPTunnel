<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-card :bordered="false" class="page-hero">
      <a-page-header title="总览监控" sub-title="先看结论，再下钻排障；首页聚焦健康、风险与待处理事项。">
        <template #extra>
          <a-space>
            <a-button>导出诊断</a-button>
            <a-button type="primary">全部测试</a-button>
          </a-space>
        </template>
      </a-page-header>
    </a-card>

    <a-row :gutter="[16, 16]">
      <a-col v-for="item in summaryCards" :key="item.title" :xs="24" :sm="12" :xl="8">
        <a-card size="small" :bordered="false">
          <a-statistic :title="item.title" :value="item.value" :suffix="item.suffix" />
          <a-typography-text type="secondary">{{ item.hint }}</a-typography-text>
        </a-card>
      </a-col>
    </a-row>

    <a-card title="热点与异常 TopN" :bordered="false">
      <a-row :gutter="[16, 16]">
        <a-col v-for="item in topPanels" :key="item.title" :xs="24" :xl="12">
          <a-card size="small" :title="item.title">
            <a-list size="small" :data-source="item.data">
              <template #renderItem="{ item: row }">
                <a-list-item>
                  <a-space style="width: 100%; justify-content: space-between">
                    <span>{{ row.name }}</span>
                    <a-tag :color="row.color">{{ row.value }}</a-tag>
                  </a-space>
                </a-list-item>
              </template>
            </a-list>
          </a-card>
        </a-col>
      </a-row>
    </a-card>

    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xl="14">
        <a-card title="资源与保护状态" :bordered="false">
          <a-row :gutter="[12, 12]">
            <a-col v-for="resource in resourceCards" :key="resource.title" :span="12">
              <a-card size="small">
                <a-statistic :title="resource.title" :value="resource.value" :suffix="resource.suffix" />
                <a-progress :percent="resource.percent" :status="resource.status" size="small" style="margin-top: 8px" />
              </a-card>
            </a-col>
          </a-row>
          <a-alert
            style="margin-top: 12px"
            show-icon
            type="warning"
            message="最近保护动作"
            description="10:42 对映射“支付回调”触发熔断；10:47 自动半开恢复；11:03 来源 IP 10.2.8.9 命中限流。"
          />
        </a-card>
      </a-col>
      <a-col :xs="24" :xl="10">
        <a-card title="待处理事项与快捷操作" :bordered="false">
          <a-list size="small" :data-source="todoItems">
            <template #renderItem="{ item }">
              <a-list-item>
                <a-space style="width: 100%; justify-content: space-between">
                  <span>{{ item.title }}</span>
                  <a-tag :color="item.color">{{ item.level }}</a-tag>
                </a-space>
              </a-list-item>
            </template>
          </a-list>
          <a-divider />
          <a-space wrap>
            <a-button type="primary">全部测试</a-button>
            <a-button>查看失败日志</a-button>
            <a-button>导出诊断</a-button>
            <a-button>进入系统设置</a-button>
          </a-space>
        </a-card>
      </a-col>
    </a-row>
  </a-space>
</template>

<script setup lang="ts">
type SummaryCard = { title: string; value: string | number; hint: string; suffix?: string }

const summaryCards: SummaryCard[] = [
  { title: '系统健康状态', value: '良好', hint: '核心链路可用，未发现阻塞性故障。' },
  { title: '当前连接数', value: 186, hint: '较昨日同时间段 +8.1%。' },
  { title: '映射总数 / 异常数', value: '42 / 3', hint: '异常集中在支付与报表映射。' },
  { title: '近 15 分钟失败请求数', value: 27, hint: '较上一时段下降 12%。' },
  { title: '当前限流状态', value: '已启用', hint: '3 条规则生效，2 条有命中。' },
  { title: '当前熔断状态', value: '半开恢复', hint: '1 条熔断策略处于观察阶段。' }
]

const topPanels = [
  {
    title: '热点映射 TopN',
    data: [
      { name: '订单同步', value: '1.2k 次', color: 'blue' },
      { name: '支付回调', value: '930 次', color: 'blue' },
      { name: '库存查询', value: '610 次', color: 'blue' }
    ]
  },
  {
    title: '失败最多映射 TopN',
    data: [
      { name: '支付回调', value: '19 次失败', color: 'red' },
      { name: '账单归档', value: '11 次失败', color: 'red' },
      { name: '短信发送', value: '7 次失败', color: 'red' }
    ]
  },
  {
    title: '热点来源 IP TopN',
    data: [
      { name: '10.2.8.21', value: '420 请求', color: 'cyan' },
      { name: '10.2.8.18', value: '368 请求', color: 'cyan' },
      { name: '10.2.8.35', value: '265 请求', color: 'cyan' }
    ]
  },
  {
    title: '失败最多来源 IP TopN',
    data: [
      { name: '10.2.8.9', value: '14 失败', color: 'orange' },
      { name: '10.2.8.44', value: '6 失败', color: 'orange' },
      { name: '10.2.8.56', value: '4 失败', color: 'orange' }
    ]
  }
]

const resourceCards = [
  { title: 'CPU', value: 52, suffix: '%', percent: 52, status: 'active' as const },
  { title: '内存', value: 68, suffix: '%', percent: 68, status: 'active' as const },
  { title: '网络', value: 41, suffix: '%', percent: 41, status: 'normal' as const },
  { title: 'RTP 端口池', value: 74, suffix: '%', percent: 74, status: 'exception' as const }
]

const todoItems = [
  { title: '最近告警：支付回调 5 分钟失败率超过阈值', level: '高优先级', color: 'red' },
  { title: '最近测试失败映射：账单归档（目标响应超时）', level: '需处理', color: 'orange' },
  { title: '最近诊断失败：对端节点 B 心跳抖动', level: '关注', color: 'gold' }
]
</script>

<style scoped>
.page-hero :deep(.ant-page-header) {
  padding: 0;
}
</style>
