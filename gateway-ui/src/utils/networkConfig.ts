import type { NetworkConfigPayload, UpdateNetworkConfigPayload } from '../types/gateway'

export interface HighRiskChange {
  field: string
  before: string | number
  after: string | number
  risk: string
}

const highRiskMap: Record<string, { label: string; risk: string }> = {
  'sip.listenPort': { label: 'SIP 监听端口', risk: '变更后可能导致控制面连接中断。' },
  'sip.protocol': { label: 'SIP 协议', risk: '协议切换可能造成上下游协商失败。' },
  'sip.advertisedAddress': { label: 'SIP 对外通告地址', risk: '变更后外部节点可能短期无法回连。' },
  'rtp.portRangeStart': { label: 'RTP 端口范围', risk: '端口池变化可能影响正在进行的传输会话。' },
  'rtp.portRangeEnd': { label: 'RTP 端口范围', risk: '端口池变化可能影响正在进行的传输会话。' },
  'rtp.protocol': { label: 'RTP 协议', risk: '协议切换可能导致传输链路不兼容。' },
  'rtp.maxConcurrentTransfers': { label: '最大并发传输数', risk: '阈值调整可能引发性能抖动或排队积压。' }
}

export const detectHighRiskChanges = (
  current: NetworkConfigPayload,
  draft: UpdateNetworkConfigPayload
): HighRiskChange[] => {
  const result: HighRiskChange[] = []

  Object.entries(highRiskMap).forEach(([path, meta]) => {
    const [group, key] = path.split('.') as ['sip' | 'rtp', string]
    const before = current[group][key as keyof (typeof current)[typeof group]]
    const after = draft[group][key as keyof (typeof draft)[typeof group]]

    if (before !== after) {
      result.push({ field: meta.label, before: String(before), after: String(after), risk: meta.risk })
    }
  })

  return result
}

export const formatPortRange = (start: number, end: number) => `${start} - ${end}`
