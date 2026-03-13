import { describe, expect, it } from 'vitest'
import {
  createDiagnosticExportMock,
  exportConfigYamlMock,
  fetchConfigGovernanceMock,
  getDiagnosticExportMock,
  retryDiagnosticExportMock,
  rollbackConfigMock,
  fetchDashboardMock,
  fetchNetworkConfigMock
} from '../mockGateway'

describe('config governance mock api', () => {
  it('supports operator and version filtering', async () => {
    const result = await fetchConfigGovernanceMock({ operator: 'ops_admin', version: 'v2026.03.12' })
    expect(result.snapshots.length).toBeGreaterThan(0)
    expect(result.snapshots.every((item) => item.operator.includes('ops_admin'))).toBe(true)
  })

  it('exports yaml for current and pending target', async () => {
    const current = await exportConfigYamlMock('current')
    const pending = await exportConfigYamlMock('pending')
    expect(current).toContain('sip:')
    expect(pending).toContain('max_message_bytes:')
  })

  it('marks rollback target as active', async () => {
    const result = await rollbackConfigMock('v2026.03.12.2')
    const target = result.snapshots.find((item) => item.version === 'v2026.03.12.2')
    expect(target?.status).toBe('active')
  })



  it('provides enhanced dashboard metrics for ops', async () => {
    const result = await fetchDashboardMock()
    expect(result.metrics.currentConnections).toBeGreaterThan(0)
    expect(result.metrics.failedTasks1h).toBeGreaterThanOrEqual(0)
    expect(result.metrics.transportErrors1h).toBeGreaterThanOrEqual(0)
    expect(result.metrics.rateLimitHits1h).toBeGreaterThanOrEqual(0)
  })

  it('provides network status extensions and link test data', async () => {
    const result = await fetchNetworkConfigMock()
    expect(result.portPool.usageRate).toBeGreaterThan(0)
    expect(result.connectionErrors.length).toBeGreaterThan(0)
    expect(result.selfCheckItems.length).toBeGreaterThan(0)
    expect(result.linkTests.length).toBeGreaterThan(0)
  })

  it('supports diagnostic export fail then retry success flow', async () => {
    const created = await createDiagnosticExportMock({ nodeId: 'gateway-a-02' })
    let status = created
    for (let i = 0; i < 4; i += 1) {
      status = await getDiagnosticExportMock(created.jobId)
    }
    expect(status.status).toBe('failed')
    expect(status.errorMessage).toContain('导出包生成失败')

    await retryDiagnosticExportMock(created.jobId)

    for (let i = 0; i < 4; i += 1) {
      status = await getDiagnosticExportMock(created.jobId)
    }
    expect(status.status).toBe('succeeded')
    expect(status.downloadUrl).toContain('data:application/zip;base64')
  })
})
