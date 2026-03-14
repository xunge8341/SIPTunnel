package config

import "testing"

func TestResolveTransportPlan(t *testing.T) {
	tests := []struct {
		name string
		mode NetworkMode
		want TunnelTransportPlan
	}{
		{
			name: "A_TO_B_SIP__B_TO_A_RTP derives sip request and rtp response",
			mode: NetworkModeAToBSIPBToARTP,
			want: TunnelTransportPlan{
				RequestMetaTransport:  TransportSIPControl,
				RequestBodyTransport:  TransportSIPBodyOnly,
				ResponseMetaTransport: TransportSIPControl,
				ResponseBodyTransport: TransportRTPStream,
				RequestBodySizeLimit:  DefaultNetworkConfig().SIP.MaxMessageBytes,
				ResponseBodySizeLimit: UnlimitedBodySizeLimit,
			},
		},
		{
			name: "A_B_BIDIR_SIP__BIDIR_RTP derives full duplex large body plan",
			mode: NetworkModeABBiDirSIPBiDirRTP,
			want: TunnelTransportPlan{
				RequestMetaTransport:  TransportSIPControl,
				RequestBodyTransport:  TransportSIPOrRTPAuto,
				ResponseMetaTransport: TransportSIPControl,
				ResponseBodyTransport: TransportRTPStream,
				RequestBodySizeLimit:  UnlimitedBodySizeLimit,
				ResponseBodySizeLimit: UnlimitedBodySizeLimit,
			},
		},
		{
			name: "A_B_BIDIR_SIP__B_TO_A_RTP keeps request large upload disabled",
			mode: NetworkModeABBiDirSIPBToARTP,
			want: TunnelTransportPlan{
				RequestMetaTransport:  TransportSIPControl,
				RequestBodyTransport:  TransportSIPBodyOnly,
				ResponseMetaTransport: TransportSIPControl,
				ResponseBodyTransport: TransportRTPStream,
				RequestBodySizeLimit:  DefaultNetworkConfig().SIP.MaxMessageBytes,
				ResponseBodySizeLimit: UnlimitedBodySizeLimit,
			},
		},
		{
			name: "unknown mode degrades to minimal control-only plan",
			mode: NetworkMode("RESERVED_FUTURE"),
			want: TunnelTransportPlan{
				RequestMetaTransport:  TransportSIPControl,
				RequestBodyTransport:  TransportNone,
				ResponseMetaTransport: TransportSIPControl,
				ResponseBodyTransport: TransportNone,
				RequestBodySizeLimit:  0,
				ResponseBodySizeLimit: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capability := DeriveCapability(tt.mode)
			got := ResolveTransportPlan(tt.mode, capability)
			if got.RequestMetaTransport != tt.want.RequestMetaTransport ||
				got.RequestBodyTransport != tt.want.RequestBodyTransport ||
				got.ResponseMetaTransport != tt.want.ResponseMetaTransport ||
				got.ResponseBodyTransport != tt.want.ResponseBodyTransport ||
				got.RequestBodySizeLimit != tt.want.RequestBodySizeLimit ||
				got.ResponseBodySizeLimit != tt.want.ResponseBodySizeLimit {
				t.Fatalf("ResolveTransportPlan(%s)=%+v, want core=%+v", tt.mode, got, tt.want)
			}
			if len(got.Notes) == 0 {
				t.Fatalf("expected notes for %s", tt.mode)
			}
		})
	}
}
