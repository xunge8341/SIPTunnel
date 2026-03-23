import { beforeEach, describe, expect, it, vi } from 'vitest'

const requestMock = vi.fn()
vi.mock('../http', () => ({ request: requestMock }))

describe('gatewayApi access logs contract', () => {
  beforeEach(() => {
    vi.resetModules()
    requestMock.mockReset()
  })

  it('calls GET /access-logs with contract-aligned params', async () => {
    requestMock.mockResolvedValue({
      data: {
        items: [],
        pagination: { total: 0, page: 1, page_size: 50 }
      }
    })
    const { gatewayApi } = await import('../gateway')
    await gatewayApi.fetchAccessLogs({ mapping: 'm1', sourceIP: '10.0.0.8', method: 'GET', slowOnly: true }, 1, 50)
    expect(requestMock).toHaveBeenCalledWith('/access-logs', {
      method: 'GET',
      params: {
        status: undefined,
        mapping: 'm1',
        source_ip: '10.0.0.8',
        method: 'GET',
        slow_only: true,
        page: 1,
        page_size: 50
      }
    })
  })
})
