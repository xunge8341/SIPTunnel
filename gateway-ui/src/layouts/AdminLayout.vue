<template>
  <a-app>
    <a-layout class="layout-shell">
      <a-layout-sider v-model:collapsed="appStore.collapsed" collapsible :trigger="null" :width="240" :collapsed-width="80" class="layout-sider">
        <div class="logo" :class="{ collapsed: appStore.collapsed }">
          <span class="logo-icon" aria-hidden="true">◉</span>
          <span class="logo-text">SIPTunnel 控制台</span>
        </div>
        <a-menu theme="dark" mode="inline" :selected-keys="[activeMenuKey]" :items="menuItems" @click="handleMenuClick" />
      </a-layout-sider>

      <a-layout>
        <a-layout-header class="layout-header">
          <a-space>
            <a-button type="text" @click="appStore.toggleSidebar()">{{ appStore.collapsed ? '展开菜单' : '收起菜单' }}</a-button>
            <a-breadcrumb>
              <a-breadcrumb-item>企业运维控制台</a-breadcrumb-item>
              <a-breadcrumb-item>{{ currentTitle }}</a-breadcrumb-item>
            </a-breadcrumb>
          </a-space>
          <a-space>
            <a-tag color="blue">统一运维范式</a-tag>
            <a-tag color="green">中文控制台</a-tag>
          </a-space>
        </a-layout-header>

        <a-layout-content class="layout-content">
          <router-view />
        </a-layout-content>
      </a-layout>
    </a-layout>
    <global-message-host />
  </a-app>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '../stores/app'
import GlobalMessageHost from '../components/GlobalMessageHost.vue'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const activeMenuKey = computed(() => route.name?.toString() ?? 'dashboard')
const currentTitle = computed(() => (route.meta.title as string) ?? '总览监控')

const menuItems = computed(() =>
  appStore.navigation.map((item) => ({
    key: item.key,
    label: item.label,
    title: item.label
  }))
)

const handleMenuClick = ({ key }: { key: string }) => {
  const target = appStore.navigation.find((item) => item.key === key)
  if (target) {
    router.push(target.path)
  }
}
</script>

<style scoped>
.layout-shell {
  min-height: 100vh;
}

.layout-sider {
  box-shadow: 2px 0 8px rgba(0, 0, 0, 0.2);
}

.logo {
  color: #fff;
  height: 52px;
  margin: 14px 12px;
  font-size: 16px;
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.15);
  overflow: hidden;
  white-space: nowrap;
}

.logo.collapsed {
  justify-content: center;
  padding: 0;
}

.logo-icon {
  width: 18px;
  height: 18px;
  border-radius: 4px;
  background: linear-gradient(135deg, #5ea0ff 0%, #8cc8ff 100%);
  color: #0a2a5e;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  flex: 0 0 auto;
}

.logo-text {
  overflow: hidden;
  text-overflow: ellipsis;
}

.layout-header {
  background: #fff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid #f0f0f0;
  padding: 0 20px;
}

.layout-content {
  margin: 16px;
}

:deep(.ant-menu-item) {
  margin-inline: 8px;
  width: calc(100% - 16px);
}

:deep(.ant-menu-title-content) {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
