package config

import (
	"fmt"
	"strings"
)

type NetworkMode string

const (
	// 新模型：按发送端 / 接收端表达链路能力。
	NetworkModeSenderSIPReceiverRTP    NetworkMode = "SENDER_SIP__RECEIVER_RTP"
	NetworkModeSenderSIPReceiverSIPRTP NetworkMode = "SENDER_SIP__RECEIVER_SIP_RTP"
	NetworkModeSenderSIPRTPReceiverAll NetworkMode = "SENDER_SIP_RTP__RECEIVER_SIP_RTP"

	NetworkModeReservedPlaceholder NetworkMode = "RESERVED_PLACEHOLDER"
)

var legacyNetworkModeAlias = map[NetworkMode]NetworkMode{
	"A_TO_B_SIP__B_TO_A_RTP":    NetworkModeSenderSIPReceiverRTP,
	"A_B_BIDIR_SIP__B_TO_A_RTP": NetworkModeSenderSIPReceiverSIPRTP,
	"A_B_BIDIR_SIP__BIDIR_RTP":  NetworkModeSenderSIPRTPReceiverAll,
}

type Capability struct {
	SupportsSmallRequestBody        bool `json:"supports_small_request_body"`
	SupportsLargeRequestBody        bool `json:"supports_large_request_body"`
	SupportsLargeResponseBody       bool `json:"supports_large_response_body"`
	SupportsStreamingResponse       bool `json:"supports_streaming_response"`
	SupportsBidirectionalHTTPTunnel bool `json:"supports_bidirectional_http_tunnel"`
	SupportsTransparentHTTPProxy    bool `json:"supports_transparent_http_proxy"`
}

type CapabilityItem struct {
	Key         string `json:"key"`
	Supported   bool   `json:"supported"`
	Description string `json:"description"`
}

func DefaultNetworkMode() NetworkMode {
	return NetworkModeSenderSIPReceiverRTP
}

func (m NetworkMode) Normalize() NetworkMode {
	normalized := NetworkMode(strings.ToUpper(strings.TrimSpace(string(m))))
	if normalized == "" {
		return DefaultNetworkMode()
	}
	if mapped, ok := legacyNetworkModeAlias[normalized]; ok {
		return mapped
	}
	return normalized
}

func (m NetworkMode) Validate() error {
	normalized := m.Normalize()
	if _, ok := knownNetworkModes[normalized]; ok {
		return nil
	}
	if strings.HasPrefix(string(normalized), "RESERVED_") {
		return nil
	}
	return fmt.Errorf("network.mode %q is unsupported", m)
}

func (m NetworkMode) Description() string {
	switch m.Normalize() {
	case NetworkModeSenderSIPReceiverRTP:
		return "发送端(SIP上级域): SIP --> | <-- RTP : 接收端(SIP下级域)；适合小请求 + 大响应"
	case NetworkModeSenderSIPReceiverSIPRTP:
		return "发送端(SIP上级域): SIP --> | <-- SIP&RTP : 接收端(SIP下级域)；双向小报文 + 下行大载荷"
	case NetworkModeSenderSIPRTPReceiverAll:
		return "发送端(SIP上级域): SIP&RTP --> | <-- SIP&RTP : 接收端(SIP下级域)；双向大载荷"
	default:
		if strings.HasPrefix(string(m.Normalize()), "RESERVED_") {
			return "预留网络模式：尚未定义稳定能力边界"
		}
		return "未知网络模式：能力默认降级为不支持"
	}
}

func DeriveCapability(mode NetworkMode) Capability {
	switch mode.Normalize() {
	case NetworkModeSenderSIPReceiverRTP:
		return Capability{
			SupportsSmallRequestBody:        true,
			SupportsLargeRequestBody:        false,
			SupportsLargeResponseBody:       true,
			SupportsStreamingResponse:       true,
			SupportsBidirectionalHTTPTunnel: false,
			SupportsTransparentHTTPProxy:    false,
		}
	case NetworkModeSenderSIPRTPReceiverAll:
		return Capability{
			SupportsSmallRequestBody:        true,
			SupportsLargeRequestBody:        true,
			SupportsLargeResponseBody:       true,
			SupportsStreamingResponse:       true,
			SupportsBidirectionalHTTPTunnel: true,
			SupportsTransparentHTTPProxy:    true,
		}
	case NetworkModeSenderSIPReceiverSIPRTP:
		return Capability{
			SupportsSmallRequestBody:        true,
			SupportsLargeRequestBody:        false,
			SupportsLargeResponseBody:       true,
			SupportsStreamingResponse:       true,
			SupportsBidirectionalHTTPTunnel: false,
			SupportsTransparentHTTPProxy:    false,
		}
	default:
		return Capability{}
	}
}

func (c Capability) Matrix() []CapabilityItem {
	return []CapabilityItem{
		{Key: "supports_small_request_body", Supported: c.SupportsSmallRequestBody, Description: "支持小请求体（典型 SIP 载荷范围）"},
		{Key: "supports_large_request_body", Supported: c.SupportsLargeRequestBody, Description: "支持大请求体上传"},
		{Key: "supports_large_response_body", Supported: c.SupportsLargeResponseBody, Description: "支持大响应体回传"},
		{Key: "supports_streaming_response", Supported: c.SupportsStreamingResponse, Description: "支持流式响应/分块回传"},
		{Key: "supports_bidirectional_http_tunnel", Supported: c.SupportsBidirectionalHTTPTunnel, Description: "支持双向 HTTP 隧道"},
		{Key: "supports_transparent_http_proxy", Supported: c.SupportsTransparentHTTPProxy, Description: "支持透明 HTTP 代理"},
	}
}

func (c Capability) SupportedFeatures() []string {
	matrix := c.Matrix()
	features := make([]string, 0, len(matrix))
	for _, item := range matrix {
		if item.Supported {
			features = append(features, item.Key)
		}
	}
	return features
}

func (c Capability) UnsupportedFeatures() []string {
	matrix := c.Matrix()
	features := make([]string, 0, len(matrix))
	for _, item := range matrix {
		if !item.Supported {
			features = append(features, item.Key)
		}
	}
	return features
}

var knownNetworkModes = map[NetworkMode]struct{}{
	NetworkModeSenderSIPReceiverRTP:    {},
	NetworkModeSenderSIPReceiverSIPRTP: {},
	NetworkModeSenderSIPRTPReceiverAll: {},
}
