<template>
  <a-app>
    <a-layout class="layout-shell">
      <a-layout-sider v-model:collapsed="appStore.collapsed" collapsible :trigger="null">
        <div class="logo">SIPTunnel</div>
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
          <a-tag color="blue">后台管理界面</a-tag>
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
const currentTitle = computed(() => (route.meta.title as string) ?? 'Dashboard')

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
  font-size: 18px;
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.15);
  border-radius: 6px;
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
