package server

import (
	"encoding/base64"
	"log"
	"net/http"
	"strconv"
	"strings"

	"siptunnel/internal/tunnelmapping"
)

const (
	responseModeAuto   = "AUTO"
	responseModeInline = "INLINE"
	responseModeRTP    = "RTP"

	// inlineSIPStartLineAndHeadersBytes 是对 SIP 起始行和头部开销的保守估算，
	// 用来避免预算模型只看 XML 正文而低估最终 UDP 负载。
	inlineSIPStartLineAndHeadersBytes = 180
	// inlineMANSCDPXMLWrapBytes 是对 MANSCDP 外层 XML 包装的保守估算，
	// 这里单独列项，便于日志和现场排障时理解预算为什么变紧。
	inlineMANSCDPXMLWrapBytes = 96
)

type responseModeDecision struct {
	Mode                  string
	RequestedMode         string
	EffectiveMode         string
	FinalMode             string
	Reason                string
	ResponseShape         string
	ContentLength         int64
	EstimatedWireBytes    int64
	ActualWireBytes       int64
	EncodedBodyBytes      int64
	UDPBudgetBytes        int64
	SafetyReserveBytes    int64
	SIPHeaderBytes        int64
	MANSCDPXMLWrapBytes   int64
	EnvelopeOverheadBytes int64
	HeadroomBytes         int64
	InlineBodyBudgetBytes int64
	ResponseModePolicy    string
}

func responseModePolicyName() string {
	return "AUTO(budget_driven_inline_or_rtp)"
}

func normalizeResponseMode(mode string) string {
	return tunnelmapping.NormalizeResponseMode(mode)
}

func preferredResponseMode(requested string, mapping tunnelmapping.TunnelMapping) string {
	requested = normalizeResponseMode(requested)
	if requested != responseModeAuto {
		return requested
	}
	return normalizeResponseMode(mapping.ResponseMode)
}

func hasRTPDestination(session *gbInboundSession) bool {
	if session == nil {
		return false
	}
	return strings.TrimSpace(session.remoteRTPIP) != "" && session.remoteRTPPort > 0
}

// responseModeDecisionForHeaders 执行第一阶段“预判”：
// 1. 先根据响应头估算 INLINE 可承载预算；
// 2. 再用响应画像（tiny_control / small_page_data / uncertain_streaming / bulk_download）决定倾向 INLINE 还是 RTP；
// 3. 只有在已知长度、且长度落入安全预算时，AUTO 才会预判为 INLINE。
func responseModeDecisionForHeaders(requested string, mapping tunnelmapping.TunnelMapping, prepared *mappingForwardRequest, resp *http.Response, session *gbInboundSession) responseModeDecision {
	decision := inlineBudgetDecision(session, mapping, parseResponseContentLength(resp), -1)
	decision.RequestedMode = firstNonEmpty(strings.TrimSpace(requested), responseModeAuto)
	decision.ResponseShape = string(classifyResponseShape(prepared, resp, decision.ContentLength, decision.InlineBodyBudgetBytes))
	preferred := preferredResponseMode(requested, mapping)
	decision.Mode = preferred
	decision.EffectiveMode = preferred
	switch preferred {
	case responseModeRTP:
		decision.Reason = "requested_or_mapping_rtp"
		decision.FinalMode = decision.Mode
		return decision
	case responseModeInline:
		decision.Reason = "requested_or_mapping_inline"
		decision.FinalMode = decision.Mode
		return decision
	}
	if !hasRTPDestination(session) {
		decision.Mode = responseModeInline
		decision.EffectiveMode = decision.Mode
		decision.FinalMode = decision.Mode
		decision.Reason = "rtp_destination_unavailable"
		return decision
	}
	switch responseShape(decision.ResponseShape) {
	case responseShapeTinyControl, responseShapeSmallPageData:
		if decision.ContentLength >= 0 && decision.InlineBodyBudgetBytes > 0 && decision.ContentLength <= decision.InlineBodyBudgetBytes {
			decision.Mode = responseModeInline
			decision.EffectiveMode = decision.Mode
			decision.FinalMode = decision.Mode
			decision.Reason = "shape_within_inline_budget"
			return decision
		}
		decision.Mode = responseModeRTP
		decision.EffectiveMode = decision.Mode
		decision.FinalMode = decision.Mode
		if decision.ContentLength >= 0 {
			decision.Reason = "shape_exceeds_inline_budget"
		} else {
			decision.Reason = "shape_requires_known_length"
		}
		return decision
	case responseShapeUncertainStream:
		decision.Mode = responseModeRTP
		decision.EffectiveMode = decision.Mode
		decision.FinalMode = decision.Mode
		decision.Reason = "uncertain_streaming_response"
		return decision
	default:
		decision.Mode = responseModeRTP
		decision.EffectiveMode = decision.Mode
		decision.FinalMode = decision.Mode
		decision.Reason = "bulk_response_prefers_rtp"
		return decision
	}
}

