package config

const (
	TransportSIPControl    = "sip_control"
	TransportSIPBodyJSON   = "sip_body_json"
	TransportSIPBodyOnly   = "sip_body_only"
	TransportSIPOrRTPAuto  = "sip_or_rtp_auto"
	TransportRTPStream     = "rtp_stream"
	TransportNone          = "none"
	UnlimitedBodySizeLimit = -1
)

type TunnelTransportPlan struct {
	RequestMetaTransport  string   `json:"request_meta_transport"`
	RequestBodyTransport  string   `json:"request_body_transport"`
	ResponseMetaTransport string   `json:"response_meta_transport"`
	ResponseBodyTransport string   `json:"response_body_transport"`
	RequestBodySizeLimit  int      `json:"request_body_size_limit"`
	ResponseBodySizeLimit int      `json:"response_body_size_limit"`
	Notes                 []string `json:"notes"`
	Warnings              []string `json:"warnings"`
}

// ResolveTransportPlan 统一根据全局 NetworkMode/Capability 推导 HTTP 映射承载策略。
// 注意：该计划属于系统全局能力，不属于逐条映射可编辑字段。
func ResolveTransportPlan(mode NetworkMode) TunnelTransportPlan {
	capability := DeriveCapability(mode)
	plan := TunnelTransportPlan{
		RequestMetaTransport:  TransportSIPControl,
		ResponseMetaTransport: TransportSIPControl,
		RequestBodyTransport:  TransportNone,
		ResponseBodyTransport: TransportNone,
		RequestBodySizeLimit:  0,
		ResponseBodySizeLimit: 0,
		Notes: []string{
			"transport 决策由全局 network.mode 推导，禁止在单条映射上覆盖。",
			"request/response 元信息固定走 SIP 控制面，避免业务侧分散拼接承载逻辑。",
		},
	}

	if capability.SupportsSmallRequestBody {
		plan.RequestBodyTransport = TransportSIPBodyJSON
		plan.RequestBodySizeLimit = DefaultNetworkConfig().SIP.MaxMessageBytes
		plan.Notes = append(plan.Notes, "小请求体默认走 SIP JSON 载荷。")
	} else {
		plan.Warnings = append(plan.Warnings, "当前模式不支持请求体承载，仅允许控制类空载请求。")
	}

	if capability.SupportsLargeRequestBody {
		plan.RequestBodyTransport = TransportSIPOrRTPAuto
		plan.RequestBodySizeLimit = UnlimitedBodySizeLimit
		plan.Notes = append(plan.Notes, "大请求体可按阈值自动切换 RTP 上行承载。")
	} else if capability.SupportsSmallRequestBody {
		plan.RequestBodyTransport = TransportSIPBodyOnly
		plan.Warnings = append(plan.Warnings, "不支持大请求体上传；超过 SIP 限制的请求体将被拒绝。")
	}

	if capability.SupportsLargeResponseBody {
		plan.ResponseBodyTransport = TransportRTPStream
		plan.ResponseBodySizeLimit = UnlimitedBodySizeLimit
		plan.Notes = append(plan.Notes, "响应体默认走 RTP 回传，支持大载荷与流式输出。")
	} else if capability.SupportsSmallRequestBody {
		plan.ResponseBodyTransport = TransportSIPBodyJSON
		plan.ResponseBodySizeLimit = DefaultNetworkConfig().SIP.MaxMessageBytes
		plan.Warnings = append(plan.Warnings, "不支持大响应体回传；响应体将受 SIP 大小限制。")
	} else {
		plan.Warnings = append(plan.Warnings, "当前模式不支持响应体承载。")
	}

	switch mode.Normalize() {
	case NetworkModeSenderSIPReceiverRTP:
		plan.Notes = append(plan.Notes, "模式=SENDER_SIP__RECEIVER_RTP：适合小请求 + 大响应。")
	case NetworkModeSenderSIPRTPReceiverAll:
		plan.Notes = append(plan.Notes, "模式=SENDER_SIP_RTP__RECEIVER_SIP_RTP：支持双向大载荷承载。")
	case NetworkModeSenderSIPReceiverSIPRTP:
		plan.Notes = append(plan.Notes, "模式=SENDER_SIP__RECEIVER_SIP_RTP：上行大请求受限，下行大响应可用。")
	default:
		plan.Warnings = append(plan.Warnings, "未知/预留 network.mode，transport 计划已降级为最小可用能力。")
	}

	return plan
}
