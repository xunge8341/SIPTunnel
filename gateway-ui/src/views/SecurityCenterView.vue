<template>
  <a-space direction="vertical" style="width: 100%">
    <a-card title="授权信息">
      <a-descriptions :column="2" bordered>
        <a-descriptions-item label="授权状态">{{ license.status }}</a-descriptions-item>
        <a-descriptions-item label="到期时间">{{ license.expire_at }}</a-descriptions-item>
        <a-descriptions-item label="最近校验">{{ license.last_verify_result }}</a-descriptions-item>
        <a-descriptions-item label="已授权功能">{{ (license.features || []).join('、') }}</a-descriptions-item>
      </a-descriptions>
      <a-space style="margin-top: 12px">
        <a-input v-model:value="licenseToken" placeholder="粘贴授权串" style="width: 360px" />
        <a-button type="primary" @click="saveLicense">导入/更新授权</a-button>
      </a-space>
    </a-card>

    <a-card title="加密与安全策略">
      <a-form layout="vertical">
        <a-form-item label="签名算法">
          <a-select v-model:value="settings.signer" style="width: 200px">
            <a-select-option value="HMAC-SHA256">HMAC-SHA256</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="数据加密算法">
          <a-radio-group v-model:value="settings.encryption">
            <a-radio value="AES">AES</a-radio>
            <a-radio value="SM4">SM4</a-radio>
          </a-radio-group>
        </a-form-item>
        <a-form-item label="自动校验周期（分钟）">
          <a-input-number v-model:value="settings.verify_interval_min" :min="1" />
        </a-form-item>
        <a-button type="primary" @click="saveSettings">保存安全配置</a-button>
      </a-form>
    </a-card>
  </a-space>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { message } from 'ant-design-vue'
import { gatewayApi } from '../api/gateway'

const license = reactive({ status: '-', expire_at: '-', last_verify_result: '-', features: [] as string[] })
const settings = reactive({ signer: 'HMAC-SHA256', encryption: 'AES', verify_interval_min: 30 })
const licenseToken = ref('')

const load = async () => {
  Object.assign(license, await gatewayApi.fetchLicense())
  Object.assign(settings, await gatewayApi.fetchSecuritySettings())
}

const saveLicense = async () => {
  await gatewayApi.updateLicense({ token: licenseToken.value })
  message.success('授权已更新')
  await load()
}

const saveSettings = async () => {
  await gatewayApi.updateSecuritySettings(settings)
  message.success('安全配置已保存')
}

onMounted(load)
</script>
