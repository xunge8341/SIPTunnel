import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

const normalizeId = (id: string) => id.replace(/\\/g, '/')

const featureChunkName = (rawId: string) => {
  const id = normalizeId(rawId)
  if (!id.includes('/src/')) {
    return undefined
  }
  if (id.includes('/src/api/mockGateway')) {
    return 'mock-gateway'
  }
  if (id.includes('/src/views/')) {
    if (/DashboardView|AccessLogsView|OpsAuditsView|LinkMonitorView/.test(id)) {
      return 'feature-observability'
    }
    if (/AlertsAndRateLimitView/.test(id)) {
      return 'feature-protection'
    }
    if (/SecurityCenterView|SecurityEventsView/.test(id)) {
      return 'feature-security'
    }
    if (/TunnelMappingsView|LocalResourcesView|NodesAndTunnelsView/.test(id)) {
      return 'feature-tunnel-ops'
    }
    if (/SystemSettingsView/.test(id)) {
      return 'feature-system-settings'
    }
    if (/DiagnosticsLoadtestView/.test(id)) {
      return 'feature-admin-tools'
    }
  }
  if (id.includes('/src/api/')) {
    return 'gateway-api'
  }
  return undefined
}

const vendorChunkName = (rawId: string) => {
  const id = normalizeId(rawId)
  if (!id.includes('node_modules')) {
    return undefined
  }
  if (id.includes('/@babel/runtime/')) {
    return 'vendor-babel-runtime'
  }
  if (
    id.includes('/dayjs/') ||
    id.includes('/@ctrl/tinycolor/') ||
    id.includes('/compute-scroll-into-view/') ||
    id.includes('/scroll-into-view-if-needed/') ||
    id.includes('/resize-observer-polyfill/') ||
    id.includes('/throttle-debounce/') ||
    id.includes('/async-validator/')
  ) {
    return 'vendor-ui-utils'
  }
  if (id.includes('/@rc-component/') || id.includes('/rc-') || id.includes('ant-design-vue')) {
    return 'vendor-antd'
  }
  if (id.includes('@ant-design/icons-vue')) {
    return 'vendor-antd-icons'
  }
  if (
    id.includes('/vue/') ||
    id.includes('/vue-router/') ||
    id.includes('/pinia/')
  ) {
    return 'vendor-vue'
  }
  return 'vendor'
}

export default defineConfig({
  base: './',
  plugins: [vue()],
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          return vendorChunkName(id) ?? featureChunkName(id)
        }
      }
    }
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    globals: true
  }
})
