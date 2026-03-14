import { describe, expect, it } from 'vitest'
import { gatewayApi } from '../gateway'

describe('gatewayApi mappings adapter', () => {
  it('supports CRUD via mappings endpoints (mock mode primary adapter)', async () => {
    const initial = await gatewayApi.fetchMappings()

    const payload = {
      mapping_id: 'map-adapter-test',
      name: 'adapter test',
      enabled: true,
      peer_node_id: 'peer-1',
      local_bind_ip: '127.0.0.1',
      local_bind_port: 18089,
      local_base_path: '/in',
      remote_target_ip: '127.0.0.2',
      remote_target_port: 8089,
      remote_base_path: '/out',
      allowed_methods: ['POST'],
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

    await gatewayApi.updateMapping(payload.mapping_id, { ...payload, name: 'adapter test v2' })
    listed = await gatewayApi.fetchMappings()
    expect(listed.items.find((item) => item.mapping_id === payload.mapping_id)?.name).toBe('adapter test v2')

    await gatewayApi.deleteMapping(payload.mapping_id)
    listed = await gatewayApi.fetchMappings()
    expect(listed.items.find((item) => item.mapping_id === payload.mapping_id)).toBeUndefined()
  })
})
