import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { gatewayApi } from '../gateway'

describe('gatewayApi mappings adapter', () => {
  beforeAll(() => {
    vi.stubEnv('VITE_API_MODE', 'mock')
  })

  afterAll(() => {
    vi.unstubAllEnvs()
  })

  it('supports CRUD via mappings endpoints (mock mode primary adapter)', async () => {
    const initial = await gatewayApi.fetchMappings()

    const payload = {
      mapping_id: 'map-adapter-test',
      enabled: true,
      local_bind_ip: '127.0.0.1',
      local_bind_port: 18089,
      local_base_path: '/in',
      remote_target_ip: '127.0.0.2',
      remote_target_port: 8089,
      remote_base_path: '/out',
      allowed_methods: ['*'],
      connect_timeout_ms: 500,
      request_timeout_ms: 3000,
      response_timeout_ms: 3000,
      max_request_body_bytes: 1024,
      max_response_body_bytes: 4096,
      require_streaming_response: false,
      description: 'adapter test'
    }

    await gatewayApi.createMapping(payload)
    let listed = await gatewayApi.fetchMappings()
    expect(listed.items.length).toBe(initial.items.length + 1)

    await gatewayApi.updateMapping(payload.mapping_id, { ...payload, remote_base_path: '/out-v2' })
    listed = await gatewayApi.fetchMappings()
    expect(listed.items.find((item) => item.mapping_id === payload.mapping_id)?.remote_base_path).toBe('/out-v2')

    await gatewayApi.deleteMapping(payload.mapping_id)
    listed = await gatewayApi.fetchMappings()
    expect(listed.items.find((item) => item.mapping_id === payload.mapping_id)).toBeUndefined()
  })


  it('supports mapping test in mock mode', async () => {
    const result = await gatewayApi.testMapping()
    expect(result).toEqual({ sip_request: 'success', rtp_channel: 'fail' })
  })

})
