package config

import (
	"strings"
	"testing"
)

func TestBuildEffectiveStrategySnapshot(t *testing.T) {
	cfg := DefaultNetworkConfig()
	cfg.SIP.Transport = "UDP"
	snapshot := BuildEffectiveStrategySnapshot(cfg)
	checks := []struct {
		name string
		got  string
		want string
	}{
		{name: "response policy", got: snapshot.ResponseModePolicy, want: "AUTO(budget_driven_inline_or_rtp)"},
		{name: "delivery family", got: snapshot.LargeResponseDeliveryFamily, want: "adaptive_segmented_primary"},
		{name: "selector", got: snapshot.SegmentedProfileSelector, want: "explicit_child>generic-rtp>boundary-rtp>boundary-http>standard-http"},
		{name: "entry policy", got: snapshot.EntrySelectionPolicy, want: "path_clean(dedupe_slash+dot_segment)=>normalized_path"},
		{name: "udp control header policy", got: snapshot.UDPControlHeaderPolicy, want: "severe_budget_rescue(cookie64_or_auth_only)"},
		{name: "generic tolerance", got: snapshot.GenericDownloadRTPTolerance, want: "generic_download(reorder=512 loss=192 gap_timeout_ms=900 fec=true/8)"},
	}
	for _, tc := range checks {
		if !strings.Contains(tc.got, tc.want) {
			t.Fatalf("%s=%q, want contains %q", tc.name, tc.got, tc.want)
		}
	}
}
