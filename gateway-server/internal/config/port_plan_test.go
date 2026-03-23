package config

import "testing"

func TestRecommendedInlineBodyBudgetBytes(t *testing.T) {
	got := RecommendedInlineBodyBudgetBytes(DefaultTransportTuningConfig())
	if got != 153 {
		t.Fatalf("RecommendedInlineBodyBudgetBytes=%d, want 153", got)
	}
}

func TestRTPPortPoolValidationRejectsPoolSmallerThanInflight(t *testing.T) {
	cfg := DefaultNetworkConfig()
	cfg.RTP.PortStart = 30000
	cfg.RTP.PortEnd = 30015
	cfg.RTP.MaxInflightTransfers = 64
	if err := cfg.RTP.Validate(); err == nil {
		t.Fatal("expected port pool validation error")
	}
}

func TestRecommendedRTPReorderBufferBytes(t *testing.T) {
	cfg := DefaultTransportTuningConfig()
	want := int64(cfg.BoundaryRTPPayloadBytes) * int64(cfg.BoundaryRTPReorderWindowPackets+cfg.BoundaryRTPLossTolerancePackets)
	got := RecommendedRTPReorderBufferBytes(cfg)
	if got != want {
		t.Fatalf("RecommendedRTPReorderBufferBytes=%d, want %d", got, want)
	}
}

func TestRecommendedPlaybackRTPReorderBufferBytes(t *testing.T) {
	cfg := DefaultTransportTuningConfig()
	want := int64(cfg.BoundaryRTPPayloadBytes) * int64(cfg.BoundaryPlaybackRTPReorderWindowPackets+cfg.BoundaryPlaybackRTPLossTolerancePackets)
	got := RecommendedPlaybackRTPReorderBufferBytes(cfg)
	if got != want {
		t.Fatalf("RecommendedPlaybackRTPReorderBufferBytes=%d, want %d", got, want)
	}
}
