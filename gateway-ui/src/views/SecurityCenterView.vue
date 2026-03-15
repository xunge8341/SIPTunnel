<template>
  <a-space direction="vertical" style="width: 100%" size="large">
    <a-card :bordered="false">
      <a-page-header title="授权与安全" sub-title="聚焦 License 与管理面安全，不承载节点通信加密主配置。" />
      <a-row :gutter="[12, 12]">
        <a-col v-for="item in summary" :key="item.title" :xs="24" :md="12" :xl="6">
          <a-card size="small">
            <a-statistic :title="item.title" :value="item.value" />
            <a-typography-text type="secondary">{{ item.hint }}</a-typography-text>
          </a-card>
        </a-col>
      </a-row>
    </a-card>

    <a-row :gutter="[16, 16]">
      <a-col :xs="24" :xl="12">
        <a-card title="A. 授权管理" :bordered="false">
          <a-form layout="vertical">
            <a-form-item label="导入授权" extra="支持上传授权文件并自动校验签名。">
              <a-upload><a-button>选择授权文件</a-button></a-upload>
            </a-form-item>
            <a-form-item label="更新授权" extra="到期前建议提前 7 天更新。">
              <a-space>
                <a-button>在线更新</a-button>
                <a-button type="primary">离线导入更新</a-button>
              </a-space>
            </a-form-item>
            <a-alert type="info" show-icon message="授权说明" description="授权仅影响功能可用性，不负责节点通信加密。节点 AES/SM4 请在“节点与隧道”页面维护。" />
          </a-form>
        </a-card>
      </a-col>

      <a-col :xs="24" :xl="12">
        <a-card title="B. 平台与管理面安全" :bordered="false">
          <a-form layout="vertical">
            <a-form-item label="管理访问控制" extra="限制来源网段与账号权限分级。">
              <a-input value="10.2.0.0/16，172.16.8.0/24" />
            </a-form-item>
            <a-form-item label="校验周期" extra="定期核验授权与策略生效情况。">
              <a-select :options="[{label:'每日',value:'day'},{label:'每周',value:'week'}]" value="day" />
            </a-form-item>
            <a-form-item label="安全策略摘要" extra="统一展示管理面 MFA、Token、会话策略。">
              <a-textarea :rows="3" value="MFA 已启用；Token 12 小时轮换；高危动作二次确认。" />
            </a-form-item>
            <a-form-item label="生效状态" extra="保存后会立即校验并提示结果。">
              <a-badge status="success" text="已生效" />
            </a-form-item>
            <a-button type="primary">保存并应用</a-button>
          </a-form>
        </a-card>
      </a-col>
    </a-row>
  </a-space>
</template>

<script setup lang="ts">
const summary = [
  { title: '授权状态', value: '有效', hint: '授权校验通过。' },
  { title: '到期时间', value: '2027-01-31', hint: '距离到期 322 天。' },
  { title: '已授权功能', value: '企业版全功能', hint: '包含审计、压测与保护模块。' },
  { title: '最近校验结果', value: '11:30 校验成功', hint: '签名与特性码一致。' }
]
</script>
