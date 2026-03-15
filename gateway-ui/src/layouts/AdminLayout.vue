<template>
  <a-app>
    <a-layout class="layout-shell">
      <a-layout-sider v-model:collapsed="appStore.collapsed" collapsible :trigger="null" :width="220" :collapsed-width="80">
        <div class="logo">
          <span class="logo-icon" aria-hidden="true">◉</span>
          <span class="logo-text">隧道网关</span>
        </div>
        <a-menu
          theme="dark"
          mode="inline"
          :selected-keys="[activeMenuKey]"
          :items="menuItems"
          @click="handleMenuClick"
        />
      </a-layout-sider>

      <a-layout>
        <a-layout-header class="layout-header">
          <a-space>
            <a-button type="text" @click="appStore.toggleSidebar()">
              {{ appStore.collapsed ? '展开' : '收起' }}
            </a-button>
            <a-typography-title :level="5" style="margin: 0">{{ currentTitle }}</a-typography-title>
          </a-space>
          <a-tag color="blue">Lightweight Tunnel Gateway</a-tag>
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
const currentTitle = computed(() => (route.meta.title as string) ?? '首页')

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

.logo {
  color: #fff;
  height: 48px;
  margin: 16px;
  font-size: 17px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  background: rgba(255, 255, 255, 0.15);
  border-radius: 6px;
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
}

.logo-text {
  line-height: 1;
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
  padding: 16px;
  background: #fff;
}
</style>

<style scoped>
:deep(.ant-menu-title-content){white-space:nowrap;overflow:hidden;text-overflow:ellipsis;}
</style>
