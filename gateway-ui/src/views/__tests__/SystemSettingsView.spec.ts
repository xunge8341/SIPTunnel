import { mount, flushPromises } from '@vue/test-utils'
import SystemSettingsView from '../SystemSettingsView.vue'
import { gatewayApi } from '../../api/gateway'
import type { SystemResourceUsage, SystemSettingsState } from '../../types/gateway'

vi.mock('../../api/gateway', () => ({ gatewayApi: { fetchSystemSettings: vi.fn(), updateSystemSettings: vi.fn(), fetchSystemResourceUsage: vi.fn() } }))

describe('SystemSettingsView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  const state: SystemSettingsState = {
    sqlitePath: 'a.db',
    logPath: '/log',
    uiMode: 'embedded',
    apiBaseUrl: 'http://127.0.0.1:18080/api',
    metricsEndpoint: 'http://127.0.0.1:18080/metrics',
    readyEndpoint: 'http://127.0.0.1:18080/readyz',
    selfCheckEndpoint: 'http://127.0.0.1:18080/api/selfcheck',
    startupSummaryEndpoint: 'http://127.0.0.1:18080/api/startup-summary',
    logRetentionDays: 1,
    logRetentionRecords: 100,
    auditRetentionDays: 1,
    auditRetentionRecords: 100,
    accessLogRetentionDays: 1,
    accessLogRetentionRecords: 100,
    diagnosticsRetentionDays: 1,
    diagnosticsRetentionRecords: 10,
    loadtestRetentionDays: 1,
    loadtestRetentionRecords: 10,
    cleanupCron: '* * * * *',
    adminCIDR: '127.0.0.1/32',
    mfaEnabled: false,
    genericDownloadTotalMbps: 24,
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
    lastCleanupStatus: 'ok',
    lastCleanupRemovedRecords: 0
  }

  const usage: SystemResourceUsage = {
    captured_at: '2026-03-22T12:00:00Z',
    cpu_cores: 8,
    gomaxprocs: 8,
    goroutines: 42,
    heap_alloc_bytes: 32 * 1024 * 1024,
    heap_sys_bytes: 64 * 1024 * 1024,
    heap_idle_bytes: 16 * 1024 * 1024,
    stack_inuse_bytes: 2 * 1024 * 1024,
    last_gc_time: '2026-03-22T11:59:00Z',
    sip_connections: 2,
    rtp_active_transfers: 3,
    rtp_port_pool_used: 4,
    rtp_port_pool_total: 20,
    active_requests: 5,
    configured_generic_download_mbps: 24,
    configured_generic_per_transfer_mbps: 8,
    configured_adaptive_hot_cache_mb: 32,
    configured_adaptive_hot_window_mb: 16,
    configured_generic_download_window_mb: 2,
    configured_generic_segment_concurrency: 2,
    configured_generic_rtp_reorder_window_packets: 512,
    configured_generic_rtp_loss_tolerance_packets: 128,
    configured_generic_rtp_gap_timeout_ms: 1200,
    configured_generic_rtp_fec_enabled: true,
    configured_generic_rtp_fec_group_packets: 8,
  }

  it('loads settings', async () => {
    vi.mocked(gatewayApi.fetchSystemSettings).mockResolvedValue(state)
    vi.mocked(gatewayApi.fetchSystemResourceUsage).mockResolvedValue(usage)
    mount(SystemSettingsView)
    await flushPromises()
    expect(gatewayApi.fetchSystemSettings).toHaveBeenCalled()
  })

  it('saves and reloads settings', async () => {
    vi.mocked(gatewayApi.fetchSystemSettings).mockResolvedValue(state)
    vi.mocked(gatewayApi.fetchSystemResourceUsage).mockResolvedValue(usage)
    vi.mocked(gatewayApi.updateSystemSettings).mockResolvedValue(state)
    const wrapper = mount(SystemSettingsView)
    await flushPromises()

    const saveButton = wrapper.findAll('button').find((button) => button.text().includes('保存设置'))
    expect(saveButton).toBeTruthy()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(gatewayApi.updateSystemSettings).toHaveBeenCalledTimes(1)
    expect(gatewayApi.fetchSystemSettings).toHaveBeenCalledTimes(1)
  })
})
