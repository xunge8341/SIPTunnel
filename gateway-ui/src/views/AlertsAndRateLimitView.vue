<template>
  <a-space direction="vertical" style="width: 100%" size="large">
    <a-card :bordered="false">
      <a-page-header title="告警与保护" sub-title="每类规则同时展示配置与当前命中状态，避免只看配置不看实况。" />
    </a-card>

    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xl="8" v-for="panel in panels" :key="panel.title">
        <a-card :title="panel.title" :bordered="false">
          <a-descriptions :column="1" size="small" bordered>
            <a-descriptions-item label="当前配置">{{ panel.config }}</a-descriptions-item>
            <a-descriptions-item label="当前是否命中">
              <a-tag :color="panel.hitting ? 'red' : 'green'">{{ panel.hitting ? '命中中' : '未命中' }}</a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="最近命中时间">{{ panel.lastHitAt }}</a-descriptions-item>
            <a-descriptions-item label="最近命中对象">{{ panel.lastTarget }}</a-descriptions-item>
            <a-descriptions-item label="当前保护状态">
              <a-badge :status="panel.protecting ? 'processing' : 'default'" :text="panel.protecting ? '保护生效中' : '待机'" />
            </a-descriptions-item>
          </a-descriptions>
          <a-divider />
          <a-form layout="vertical">
            <a-form-item label="阈值" extra="字段说明放在字段下方，保存后立即应用。">
              <a-input :value="panel.threshold" />
            </a-form-item>
            <a-space>
              <a-button>刷新回读</a-button>
              <a-button type="primary">保存并应用</a-button>
            </a-space>
          </a-form>
        </a-card>
      </a-col>
    </a-row>
  </a-space>
</template>

<script setup lang="ts">
const panels = [
  {
    title: '告警规则',
    config: '失败率 > 5% 且持续 3 分钟触发告警。',
    hitting: true,
    lastHitAt: '2026-03-15 11:34:20',
    lastTarget: '支付回调',
    protecting: false,
    threshold: '失败率 5% / 持续 3 分钟'
  },
  {
    title: '限流规则',
    config: '单来源 IP 每分钟 300 请求。',
    hitting: true,
    lastHitAt: '2026-03-15 11:36:01',
    lastTarget: '10.2.8.9',
    protecting: true,
    threshold: '300 req/min'
  },
  {
    title: '熔断规则',
    config: '连续 10 次失败触发熔断，30 秒后半开。',
    hitting: false,
    lastHitAt: '2026-03-15 10:42:10',
    lastTarget: '账单归档',
    protecting: true,
    threshold: '连续失败 10 次 / 恢复 30 秒'
  }
]
</script>
