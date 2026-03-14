import { beforeEach, describe, expect, it, vi } from 'vitest'

const requestMock = vi.fn()

vi.mock('../http', () => ({
  request: requestMock
}))

const loadApi = async () => {
  vi.resetModules()
  return import('../gateway')
}

describe('gatewayApi mode switch', () => {
  beforeEach(() => {
    requestMock.mockReset()
    requestMock.mockResolvedValue({ data: { items: [], warnings: [] } })
    vi.unstubAllEnvs()
  })

  it('uses real backend by default when VITE_API_MODE is unset', async () => {
    const { gatewayApi } = await loadApi()
    await gatewayApi.fetchMappings()
    expect(requestMock).toHaveBeenCalledWith('/mappings', { method: 'GET' })
  })


  it('calls real mapping test endpoint when VITE_API_MODE is unset', async () => {
    requestMock.mockResolvedValue({ data: { sip_request: 'success', rtp_channel: 'fail' } })
    const { gatewayApi } = await loadApi()
    await gatewayApi.testMapping()
    expect(requestMock).toHaveBeenCalledWith('/mapping/test', { method: 'POST' })
  })

  it('uses mock only when VITE_API_MODE=mock', async () => {
    vi.stubEnv('VITE_API_MODE', 'mock')
    const { gatewayApi } = await loadApi()
    await gatewayApi.fetchMappings()
    expect(requestMock).not.toHaveBeenCalled()
  })
})

