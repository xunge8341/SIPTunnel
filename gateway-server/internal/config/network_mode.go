package config

import (
	"fmt"
	"strings"
)

type NetworkMode string

const (
	NetworkModeAToBSIPBToARTP      NetworkMode = "A_TO_B_SIP__B_TO_A_RTP"
	NetworkModeABBiDirSIPBiDirRTP  NetworkMode = "A_B_BIDIR_SIP__BIDIR_RTP"
	NetworkModeABBiDirSIPBToARTP   NetworkMode = "A_B_BIDIR_SIP__B_TO_A_RTP"
	NetworkModeReservedPlaceholder NetworkMode = "RESERVED_PLACEHOLDER"
)

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
	return NetworkModeAToBSIPBToARTP
}

func (m NetworkMode) Normalize() NetworkMode {
	normalized := strings.ToUpper(strings.TrimSpace(string(m)))
	if normalized == "" {
		return DefaultNetworkMode()
	}
	return NetworkMode(normalized)
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
	case NetworkModeAToBSIPBToARTP:
		return "A->B 仅 SIP 小报文，B->A RTP 回传大载荷；适合小请求 + 大响应"
	case NetworkModeABBiDirSIPBiDirRTP:
		return "A/B 双向 SIP + 双向 RTP；可支持双向大载荷与透明代理类能力"
	case NetworkModeABBiDirSIPBToARTP:
		return "A/B 双向 SIP，但 RTP 仅 B->A；支持双向小请求，受限于上行大载荷"
	default:
		if strings.HasPrefix(string(m.Normalize()), "RESERVED_") {
			return "预留网络模式：尚未定义稳定能力边界"
		}
		return "未知网络模式：能力默认降级为不支持"
	}
}

func DeriveCapability(mode NetworkMode) Capability {
	switch mode.Normalize() {
	case NetworkModeAToBSIPBToARTP:
		return Capability{
			SupportsSmallRequestBody:        true,
			SupportsLargeRequestBody:        false,
			SupportsLargeResponseBody:       true,
			SupportsStreamingResponse:       true,
			SupportsBidirectionalHTTPTunnel: false,
			SupportsTransparentHTTPProxy:    false,
		}
	case NetworkModeABBiDirSIPBiDirRTP:
		return Capability{
			SupportsSmallRequestBody:        true,
			SupportsLargeRequestBody:        true,
			SupportsLargeResponseBody:       true,
			SupportsStreamingResponse:       true,
			SupportsBidirectionalHTTPTunnel: true,
			SupportsTransparentHTTPProxy:    true,
		}
	case NetworkModeABBiDirSIPBToARTP:
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
	NetworkModeAToBSIPBToARTP:     {},
	NetworkModeABBiDirSIPBiDirRTP: {},
	NetworkModeABBiDirSIPBToARTP:  {},
}
