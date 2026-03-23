export interface NetworkModeProfile {
  value: string
  shortLabel: string
  flowLabel: string
  senderRole: string
  receiverRole: string
  requestDirection: string
  responseDirection: string
}

export const NETWORK_MODE_PROFILES: NetworkModeProfile[] = [
  {
    value: 'SENDER_SIP__RECEIVER_SIP',
    shortLabel: '模式0：SIP --> | <-- SIP',
    flowLabel: '本级域/发起侧: SIP --> | <-- SIP : 级联对端/应答侧',
    senderRole: '本级域（发起侧）仅发送 SIP 请求',
    receiverRole: '级联对端（应答侧）仅通过 SIP 返回响应',
    requestDirection: '单向请求：发送端 -> 接收端（SIP）',
    responseDirection: '单向响应：接收端 -> 发送端（SIP）'
  },
  {
    value: 'SENDER_SIP__RECEIVER_RTP',
    shortLabel: '模式1：SIP --> | <-- RTP',
    flowLabel: '本级域/发起侧: SIP --> | <-- RTP : 级联对端/应答侧',
    senderRole: '本级域（发起侧）仅发送 SIP 请求',
    receiverRole: '级联对端（应答侧）通过 RTP 回传响应',
    requestDirection: '单向请求：发送端 -> 接收端（SIP）',
    responseDirection: '单向响应：接收端 -> 发送端（RTP）'
  },
  {
    value: 'SENDER_SIP__RECEIVER_SIP_RTP',
    shortLabel: '模式2：SIP --> | <-- SIP&RTP',
    flowLabel: '本级域/发起侧: SIP --> | <-- SIP&RTP : 级联对端/应答侧',
    senderRole: '本级域（发起侧）仅发送 SIP 请求',
    receiverRole: '级联对端（应答侧）可用 SIP/RTP 返回响应',
    requestDirection: '单向请求：发送端 -> 接收端（SIP）',
    responseDirection: '双通道响应：接收端 -> 发送端（SIP 或 RTP）'
  },
  {
    value: 'SENDER_SIP_RTP__RECEIVER_SIP_RTP',
    shortLabel: '模式3：SIP&RTP --> | <-- SIP&RTP',
    flowLabel: '本级域/发起侧: SIP&RTP --> | <-- SIP&RTP : 级联对端/应答侧',
    senderRole: '本级域（发起侧）支持 SIP/RTP 发送',
    receiverRole: '级联对端（应答侧）支持 SIP/RTP 收发',
    requestDirection: '双通道请求：发送端 -> 接收端（SIP 或 RTP）',
    responseDirection: '双通道响应：接收端 -> 发送端（SIP 或 RTP）'
  }
]

export function getNetworkModeProfile(mode: string): NetworkModeProfile | undefined {
  return NETWORK_MODE_PROFILES.find((item) => item.value === mode)
}