// finalizeResponseBodyMode 执行第二阶段“终判”：
// 在 body 已知后再次核算：若实际包长（含 base64 膨胀、XML/SIP 包裹、reserve/headroom）仍在预算内，才允许最终 INLINE，否则切回 RTP。
func finalizeResponseBodyMode(currentMode string, mapping tunnelmapping.TunnelMapping, session *gbInboundSession, body []byte) (string, responseModeDecision) {
	decision := inlineBudgetDecision(session, mapping, int64(len(body)), int64(len(body)))
	decision.ResponseShape = string(classifyResponseShape(nil, nil, int64(len(body)), decision.InlineBodyBudgetBytes))
	decision.Mode = normalizeResponseMode(currentMode)
	decision.EffectiveMode = decision.Mode
	if decision.Mode != responseModeInline {
		decision.Reason = "non_inline_mode_retained"
		decision.FinalMode = decision.Mode
		return decision.Mode, decision
	}
	if decision.InlineBodyBudgetBytes > 0 && int64(len(body)) <= decision.InlineBodyBudgetBytes {
		decision.Reason = "inline_body_within_budget"
		decision.FinalMode = decision.Mode
		return decision.Mode, decision
	}
	if hasRTPDestination(session) {
		decision.Mode = responseModeRTP
		decision.Reason = "inline_actual_body_exceeds_budget"
		decision.FinalMode = decision.Mode
		return decision.Mode, decision
	}
	decision.Reason = "inline_budget_exceeded_without_rtp_fallback"
	decision.FinalMode = decision.Mode
	return decision.Mode, decision
}

func shouldForceInlineResponse(statusCode int, contentLength int64, currentMode string, session *gbInboundSession, mapping tunnelmapping.TunnelMapping) bool {
	mode := normalizeResponseMode(currentMode)
	if mode == responseModeInline {
		return false
	}
	if !hasRTPDestination(session) {
		return true
	}
	if statusCode >= http.StatusBadRequest {
		decision := inlineBudgetDecision(session, mapping, contentLength, -1)
		return contentLength >= 0 && decision.InlineBodyBudgetBytes > 0 && contentLength <= decision.InlineBodyBudgetBytes
	}
	return false
}

// inlineBudgetDecision 统一计算 UDP INLINE 安全预算。
// 预算模型：
//
//	可用线长预算 = udp_budget - safety_reserve - SIP 头开销 - XML 包装开销 - 额外 envelope_overhead - headroom
//	INLINE body 预算 = 可用线长预算 * 3/4
func inlineBudgetDecision(session *gbInboundSession, mapping tunnelmapping.TunnelMapping, contentLength int64, actualBodyLen int64) responseModeDecision {
	transport := "UDP"
	if session != nil && strings.TrimSpace(session.transport) != "" {
		transport = strings.ToUpper(strings.TrimSpace(session.transport))
	}
	mappingLimit := maxOrDefault(mapping.MaxInlineResponseBody, tunnelmapping.DeriveBodyLimitProfile(mapping.ResponseMode, false).MaxInlineResponseBody)
	decision := responseModeDecision{
		ContentLength:         contentLength,
		ActualWireBytes:       -1,
		EncodedBodyBytes:      -1,
		UDPBudgetBytes:        int64(inlineResponseUDPBudgetBytes()),
		SafetyReserveBytes:    int64(inlineResponseSafetyReserveBytes()),
		SIPHeaderBytes:        inlineSIPStartLineAndHeadersBytes,
		MANSCDPXMLWrapBytes:   inlineMANSCDPXMLWrapBytes,
		EnvelopeOverheadBytes: int64(inlineResponseEnvelopeOverheadBytes()),
		ResponseModePolicy:    responseModePolicyName(),
	}
	decision.HeadroomBytes = int64(float64(decision.UDPBudgetBytes) * inlineResponseHeadroomRatio())
	decision.InlineBodyBudgetBytes = mappingLimit
	if transport == "UDP" {
		effectiveWireBudget := decision.UDPBudgetBytes - decision.SafetyReserveBytes - decision.SIPHeaderBytes - decision.MANSCDPXMLWrapBytes - decision.EnvelopeOverheadBytes - decision.HeadroomBytes
		if effectiveWireBudget < 0 {
			effectiveWireBudget = 0
		}
		udpBodyBudget := effectiveWireBudget * 3 / 4
		if udpBodyBudget < decision.InlineBodyBudgetBytes || decision.InlineBodyBudgetBytes <= 0 {
			decision.InlineBodyBudgetBytes = udpBodyBudget
		}
	}
	if decision.ContentLength >= 0 {
		decision.EstimatedWireBytes = estimateInlineWireBytes(decision.ContentLength)
		decision.EncodedBodyBytes = int64(base64.StdEncoding.EncodedLen(int(decision.ContentLength)))
	}
	if actualBodyLen >= 0 {
		decision.ActualWireBytes = estimateInlineWireBytes(actualBodyLen)
		decision.EncodedBodyBytes = int64(base64.StdEncoding.EncodedLen(int(actualBodyLen)))
	}
	return decision
}

