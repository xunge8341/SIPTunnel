import { describe, expect, it } from 'vitest'
import { exportConfigYamlMock, fetchConfigGovernanceMock, rollbackConfigMock } from '../mockGateway'

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
})
