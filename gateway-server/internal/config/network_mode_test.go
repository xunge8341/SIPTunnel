package config

import (
	"reflect"
	"testing"
)

func TestDeriveCapability(t *testing.T) {
	tests := []struct {
		name string
		mode NetworkMode
		want Capability
	}{
		{
			name: "restricted sip one-way rtp",
			mode: NetworkModeSenderSIPReceiverRTP,
			want: Capability{
				SupportsSmallRequestBody:        true,
				SupportsLargeRequestBody:        false,
				SupportsLargeResponseBody:       true,
				SupportsStreamingResponse:       true,
				SupportsBidirectionalHTTPTunnel: false,
				SupportsTransparentHTTPProxy:    false,
			},
		},
		{
			name: "full bidirectional",
			mode: NetworkModeSenderSIPRTPReceiverAll,
			want: Capability{
				SupportsSmallRequestBody:        true,
				SupportsLargeRequestBody:        true,
				SupportsLargeResponseBody:       true,
				SupportsStreamingResponse:       true,
				SupportsBidirectionalHTTPTunnel: true,
				SupportsTransparentHTTPProxy:    true,
			},
		},
		{
			name: "bidirectional sip but limited rtp",
			mode: NetworkModeSenderSIPReceiverSIPRTP,
			want: Capability{
				SupportsSmallRequestBody:        true,
				SupportsLargeRequestBody:        false,
				SupportsLargeResponseBody:       true,
				SupportsStreamingResponse:       true,
				SupportsBidirectionalHTTPTunnel: false,
				SupportsTransparentHTTPProxy:    false,
			},
		},
		{
			name: "reserved mode downgraded",
			mode: NetworkMode("RESERVED_FUTURE_MODE"),
			want: Capability{},
		},
		{
			name: "unknown mode downgraded",
			mode: NetworkMode("UNKNOWN_MODE"),
			want: Capability{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeriveCapability(tt.mode)
			if got != tt.want {
				t.Fatalf("DeriveCapability(%s)=%+v, want %+v", tt.mode, got, tt.want)
			}
		})
	}
}

func TestNetworkModeValidate(t *testing.T) {
	if err := NetworkModeSenderSIPReceiverRTP.Validate(); err != nil {
		t.Fatalf("known mode should be valid: %v", err)
	}
	if err := NetworkMode("reserved_custom").Validate(); err != nil {
		t.Fatalf("reserved mode should be accepted: %v", err)
	}
	if err := NetworkMode("unknown").Validate(); err == nil {
		t.Fatal("unknown mode should be rejected")
	}
}

func TestCapabilityHelpers(t *testing.T) {
	capability := DeriveCapability(NetworkModeSenderSIPReceiverRTP)
	if len(capability.Matrix()) != 6 {
		t.Fatalf("matrix size=%d, want 6", len(capability.Matrix()))
	}
	if !reflect.DeepEqual(capability.SupportedFeatures(), []string{"supports_small_request_body", "supports_large_response_body", "supports_streaming_response"}) {
		t.Fatalf("unexpected supported features: %+v", capability.SupportedFeatures())
	}
	if !reflect.DeepEqual(capability.UnsupportedFeatures(), []string{"supports_large_request_body", "supports_bidirectional_http_tunnel", "supports_transparent_http_proxy"}) {
		t.Fatalf("unexpected unsupported features: %+v", capability.UnsupportedFeatures())
	}
}

func TestNetworkModeNormalizeLegacyAlias(t *testing.T) {
	if got := NetworkMode("A_TO_B_SIP__B_TO_A_RTP").Normalize(); got != NetworkModeSenderSIPReceiverRTP {
		t.Fatalf("legacy alias normalize got %s", got)
	}
}
