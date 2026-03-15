import type { SystemSettingsState } from '../types/gateway'

export interface ViewContract<TResponse> {
  feature: string
  apiPath: string
  handler: string
  dataSource: string
  response: TResponse
}

export const SYSTEM_SETTINGS_CONTRACT: ViewContract<SystemSettingsState> = {
  feature: '系统设置',
  apiPath: '/system/settings',
  handler: 'gatewayApi.fetchSystemSettings / gatewayApi.updateSystemSettings',
  dataSource: 'SQLite 持久化配置与系统清理策略',
  response: {
    sqlitePath: '',
    logPath: '',
    logRetentionDays: 7,
    auditRetentionDays: 30,
    accessLogRetentionDays: 30,
    diagnosticsRetentionDays: 7,
    loadtestRetentionDays: 7,
    cleanupCron: '0 3 * * *',
    adminCIDR: '',
    mfaEnabled: false,
    lastCleanupStatus: ''
  }
}
