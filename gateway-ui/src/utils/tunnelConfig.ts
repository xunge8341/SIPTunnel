import type { TunnelConfigCapability, TunnelConfigCapabilityItem, TunnelConfigPayload } from '../types/gateway'

export function deriveTunnelCapability(config: Pick<TunnelConfigPayload, 'network_mode'>): TunnelConfigCapability {
  if (config.network_mode === 'SENDER_SIP_RTP__RECEIVER_SIP_RTP') {
    return {
      supports_small_request_body: true,
      supports_large_request_body: true,
      supports_large_response_body: true,
      supports_streaming_response: true,
      supports_bidirectional_http_tunnel: true,
      supports_transparent_http_proxy: true
    }
  }

  if (config.network_mode === 'SENDER_SIP__RECEIVER_SIP') {
    return {
      supports_small_request_body: true,
      supports_large_request_body: false,
      supports_large_response_body: false,
      supports_streaming_response: false,
      supports_bidirectional_http_tunnel: false,
      supports_transparent_http_proxy: false
    }
  }

  if (config.network_mode === 'SENDER_SIP__RECEIVER_RTP' || config.network_mode === 'SENDER_SIP__RECEIVER_SIP_RTP') {
    return {
      supports_small_request_body: true,
      supports_large_request_body: false,
      supports_large_response_body: true,
      supports_streaming_response: true,
      supports_bidirectional_http_tunnel: false,
      supports_transparent_http_proxy: false
    }
  }

  return {
    supports_small_request_body: false,
    supports_large_request_body: false,
    supports_large_response_body: false,
    supports_streaming_response: false,
    supports_bidirectional_http_tunnel: false,
    supports_transparent_http_proxy: false
  }
}

export function buildTunnelCapabilityItems(capability: TunnelConfigCapability): TunnelConfigCapabilityItem[] {
  return [
    { key: 'supports_small_request_body', supported: capability.supports_small_request_body, description: '支持小请求体（典型 SIP 载荷范围）' },
    { key: 'supports_large_request_body', supported: capability.supports_large_request_body, description: '支持大请求体上传' },
    { key: 'supports_large_response_body', supported: capability.supports_large_response_body, description: '支持大响应体回传' },
    { key: 'supports_streaming_response', supported: capability.supports_streaming_response, description: '支持流式响应/分块回传' },
    { key: 'supports_bidirectional_http_tunnel', supported: capability.supports_bidirectional_http_tunnel, description: '支持双向 HTTP 隧道' },
    { key: 'supports_transparent_http_proxy', supported: capability.supports_transparent_http_proxy, description: '支持透明 HTTP 代理' }
  ]
}
