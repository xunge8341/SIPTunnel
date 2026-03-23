<template>
  <a-space direction="vertical" size="large" style="width: 100%">
    <a-page-header title="授权管理" sub-title="聚焦授权状态、机器码导出与授权导入；安全事件请在“安全事件”页面查看，本端管理面安全和节点通信加密请在“节点与级联”中维护。" />

    <a-alert v-if="notice" type="success" :message="notice" show-icon />
    <a-alert v-if="error" type="error" :message="error" show-icon />

    <a-spin :spinning="loading || saving">
      <a-row :gutter="[16, 16]">
        <a-col :xs="24" :xl="10">
          <a-card title="授权摘要" :bordered="false">
            <a-empty v-if="!state" description="暂无授权状态" />
            <a-descriptions v-else :column="1" size="small">
              <a-descriptions-item label="授权状态">{{ state.licenseStatus }}</a-descriptions-item>
              <a-descriptions-item label="产品类型">{{ state.productTypeName ? `${state.productTypeName}（${state.productType}）` : state.productType }}</a-descriptions-item>
              <a-descriptions-item label="授权类型">{{ state.licenseType }}</a-descriptions-item>
              <a-descriptions-item label="授权次数">{{ state.licenseCounter }}</a-descriptions-item>
              <a-descriptions-item label="授权日期">{{ formatDateTimeText(state.licenseTime) }}</a-descriptions-item>
              <a-descriptions-item label="启用日期">{{ formatDateTimeText(state.activeTime) }}</a-descriptions-item>
              <a-descriptions-item label="到期时间">{{ formatDateTimeText(state.expiryTime) }}</a-descriptions-item>
              <a-descriptions-item label="维保到期">{{ formatDateTimeText(state.maintenanceExpireTime) }}</a-descriptions-item>
              <a-descriptions-item label="项目编码">
                <a-typography-paragraph class="mono-inline-code wrap-break-anywhere" copyable style="margin-bottom: 0">{{ state.projectCode }}</a-typography-paragraph>
              </a-descriptions-item>
              <a-descriptions-item label="机器码">
                <a-typography-paragraph class="mono-block-code wrap-break-anywhere" copyable style="margin-bottom: 0">{{ state.machineCode }}</a-typography-paragraph>
              </a-descriptions-item>
              <a-descriptions-item label="已授权功能">{{ state.licensedFeatures.join('、') || '无' }}</a-descriptions-item>
              <a-descriptions-item label="最近校验结果">{{ state.lastValidation }}</a-descriptions-item>
            </a-descriptions>
            <a-divider />
            <a-descriptions v-if="state" :column="1" size="small" title="管理面加固状态">
              <a-descriptions-item label="管理令牌"><a-tag :color="state.adminTokenConfigured ? 'green' : 'orange'">{{ adminTokenStatusText }}</a-tag></a-descriptions-item>
              <a-descriptions-item label="管理面 MFA"><a-tag :color="state.adminMFARequired ? (state.adminMFAConfigured ? 'green' : 'red') : 'default'">{{ state.adminMFARequired ? (state.adminMFAConfigured ? '已要求且已配置' : '已要求但未配置') : '未要求' }}</a-tag></a-descriptions-item>
              <a-descriptions-item label="配置落盘加密"><a-tag :color="state.configEncryption ? 'green' : 'orange'">{{ state.configEncryption ? '已启用' : '未启用' }}</a-tag></a-descriptions-item>
              <a-descriptions-item label="隧道签名密钥"><a-tag :color="state.signerExternalized ? 'green' : 'orange'">{{ state.signerExternalized ? '已外置' : '仍为默认值' }}</a-tag></a-descriptions-item>
            </a-descriptions>
          </a-card>
        </a-col>

        <a-col :xs="24" :xl="14">
          <a-card title="机器码与授权导入" :bordered="false">
            <a-form layout="vertical">
              <a-alert type="info" show-icon message="编码展示已改为自动换行与等宽字体，长机器码/项目编码不会再被截断。" style="margin-bottom: 16px" />
              <a-form-item label="机器码" extra="机器码由 CPUID、主板序列号、主网卡 MAC 组合生成；获取不到的项会使用缺省值。">
                <a-textarea :value="machineCodeText" :auto-size="{ minRows: 6, maxRows: 10 }" disabled class="mono-block-code" />
              </a-form-item>
              <a-space wrap style="margin-bottom: 16px">
                <a-button @click="loadMachineCode">刷新机器码</a-button>
                <a-button type="primary" ghost @click="copyMachineCode">复制机器码</a-button>
                <a-button type="primary" @click="exportRequestFile">导出申请文件</a-button>
              </a-space>
              <a-divider />
              <a-form-item label="授权文件内容" extra="请粘贴完整授权文件内容，系统将按约定顺序组装字段、计算 MD5，并使用 RSA 私钥解密 Summary1 进行校验。">
                <a-textarea v-model:value="licenseContent" :auto-size="{ minRows: 10, maxRows: 18 }" placeholder="请粘贴完整授权文件内容" />
              </a-form-item>
              <a-button type="primary" :loading="saving" @click="saveLicense">导入并校验授权</a-button>
            </a-form>
          </a-card>
        </a-col>
      </a-row>
    </a-spin>
  </a-space>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'
import { formatDateTimeText } from '../utils/date'
import type { MachineCodePayload, SecurityCenterState } from '../types/gateway'

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const state = ref<SecurityCenterState>()
const machineCode = ref<MachineCodePayload>()
const licenseContent = ref('')

const adminTokenStatusText = computed(() => {
  if (!state.value?.adminTokenConfigured) return '未启用'
  return `已启用（指纹 ${state.value.adminTokenFingerprint || '-' }）`
})

const machineCodeText = computed(() => {
  if (!machineCode.value) return '暂无机器码'
  return [
    `本端编码：${machineCode.value.node_id}`,
    `主机名：${machineCode.value.hostname}`,
    `CPUID：${machineCode.value.cpu_id}`,
    `主板序列号：${machineCode.value.board_serial}`,
    `网卡 MAC：${machineCode.value.mac_address}`,
    `机器码：${machineCode.value.machine_code}`
  ].join('\n')
})

const loadMachineCode = async () => {
  machineCode.value = await gatewayApi.fetchMachineCode()
}

const load = async () => {
  loading.value = true
  error.value = ''
  notice.value = ''
  try {
    const [stateResp, machineResp] = await Promise.all([
      gatewayApi.fetchSecurityState(),
      gatewayApi.fetchMachineCode()
    ])
    state.value = stateResp
    machineCode.value = machineResp
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载授权信息失败'
  } finally {
    loading.value = false
  }
}

const copyMachineCode = async () => {
  if (!machineCode.value) return
  await navigator.clipboard.writeText(machineCode.value.machine_code)
  message.success('机器码已复制')
}

const exportRequestFile = () => {
  if (!machineCode.value?.request_file) return
  const blob = new Blob([machineCode.value.request_file], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `license-request-${machineCode.value.machine_code}.licreq`
  anchor.click()
  URL.revokeObjectURL(url)
  message.success('申请文件已导出')
}

const saveLicense = async () => {
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    await gatewayApi.updateLicense({ content: licenseContent.value })
    await load()
    licenseContent.value = ''
    notice.value = '授权文件已导入并完成校验。'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '更新授权失败'
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
