<template>
  <a-space direction="vertical" style="width: 100%" size="large">
    <a-card :bordered="false">
      <a-page-header title="系统设置" sub-title="全局治理页：每张卡片包含摘要、表单与可折叠高级项。" />
    </a-card>

    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xl="12" v-for="card in cards" :key="card.title">
        <a-card :title="card.title" :bordered="false">
          <a-alert type="info" show-icon :message="card.summary" style="margin-bottom: 12px" />
          <a-form layout="vertical">
            <a-form-item v-for="field in card.fields" :key="field.label" :label="field.label" :extra="field.extra">
              <a-input v-model:value="field.value" />
            </a-form-item>
            <a-collapse>
              <a-collapse-panel key="adv" header="高级设置">
                <a-form-item v-for="field in card.advanced" :key="field.label" :label="field.label" :extra="field.extra">
                  <a-input v-model:value="field.value" />
                </a-form-item>
              </a-collapse-panel>
            </a-collapse>
            <a-space style="margin-top: 12px">
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
import { ref } from 'vue'

type Field = { label: string; value: string; extra: string }

type SettingsCard = {
  title: string
  summary: string
  fields: Field[]
  advanced: Field[]
}

const cards = ref<SettingsCard[]>([
  {
    title: 'A. 持久化与数据库',
    summary: 'SQLite 当前可写，最近一次写入成功。',
    fields: [
      { label: 'SQLite 路径', value: '/var/lib/siptunnel/data.db', extra: '主数据文件路径。' },
      { label: '数据目录', value: '/var/lib/siptunnel', extra: '建议挂载高可靠磁盘。' },
      { label: '当前状态', value: '健康', extra: '读写均正常。' },
      { label: '最近写入状态', value: '11:35 写入成功', extra: '用于判断存储抖动。' }
    ],
    advanced: [{ label: '写入批次大小', value: '200', extra: '高并发场景可适当增大。' }]
  },
  {
    title: 'B. 日志与清理',
    summary: '日志轮转已开启，最近清理完成。',
    fields: [
      { label: '日志路径', value: '/var/log/siptunnel', extra: '建议独立磁盘分区。' },
      { label: '滚动规则', value: '按天 + 单文件 200MB', extra: '平衡排障与磁盘占用。' },
      { label: '保留天数', value: '30', extra: '建议不少于 14 天。' },
      { label: '保留文件数', value: '200', extra: '防止文件数过多影响检索。' },
      { label: '最近清理结果', value: '11:00 清理 1.2GB', extra: '本次无异常。' }
    ],
    advanced: [{ label: '压缩策略', value: '7 天前自动压缩', extra: '减少历史日志占用。' }]
  },
  {
    title: 'C. 保留策略',
    summary: '访问日志、审计与诊断数据已按策略清理。',
    fields: [
      { label: '访问日志保留', value: '30 天', extra: '用于排障与趋势分析。' },
      { label: '审计保留', value: '180 天', extra: '满足合规要求。' },
      { label: '诊断保留', value: '60 天', extra: '便于追溯稳定性问题。' },
      { label: '压测保留', value: '90 天', extra: '用于容量规划参考。' }
    ],
    advanced: [{ label: '冷存储归档', value: '启用', extra: '超过保留期后归档对象存储。' }]
  },
  {
    title: 'D. 管理面安全',
    summary: '管理访问仅允许白名单网段，MFA 已启用。',
    fields: [
      { label: '允许 CIDR', value: '10.2.0.0/16, 172.16.8.0/24', extra: '限制管理入口来源。' },
      { label: 'MFA', value: '已开启', extra: '建议与统一身份系统联动。' },
      { label: 'Token 策略', value: '12 小时轮换', extra: '降低长时间泄露风险。' }
    ],
    advanced: [{ label: '会话失效策略', value: '30 分钟无操作自动失效', extra: '提升管理面安全性。' }]
  },
  {
    title: 'E. 最近维护动作',
    summary: '维护动作留痕完整，可直接回溯执行人和结果。',
    fields: [
      { label: '最近清理', value: '2026-03-15 11:00 成功', extra: '日志与缓存清理。' },
      { label: '最近导入配置', value: '2026-03-14 19:20 成功', extra: '节点与映射配置。' },
      { label: '最近导出诊断', value: '2026-03-15 10:55 成功', extra: '已归档到运维盘。' },
      { label: '最近异常维护动作', value: '2026-03-15 09:30 导入失败', extra: '原因：签名校验未通过。' }
    ],
    advanced: [{ label: '维护审批策略', value: '高风险变更需双人审批', extra: '降低误操作风险。' }]
  }
])
</script>
