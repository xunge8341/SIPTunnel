import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest'
import { gatewayApi } from '../gateway'

describe('gatewayApi node/peer adapter', () => {
  beforeAll(() => {
    vi.stubEnv('VITE_API_MODE', 'mock')
  })

  afterAll(() => {
    vi.unstubAllEnvs()
  })

  it('supports local node read/write and peer CRUD in mock mode', async () => {
    const detail = await gatewayApi.fetchNodeDetail()
    expect(detail.local_node.node_id).toBeTruthy()

    const updated = await gatewayApi.updateLocalNode({
      ...detail.local_node,
      node_name: 'Gateway A Updated'
    })
    expect(updated.node_name).toBe('Gateway A Updated')

    const peerPayload = {
      peer_node_id: 'peer-adapter-spec',
      peer_name: 'peer adapter',
      peer_signaling_ip: '10.0.0.1',
      peer_signaling_port: 5060,
      peer_media_ip: '10.0.0.2',
      peer_media_port_start: 32000,
      peer_media_port_end: 32100,
      supported_network_mode: detail.current_network_mode,
      enabled: true
    }

    await gatewayApi.createPeer(peerPayload)
    let peers = await gatewayApi.fetchPeers()
    expect(peers.items.some((item) => item.peer_node_id === peerPayload.peer_node_id)).toBe(true)

    await gatewayApi.updatePeer(peerPayload.peer_node_id, {
      ...peerPayload,
      peer_name: 'peer adapter v2'
    })
    peers = await gatewayApi.fetchPeers()
    expect(peers.items.find((item) => item.peer_node_id === peerPayload.peer_node_id)?.peer_name).toBe('peer adapter v2')

    await gatewayApi.deletePeer(peerPayload.peer_node_id)
    peers = await gatewayApi.fetchPeers()
    expect(peers.items.some((item) => item.peer_node_id === peerPayload.peer_node_id)).toBe(false)

    const config = await gatewayApi.fetchNodeConfig()
    expect(config.local_node.device_id).toBeTruthy()

    const saveResult = await gatewayApi.saveNodeConfig({
      local_node: { ...config.local_node, device_id: 'gateway-a-m31' },
      peer_node: config.peer_node
    })
    expect(saveResult.tunnel_restarted).toBe(true)

    const status = await gatewayApi.fetchNodeNetworkStatus()
    expect(status.current_network_mode).toBeTruthy()
    expect(Array.isArray(status.capability_summary.supported)).toBe(true)
  })
})
