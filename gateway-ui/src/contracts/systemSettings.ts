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
    uiMode: 'embedded',
    apiBaseUrl: 'http://127.0.0.1:18080/api',
    metricsEndpoint: 'http://127.0.0.1:18080/metrics',
    readyEndpoint: 'http://127.0.0.1:18080/readyz',
    selfCheckEndpoint: 'http://127.0.0.1:18080/api/selfcheck',
    startupSummaryEndpoint: 'http://127.0.0.1:18080/api/startup-summary',
    logRetentionDays: 7,
    logRetentionRecords: 10000,
    auditRetentionDays: 30,
    auditRetentionRecords: 10000,
    accessLogRetentionDays: 30,
    accessLogRetentionRecords: 10000,
    diagnosticsRetentionDays: 7,
    diagnosticsRetentionRecords: 200,
    loadtestRetentionDays: 7,
    loadtestRetentionRecords: 200,
    cleanupCron: '0 3 * * *',
    adminCIDR: '',
    mfaEnabled: false,
    genericDownloadTotalMbps: 16,
    genericDownloadPerTransferMbps: 8,
    genericDownloadWindowMB: 2,
    adaptiveHotCacheMB: 32,
    adaptiveHotWindowMB: 16,
    genericDownloadSegmentConcurrency: 2,
    genericDownloadRTPReorderWindowPackets: 512,
    genericDownloadRTPLossTolerancePackets: 128,
    genericDownloadRTPGapTimeoutMS: 1200,
    genericDownloadRTPFECEnabled: true,
    genericDownloadRTPFECGroupPackets: 8,
    lastCleanupStatus: '',
    lastCleanupRemovedRecords: 0
  }
}