// estimateInlineWireBytes 估算 body 通过 INLINE 承载时的实际线上字节数。
// 这里显式把 base64 膨胀和 MANSCDP/SIP 包裹开销加进去，保证日志和预算使用同一模型。
func estimateInlineWireBytes(bodyLen int64) int64 {
	if bodyLen < 0 {
		return -1
	}
	if bodyLen > int64(^uint(0)>>1) {
		return bodyLen
	}
	return int64(base64.StdEncoding.EncodedLen(int(bodyLen))) + int64(inlineResponseEnvelopeOverheadBytes()) + inlineSIPStartLineAndHeadersBytes + inlineMANSCDPXMLWrapBytes
}

func parseResponseContentLength(resp *http.Response) int64 {
	if resp == nil {
		return -1
	}
	if resp.ContentLength >= 0 {
		return resp.ContentLength
	}
	if resp.Header == nil {
		return -1
	}
	v := strings.TrimSpace(resp.Header.Get("Content-Length"))
	if v == "" {
		return -1
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return -1
	}
	return n
}
func logRequestedResponseModeDecision(session *gbInboundSession, mapping tunnelmapping.TunnelMapping, prepared *mappingForwardRequest, requestedMode string) {
	if session == nil {
		return
	}
	target := "-"
	if prepared != nil && prepared.TargetURL != nil {
		target = prepared.TargetURL.String()
	}
	requested := firstNonEmpty(strings.TrimSpace(requestedMode), responseModeAuto)
	log.Printf("gb28181 inbound stage=response_mode_decision call_id=%s mapping_id=%s device_id=%s decision_mode=requested requested_mode=%s effective_mode=- final_mode=- decision_reason=requested_or_mapping_preference request_class=%s response_mode_policy=%s target_url=%s", session.callID, mapping.MappingID, session.deviceID, requested, string(preferredResponseShape(mapping)), responseModePolicyName(), target)
}

func logResponseModeDecision(session *gbInboundSession, mapping tunnelmapping.TunnelMapping, prepared *mappingForwardRequest, requestedMode string, decisionMode string, decision responseModeDecision) {
	if session == nil {
		return
	}
	target := "-"
	if prepared != nil && prepared.TargetURL != nil {
		target = prepared.TargetURL.String()
	}
	rangePlayback := isRangePlaybackRequest(prepared, nil, nil)
	effectiveMode := firstNonEmpty(strings.TrimSpace(decision.EffectiveMode), decision.Mode)
	finalMode := firstNonEmpty(strings.TrimSpace(decision.FinalMode), decision.Mode)
	log.Printf("gb28181 inbound stage=response_mode_decision call_id=%s mapping_id=%s device_id=%s decision_mode=%s requested_mode=%s effective_mode=%s final_mode=%s decision_reason=%s request_class=%s response_mode_policy=%s range_playback=%t content_length=%d estimated_wire_bytes=%d actual_wire_bytes=%d encoded_body_bytes=%d udp_budget_bytes=%d safety_reserve_bytes=%d sip_header_bytes=%d manscdp_xml_wrap_bytes=%d envelope_overhead_bytes=%d headroom_bytes=%d inline_body_budget_bytes=%d target_url=%s", session.callID, mapping.MappingID, session.deviceID, decisionMode, firstNonEmpty(strings.TrimSpace(requestedMode), responseModeAuto), effectiveMode, finalMode, decision.Reason, firstNonEmpty(decision.ResponseShape, string(preferredResponseShape(mapping))), firstNonEmpty(strings.TrimSpace(decision.ResponseModePolicy), responseModePolicyName()), rangePlayback, decision.ContentLength, decision.EstimatedWireBytes, decision.ActualWireBytes, decision.EncodedBodyBytes, decision.UDPBudgetBytes, decision.SafetyReserveBytes, decision.SIPHeaderBytes, decision.MANSCDPXMLWrapBytes, decision.EnvelopeOverheadBytes, decision.HeadroomBytes, decision.InlineBodyBudgetBytes, target)
}

func segmentChildParallelism(prepared *mappingForwardRequest, mapping tunnelmapping.TunnelMapping) int {
	base := maxIntVal(1, udpSegmentParallelismPerDevice())
	profile := segmentedDownloadProfileForLane(prepared, mapping)
	return maxIntVal(base, profile.concurrency*2)
}

func classifyUDPRequestLane(prepared *mappingForwardRequest, mapping tunnelmapping.TunnelMapping) (string, int) {
	if prepared != nil && strings.EqualFold(strings.TrimSpace(prepared.Headers.Get(internalRangeFetchHeader)), "1") {
		return "segment", segmentChildParallelism(prepared, mapping)
	}
	if classifyTrafficProfile(prepared, nil, nil) == trafficProfileRangePlayback {
		return "playback", maxIntVal(1, udpBulkParallelismPerDevice())
	}
	if normalizeResponseMode(mapping.ResponseMode) == responseModeRTP {
		return "bulk", maxIntVal(1, udpBulkParallelismPerDevice())
	}
	shape := classifyResponseShape(prepared, nil, -1, 0)
	switch shape {
	case responseShapeTinyControl, responseShapeSmallPageData:
		return "small", maxIntVal(1, udpRequestParallelismPerDevice())
	default:
		return "bulk", maxIntVal(1, udpBulkParallelismPerDevice())
	}
}
