import { describe, expect, it } from 'vitest'
import {
  createDiagnosticExportMock,
  exportConfigYamlMock,
  fetchConfigGovernanceMock,
  getDiagnosticExportMock,
  retryDiagnosticExportMock,
  rollbackConfigMock,
  fetchDashboardMock,
  fetchNetworkConfigMock,
  fetchDeploymentModeMock,
  fetchStartupSummaryMock,
  fetchMappingsMock,
  createMappingMock,
  updateMappingMock,
  deleteMappingMock,
  exportConfigJsonMock,
  importConfigJsonMock,
  downloadConfigTemplateMock
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

  it('provides deployment mode metadata for ui/api visibility', async () => {
    const result = await fetchDeploymentModeMock()
    expect(['embedded', 'external']).toContain(result.uiMode)
    expect(result.uiUrl).toContain('http')
    expect(result.apiUrl).toContain('http')
    expect(result.configPath).toContain('/')
    expect(result.configSource.length).toBeGreaterThan(0)
  })


  it('provides startup summary business execution status', async () => {
    const result = await fetchStartupSummaryMock()
    expect(result.business_execution.state).toBe('protocol_only')
    expect(result.business_execution.message).toContain('业务执行层未激活')
    expect(result.self_check_summary.overall).toBe('warn')
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


  it('supports config json export/import/template flow', async () => {
    const exported = await exportConfigJsonMock()
    expect(exported.version.length).toBeGreaterThan(0)
    expect(exported.network_config.sip.listenPort).toBeGreaterThan(0)

    const imported = await importConfigJsonMock(exported)
    expect(imported.imported).toBe(true)
    expect(imported.tunnel_restarted).toBe(true)

    const template = await downloadConfigTemplateMock()
    expect(template.version).toBe('template-v1')
    expect(template.node_config.local_node.device_id).toContain('template')
  })

  it('supports tunnel mapping CRUD in mock mode', async () => {
    const before = await fetchMappingsMock()
    const mapping = {
      mapping_id: 'map-test',
      name: '测试映射',
      enabled: true,
      peer_node_id: 'peer-x',
      local_bind_ip: '127.0.0.1',
      local_bind_port: 18082,
      local_base_path: '/local',
      remote_target_ip: '127.0.0.2',
      remote_target_port: 8082,
      remote_base_path: '/remote',
      allowed_methods: ['POST'],
      connect_timeout_ms: 500,
      request_timeout_ms: 3000,
      response_timeout_ms: 3000,
      max_request_body_bytes: 1024,
      max_response_body_bytes: 2048,
      require_streaming_response: false,
      description: 'desc'
    }
    await createMappingMock(mapping)
    let listed = await fetchMappingsMock()
    expect(listed.items.length).toBe(before.items.length + 1)

    await updateMappingMock('map-test', { ...mapping, name: '测试映射-更新' })
    listed = await fetchMappingsMock()
    expect(listed.items.find((item) => item.mapping_id === 'map-test')?.name).toBe('测试映射-更新')

    await deleteMappingMock('map-test')
    listed = await fetchMappingsMock()
    expect(listed.items.find((item) => item.mapping_id === 'map-test')).toBeUndefined()
  })

})
