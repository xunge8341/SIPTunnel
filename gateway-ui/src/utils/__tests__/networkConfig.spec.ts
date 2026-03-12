import { describe, expect, it } from 'vitest'
import { detectHighRiskChanges, formatPortRange } from '../networkConfig'
import type { NetworkConfigPayload, UpdateNetworkConfigPayload } from '../../types/gateway'

const baseConfig: NetworkConfigPayload = {
  sip: {
    listenIp: '0.0.0.0',
    listenPort: 5060,
    protocol: 'UDP',
    advertisedAddress: 'sip.example.com:5060',
    domain: 'gateway.local'
  },
  rtp: {
    listenIp: '0.0.0.0',
    portRangeStart: 20000,
    portRangeEnd: 21000,
    protocol: 'UDP',
    advertisedAddress: 'rtp.example.com',
    maxConcurrentTransfers: 120
  },
  portPool: {
    totalAvailablePorts: 1000,
    occupiedPorts: 120,
    activeTransfers: 48
  }
}

describe('networkConfig utils', () => {
  it('detects high risk changes', () => {
    const draft: UpdateNetworkConfigPayload = {
      sip: { ...baseConfig.sip, listenPort: 5070 },
      rtp: { ...baseConfig.rtp, maxConcurrentTransfers: 180 }
    }

    const changes = detectHighRiskChanges(baseConfig, draft)
    expect(changes).toHaveLength(2)
    expect(changes.map((item) => item.field)).toEqual(['SIP 监听端口', '最大并发传输数'])
  })

  it('formats port range', () => {
    expect(formatPortRange(10000, 10099)).toBe('10000 - 10099')
  })
})
