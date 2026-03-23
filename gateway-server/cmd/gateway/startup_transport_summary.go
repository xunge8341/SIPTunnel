package main

import (
	"log"

	"siptunnel/internal/config"
	"siptunnel/internal/startupsummary"
)

func effectiveTransportTuningSummary(networkCfg config.NetworkConfig) startupsummary.TransportTuningSummary {
	convergedGeneric := config.ConvergedGenericDownloadProfile(networkCfg.TransportTuning)
	return startupsummary.TransportTuningSummary{
		UDPControlMaxBytes:                  networkCfg.TransportTuning.UDPControlMaxBytes,
		UDPCatalogMaxBytes:                  networkCfg.TransportTuning.UDPCatalogMaxBytes,
		EffectiveInlineBudgetBytes:          config.EffectiveInlineResponseBodyBudgetBytes(networkCfg),
		UDPRequestParallelismPerDevice:      networkCfg.TransportTuning.UDPRequestParallelismPerDevice,
		UDPCallbackParallelismPerPeer:       networkCfg.TransportTuning.UDPCallbackParallelismPerPeer,
		UDPBulkParallelismPerDevice:         networkCfg.TransportTuning.UDPBulkParallelismPerDevice,
		UDPSegmentParallelismPerDevice:      networkCfg.TransportTuning.UDPSegmentParallelismPerDevice,
		UDPSmallRequestMaxWaitMS:            networkCfg.TransportTuning.UDPSmallRequestMaxWaitMS,
		AdaptivePlaybackHotWindowBytes:      networkCfg.TransportTuning.AdaptivePlaybackHotWindowBytes,
		AdaptivePlaybackSegmentCacheBytes:   networkCfg.TransportTuning.AdaptivePlaybackSegmentCacheBytes,
		AdaptivePlaybackSegmentCacheTTLMS:   networkCfg.TransportTuning.AdaptivePlaybackSegmentCacheTTLMS,
		AdaptivePlaybackPrefetchSegments:    networkCfg.TransportTuning.AdaptivePlaybackPrefetchSegments,
		AdaptivePrimarySegmentAfterFails:    networkCfg.TransportTuning.AdaptivePrimarySegmentAfterFailures,
		GenericDownloadWindowBytes:          networkCfg.TransportTuning.GenericDownloadWindowBytes,
		GenericDownloadOpenEndedWindowBytes: networkCfg.TransportTuning.GenericDownloadOpenEndedWindowBytes,
		GenericDownloadSegmentConcurrency:   networkCfg.TransportTuning.GenericDownloadSegmentConcurrency,
		GenericDownloadTotalBitrateBps:      networkCfg.TransportTuning.GenericDownloadTotalBitrateBps,
		GenericDownloadMinPerTransferBps:    networkCfg.TransportTuning.GenericDownloadMinPerTransferBitrateBps,
		GenericDownloadCircuitThreshold:     networkCfg.TransportTuning.GenericDownloadCircuitFailureThreshold,
		GenericDownloadCircuitOpenMS:        networkCfg.TransportTuning.GenericDownloadCircuitOpenMS,
		GenericDownloadRTPBitrateBps:        convergedGeneric.BitrateBps,
		GenericDownloadRTPReorderWindow:     convergedGeneric.ReorderWindowPackets,
		GenericDownloadRTPLossTolerance:     convergedGeneric.LossTolerancePackets,
		GenericDownloadRTPGapTimeoutMS:      convergedGeneric.GapTimeoutMS,
		InlineResponseHeadroomRatio:         networkCfg.TransportTuning.InlineResponseHeadroomRatio,
		ResponseModePolicy:                  config.EffectiveResponseModePolicyLabel(networkCfg),
	}
}

func logAppliedTransportTuning(networkCfg config.NetworkConfig) {
	convergedGeneric := config.ConvergedGenericDownloadProfile(networkCfg.TransportTuning)
	log.Printf(
		"transport tuning effective response_mode_policy=%s inline_response_headroom_ratio=%.2f udp_small_request_max_wait_ms=%d udp_segment_parallelism_per_device=%d adaptive_playback_hot_window_bytes=%d adaptive_playback_segment_cache_bytes=%d adaptive_playback_segment_cache_ttl_ms=%d adaptive_playback_prefetch_segments=%d adaptive_primary_segment_after_failures=%d effective_inline_budget_bytes=%d generic_download_payload_bytes=%d generic_download_rtp_bitrate_bps=%d generic_download_rtp_socket_buffer_bytes=%d generic_download_rtp_reorder_window_packets=%d generic_download_rtp_loss_tolerance_packets=%d generic_download_rtp_gap_timeout_ms=%d generic_download_rtp_fec_enabled=%t generic_download_rtp_fec_group_packets=%d generic_download_guard=%s",
		config.EffectiveResponseModePolicyLabel(networkCfg),
		networkCfg.TransportTuning.InlineResponseHeadroomRatio,
		networkCfg.TransportTuning.UDPSmallRequestMaxWaitMS,
		networkCfg.TransportTuning.UDPSegmentParallelismPerDevice,
		networkCfg.TransportTuning.AdaptivePlaybackHotWindowBytes,
		networkCfg.TransportTuning.AdaptivePlaybackSegmentCacheBytes,
		networkCfg.TransportTuning.AdaptivePlaybackSegmentCacheTTLMS,
		networkCfg.TransportTuning.AdaptivePlaybackPrefetchSegments,
		networkCfg.TransportTuning.AdaptivePrimarySegmentAfterFailures,
		config.EffectiveInlineResponseBodyBudgetBytes(networkCfg),
		convergedGeneric.PayloadBytes,
		convergedGeneric.BitrateBps,
		convergedGeneric.SocketBufferBytes,
		convergedGeneric.ReorderWindowPackets,
		convergedGeneric.LossTolerancePackets,
		convergedGeneric.GapTimeoutMS,
		convergedGeneric.FECEnabled,
		convergedGeneric.FECGroupPackets,
		config.GenericDownloadConvergenceSummary(networkCfg.TransportTuning),
	)
}
