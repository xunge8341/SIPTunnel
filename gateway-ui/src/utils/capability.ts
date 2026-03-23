import type { Capability, CapabilityItem, CapabilitySummary, StartupSummaryPayload, TunnelMapping } from '../types/gateway'

const stableMethodWarningSet = new Set(['PUT', 'PATCH', 'DELETE'])
const transparentTunnelMethodSet = new Set(['CONNECT', 'TRACE'])

export function buildCapabilityMatrix(capability: Capability, summary?: CapabilitySummary): CapabilityItem[] {
  if (summary?.items?.length) {
    return summary.items
  }
  return [
    { key: 'small_request', label: '小请求体（控制面常规请求）', supported: true, note: '默认支持，用于常规接口调用。' },
    {
      key: 'large_request',
      label: '大请求体（上传/批量提交）',
      supported: capability.supports_large_request_body,
      note: capability.supports_large_request_body ? '可配置更大请求体上限。' : '不支持大请求体，请保持请求体在模式限制内。'
    },
    {
      key: 'large_response',
      label: '大响应体（下载/批量返回）',
      supported: capability.supports_large_response_body,
      note: capability.supports_large_response_body ? '可承载大响应体。' : '响应体过大将被限制或失败。'
    },
    {
      key: 'streaming_response',
      label: '流式响应（SSE/分片输出）',
      supported: capability.supports_streaming_response,
      note: capability.supports_streaming_response ? '可在映射中启用流式响应。' : '当前模式不支持流式响应。'
    },
    {
      key: 'bidirectional_http_tunnel',
      label: '双向透明 HTTP tunnel（CONNECT/TRACE）',
      supported: capability.supports_bidirectional_http_tunnel,
      note: capability.supports_bidirectional_http_tunnel ? '支持透明隧道类请求。' : '不支持透明 tunnel，请避免 CONNECT/TRACE。'
    }
  ]
}

function resolveBodyLimit(value: number) {
  return Number.isFinite(value) && value > 0 ? value : 0
}

export function evaluateMappingCapability(
  mapping: TunnelMapping,
  startupSummary?: StartupSummaryPayload
): { blockingIssues: string[]; advisoryWarnings: string[] } {
  if (!startupSummary) {
    return { blockingIssues: [], advisoryWarnings: [] }
  }

  const cap = startupSummary.capability
  const requestLimit = resolveBodyLimit(startupSummary.transport_plan.request_body_size_limit)
  const responseLimit = resolveBodyLimit(startupSummary.transport_plan.response_body_size_limit)

  const allowedMethods = mapping.allowed_methods ?? []

  const blockingIssues: string[] = []
  if (!cap.supports_large_request_body && requestLimit > 0 && mapping.max_request_body_bytes > requestLimit) {
    blockingIssues.push(`当前模式仅支持小请求体，max_request_body_bytes 不能超过 ${requestLimit}`)
  }
  if (!cap.supports_large_response_body && responseLimit > 0 && mapping.max_response_body_bytes > responseLimit) {
    blockingIssues.push(`当前模式不支持大响应体，max_response_body_bytes 不能超过 ${responseLimit}`)
  }
  if (mapping.require_streaming_response && !cap.supports_streaming_response) {
    blockingIssues.push('当前模式不支持流式响应，请关闭“流式响应”开关')
  }
  if (!cap.supports_bidirectional_http_tunnel && allowedMethods.some((method) => transparentTunnelMethodSet.has(method.toUpperCase()))) {
    blockingIssues.push('当前模式不支持双向透明 HTTP tunnel，请移除 CONNECT/TRACE 方法')
  }

  const advisoryWarnings: string[] = []
  if (!cap.supports_bidirectional_http_tunnel && allowedMethods.some((method) => stableMethodWarningSet.has(method.toUpperCase()))) {
    advisoryWarnings.push('当前模式下 PUT/PATCH/DELETE 可能不稳定，建议压测后再上线')
  }

  return { blockingIssues, advisoryWarnings }
}
