package config

import "fmt"

type EffectiveStrategySnapshot struct {
	ResponseModePolicy            string `json:"response_mode_policy"`
	LargeResponseDeliveryFamily   string `json:"large_response_delivery_family"`
	SegmentedProfileSelector      string `json:"segmented_profile_selector"`
	EntrySelectionPolicy          string `json:"entry_selection_policy"`
	UDPControlHeaderPolicy        string `json:"udp_control_header_policy"`
	BoundaryRTPSendProfile        string `json:"boundary_rtp_send_profile"`
	BoundaryRTPToleranceProfile   string `json:"boundary_rtp_tolerance_profile"`
	PlaybackRTPToleranceProfile   string `json:"playback_rtp_tolerance_profile"`
	GenericDownloadRTPSendProfile string `json:"generic_download_rtp_send_profile"`
	GenericDownloadRTPTolerance   string `json:"generic_download_rtp_tolerance_profile"`
	GenericDownloadCircuitPolicy  string `json:"generic_download_circuit_policy"`
	GenericDownloadGuardPolicy    string `json:"generic_download_guard_policy"`
}

func BuildEffectiveStrategySnapshot(cfg NetworkConfig) EffectiveStrategySnapshot {
	t := cfg.TransportTuning
	return EffectiveStrategySnapshot{
		ResponseModePolicy:          EffectiveResponseModePolicyLabel(cfg),
		LargeResponseDeliveryFamily: "stream_primary|range_primary|adaptive_segmented_primary|fallback_segmented",
		SegmentedProfileSelector:    "explicit_child>generic-rtp>boundary-rtp>boundary-http>standard-http",
		EntrySelectionPolicy:        "path_clean(dedupe_slash+dot_segment)=>normalized_path; root_or_default_document=>mapped_base; mapped_base/default_document=>mapped_base; mapped_base/*=>suffix_forward",
		UDPControlHeaderPolicy:      "selected_headers(content-type,auth,cookie,validators,range)>oversize_budget_rescue(cookie_trim+content_type_compact+login_validator_drop+bodyless_content_type_drop)>severe_budget_rescue(cookie64_or_auth_only)",
		BoundaryRTPSendProfile: fmt.Sprintf(
			"boundary-rtp(payload=%d bitrate_bps=%d spacing_us=%d socket_buffer_bytes=%d fec=%t/%d)",
			t.BoundaryRTPPayloadBytes,
			t.BoundaryRTPBitrateBps,
			t.BoundaryRTPMinSpacingUS,
			t.BoundaryRTPSocketBufferBytes,
			t.BoundaryRTPFECEnabled,
			t.BoundaryRTPFECGroupPackets,
		),
		BoundaryRTPToleranceProfile: fmt.Sprintf(
			"boundary(reorder=%d loss=%d gap_timeout_ms=%d fec=%t/%d)",
			t.BoundaryRTPReorderWindowPackets,
			t.BoundaryRTPLossTolerancePackets,
			t.BoundaryRTPGapTimeoutMS,
			t.BoundaryRTPFECEnabled,
			t.BoundaryRTPFECGroupPackets,
		),
		PlaybackRTPToleranceProfile: fmt.Sprintf(
			"range_playback(reorder=%d loss=%d gap_timeout_ms=%d fec=%t/%d)",
			t.BoundaryPlaybackRTPReorderWindowPackets,
			t.BoundaryPlaybackRTPLossTolerancePackets,
			t.BoundaryPlaybackRTPGapTimeoutMS,
			t.BoundaryPlaybackRTPFECEnabled,
			t.BoundaryPlaybackRTPFECGroupPackets,
		),
		GenericDownloadRTPSendProfile: fmt.Sprintf(
			"generic-rtp(payload=%d bitrate_bps=%d socket_buffer_bytes=%d fec=%t/%d)",
			ConvergedGenericDownloadProfile(t).PayloadBytes,
			ConvergedGenericDownloadProfile(t).BitrateBps,
			ConvergedGenericDownloadProfile(t).SocketBufferBytes,
			ConvergedGenericDownloadProfile(t).FECEnabled,
			ConvergedGenericDownloadProfile(t).FECGroupPackets,
		),
		GenericDownloadRTPTolerance: fmt.Sprintf(
			"generic_download(reorder=%d loss=%d gap_timeout_ms=%d fec=%t/%d)",
			ConvergedGenericDownloadProfile(t).ReorderWindowPackets,
			ConvergedGenericDownloadProfile(t).LossTolerancePackets,
			ConvergedGenericDownloadProfile(t).GapTimeoutMS,
			ConvergedGenericDownloadProfile(t).FECEnabled,
			ConvergedGenericDownloadProfile(t).FECGroupPackets,
		),
		GenericDownloadCircuitPolicy: fmt.Sprintf(
			"threshold=%d open_ms=%d severe_media_failure_threshold=1",
			t.GenericDownloadCircuitFailureThreshold,
			t.GenericDownloadCircuitOpenMS,
		),
		GenericDownloadGuardPolicy: GenericDownloadConvergenceSummary(t),
	}
}
