<template>
  <component :is="contextHolder" />
</template>

<script setup lang="ts">
import { watchEffect } from 'vue'
import { message } from 'ant-design-vue'
import { useUiStore } from '../stores/ui'

const [messageApi, contextHolder] = message.useMessage()
const uiStore = useUiStore()

watchEffect(() => {
  uiStore.registerMessageApi(messageApi)
})
</script>
