package config

import (
	"errors"
	"fmt"
)

func (c TransportTuningConfig) Validate() error {
	var errs []error
	if c.UDPControlMaxBytes < 576 {
		errs = append(errs, fmt.Errorf("transport_tuning.udp_control_max_bytes %d must be >= 576", c.UDPControlMaxBytes))
	}
	if c.UDPCatalogMaxBytes < 576 {
		errs = append(errs, fmt.Errorf("transport_tuning.udp_catalog_max_bytes %d must be >= 576", c.UDPCatalogMaxBytes))
	}
	if c.InlineResponseUDPBudgetBytes < 576 {
		errs = append(errs, fmt.Errorf("transport_tuning.inline_response_udp_budget_bytes %d must be >= 576", c.InlineResponseUDPBudgetBytes))
	}
	if c.InlineResponseSafetyReserveBytes < 0 {
		errs = append(errs, fmt.Errorf("transport_tuning.inline_response_safety_reserve_bytes %d must be >= 0", c.InlineResponseSafetyReserveBytes))
	}
	if c.InlineResponseEnvelopeOverheadBytes < 0 {
		errs = append(errs, fmt.Errorf("transport_tuning.inline_response_envelope_overhead_bytes %d must be >= 0", c.InlineResponseEnvelopeOverheadBytes))
	}
	if c.InlineResponseHeadroomRatio < 0 || c.InlineResponseHeadroomRatio > 0.50 {
		errs = append(errs, fmt.Errorf("transport_tuning.inline_response_headroom_ratio %.4f out of range [0,0.50]", c.InlineResponseHeadroomRatio))
	}
	if c.InlineResponseHeadroomPercent < 0 || c.InlineResponseHeadroomPercent > 50 {
		errs = append(errs, fmt.Errorf("transport_tuning.inline_response_headroom_percent %d out of range [0,50]", c.InlineResponseHeadroomPercent))
	}
	if c.UDPSmallRequestMaxWaitMS < 100 || c.UDPSmallRequestMaxWaitMS > 30000 {
		errs = append(errs, fmt.Errorf("transport_tuning.udp_small_request_max_wait_ms %d out of range [100,30000]", c.UDPSmallRequestMaxWaitMS))
	}
	if c.UDPSegmentParallelismPerDevice <= 0 || c.UDPSegmentParallelismPerDevice > 128 {
		errs = append(errs, fmt.Errorf("transport_tuning.udp_segment_parallelism_per_device %d out of range [1,128]", c.UDPSegmentParallelismPerDevice))
	}
	if c.AdaptivePlaybackHotWindowBytes < 1<<20 || c.AdaptivePlaybackHotWindowBytes > 32<<20 {
		errs = append(errs, fmt.Errorf("transport_tuning.adaptive_playback_hot_window_bytes %d out of range [%d,%d]", c.AdaptivePlaybackHotWindowBytes, 1<<20, 32<<20))
	}
	if c.AdaptivePlaybackSegmentCacheBytes < 32<<20 || c.AdaptivePlaybackSegmentCacheBytes > 1<<30 {
		errs = append(errs, fmt.Errorf("transport_tuning.adaptive_playback_segment_cache_bytes %d out of range [%d,%d]", c.AdaptivePlaybackSegmentCacheBytes, 32<<20, 1<<30))
	}
	if c.AdaptivePlaybackSegmentCacheTTLMS < 1000 || c.AdaptivePlaybackSegmentCacheTTLMS > 300000 {
		errs = append(errs, fmt.Errorf("transport_tuning.adaptive_playback_segment_cache_ttl_ms %d out of range [1000,300000]", c.AdaptivePlaybackSegmentCacheTTLMS))
	}
	if c.AdaptivePlaybackPrefetchSegments < 0 || c.AdaptivePlaybackPrefetchSegments > 8 {
		errs = append(errs, fmt.Errorf("transport_tuning.adaptive_playback_prefetch_segments %d out of range [0,8]", c.AdaptivePlaybackPrefetchSegments))
	}
	if c.AdaptivePrimarySegmentAfterFailures < 1 || c.AdaptivePrimarySegmentAfterFailures > 8 {
		errs = append(errs, fmt.Errorf("transport_tuning.adaptive_primary_segment_after_failures %d out of range [1,8]", c.AdaptivePrimarySegmentAfterFailures))
	}
	if c.UDPRequestParallelismPerDevice <= 0 || c.UDPCallbackParallelismPerPeer <= 0 || c.UDPBulkParallelismPerDevice <= 0 {
		errs = append(errs, errors.New("transport_tuning.udp parallelism settings must be > 0"))
	}
	if c.GenericSegmentedPrimaryThresholdBytes < 1<<20 || c.GenericSegmentedPrimaryThresholdBytes > 1<<30 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_segmented_primary_threshold_bytes %d out of range [%d,%d]", c.GenericSegmentedPrimaryThresholdBytes, 1<<20, 1<<30))
	}
	if c.GenericDownloadWindowBytes < 1<<20 || c.GenericDownloadWindowBytes > 64<<20 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_window_bytes %d out of range [%d,%d]", c.GenericDownloadWindowBytes, 1<<20, 64<<20))
	}
	if c.GenericDownloadOpenEndedWindowBytes < 2<<20 || c.GenericDownloadOpenEndedWindowBytes > 64<<20 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_open_ended_window_bytes %d out of range [%d,%d]", c.GenericDownloadOpenEndedWindowBytes, 2<<20, 64<<20))
	}
	if c.GenericPrefetchSegments < 0 || c.GenericPrefetchSegments > 2 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_prefetch_segments %d out of range [0,2]", c.GenericPrefetchSegments))
	}
	if c.GenericDownloadSegmentConcurrency < 1 || c.GenericDownloadSegmentConcurrency > 8 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_segment_concurrency %d out of range [1,8]", c.GenericDownloadSegmentConcurrency))
	}
	if c.GenericDownloadSegmentRetries < 0 || c.GenericDownloadSegmentRetries > 6 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_segment_retries %d out of range [0,6]", c.GenericDownloadSegmentRetries))
	}
	if c.GenericDownloadResumeMaxAttempts < 1 || c.GenericDownloadResumeMaxAttempts > 12 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_resume_max_attempts %d out of range [1,12]", c.GenericDownloadResumeMaxAttempts))
	}
	if c.GenericDownloadResumePerRangeRetries < 1 || c.GenericDownloadResumePerRangeRetries > 6 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_resume_per_range_retries %d out of range [1,6]", c.GenericDownloadResumePerRangeRetries))
	}
	if c.GenericDownloadPenaltyWaitMS < 0 || c.GenericDownloadPenaltyWaitMS > 5000 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_penalty_wait_ms %d out of range [0,5000]", c.GenericDownloadPenaltyWaitMS))
	}
	if c.GenericDownloadTotalBitrateBps < 4*1024*1024 || c.GenericDownloadTotalBitrateBps > 128*1024*1024 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_total_bitrate_bps %d out of range [%d,%d]", c.GenericDownloadTotalBitrateBps, 4*1024*1024, 128*1024*1024))
	}
	if c.GenericDownloadMinPerTransferBitrateBps < 1*1024*1024 || c.GenericDownloadMinPerTransferBitrateBps > 32*1024*1024 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_min_per_transfer_bitrate_bps %d out of range [%d,%d]", c.GenericDownloadMinPerTransferBitrateBps, 1*1024*1024, 32*1024*1024))
	}
	if c.GenericDownloadCircuitFailureThreshold < 1 || c.GenericDownloadCircuitFailureThreshold > 10 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_circuit_failure_threshold %d out of range [1,10]", c.GenericDownloadCircuitFailureThreshold))
	}
	if c.GenericDownloadCircuitOpenMS < 1000 || c.GenericDownloadCircuitOpenMS > 300000 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_circuit_open_ms %d out of range [1000,300000]", c.GenericDownloadCircuitOpenMS))
	}
	if c.GenericDownloadRTPBitrateBps < 2*1024*1024 || c.GenericDownloadRTPBitrateBps > 64*1024*1024 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_rtp_bitrate_bps %d out of range [%d,%d]", c.GenericDownloadRTPBitrateBps, 2*1024*1024, 64*1024*1024))
	}
	if c.GenericDownloadRTPMinSpacingUS < 100 || c.GenericDownloadRTPMinSpacingUS > 10000 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_rtp_min_spacing_us %d out of range [100,10000]", c.GenericDownloadRTPMinSpacingUS))
	}
	if c.GenericDownloadRTPSocketBufferBytes < 1<<20 || c.GenericDownloadRTPSocketBufferBytes > 64<<20 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_rtp_socket_buffer_bytes %d out of range [%d,%d]", c.GenericDownloadRTPSocketBufferBytes, 1<<20, 64<<20))
	}
	if c.GenericDownloadRTPReorderWindowPackets < 16 || c.GenericDownloadRTPReorderWindowPackets > 1024 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_rtp_reorder_window_packets %d out of range [16,1024]", c.GenericDownloadRTPReorderWindowPackets))
	}
	if c.GenericDownloadRTPLossTolerancePackets < 8 || c.GenericDownloadRTPLossTolerancePackets > 512 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_rtp_loss_tolerance_packets %d out of range [8,512]", c.GenericDownloadRTPLossTolerancePackets))
	}
	if c.GenericDownloadRTPGapTimeoutMS < 100 || c.GenericDownloadRTPGapTimeoutMS > 5000 {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_rtp_gap_timeout_ms %d out of range [100,5000]", c.GenericDownloadRTPGapTimeoutMS))
	}
	if c.GenericDownloadRTPFECGroupPackets != 0 && (c.GenericDownloadRTPFECGroupPackets < 2 || c.GenericDownloadRTPFECGroupPackets > 32) {
		errs = append(errs, fmt.Errorf("transport_tuning.generic_download_rtp_fec_group_packets %d out of range {0}∪[2,32]", c.GenericDownloadRTPFECGroupPackets))
	}
	if c.BoundaryRTPPayloadBytes < 256 || c.BoundaryRTPPayloadBytes > 1400 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_rtp_payload_bytes %d out of range [256,1400]", c.BoundaryRTPPayloadBytes))
	}
	if c.BoundaryRTPBitrateBps < 2*1024*1024 || c.BoundaryRTPBitrateBps > 64*1024*1024 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_rtp_bitrate_bps %d out of range [%d,%d]", c.BoundaryRTPBitrateBps, 2*1024*1024, 64*1024*1024))
	}
	if c.BoundaryRTPMinSpacingUS < 100 || c.BoundaryRTPMinSpacingUS > 5000 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_rtp_min_spacing_us %d out of range [100,5000]", c.BoundaryRTPMinSpacingUS))
	}
	if c.BoundaryRTPSocketBufferBytes < 1<<20 || c.BoundaryRTPSocketBufferBytes > 64<<20 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_rtp_socket_buffer_bytes %d out of range [%d,%d]", c.BoundaryRTPSocketBufferBytes, 1<<20, 64<<20))
	}
	if c.BoundaryRTPReorderWindowPackets < 1 || c.BoundaryRTPReorderWindowPackets > 512 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_rtp_reorder_window_packets %d out of range [1,512]", c.BoundaryRTPReorderWindowPackets))
	}
	if c.BoundaryRTPLossTolerancePackets < 0 || c.BoundaryRTPLossTolerancePackets > 256 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_rtp_loss_tolerance_packets %d out of range [0,256]", c.BoundaryRTPLossTolerancePackets))
	}
	if c.BoundaryRTPGapTimeoutMS < 100 || c.BoundaryRTPGapTimeoutMS > 30000 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_rtp_gap_timeout_ms %d out of range [100,30000]", c.BoundaryRTPGapTimeoutMS))
	}
	if c.BoundaryRTPFECGroupPackets != 0 && (c.BoundaryRTPFECGroupPackets < 2 || c.BoundaryRTPFECGroupPackets > 32) {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_rtp_fec_group_packets %d out of range {0}∪[2,32]", c.BoundaryRTPFECGroupPackets))
	}
	if c.BoundaryPlaybackRTPReorderWindowPackets < 1 || c.BoundaryPlaybackRTPReorderWindowPackets > 512 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_playback_rtp_reorder_window_packets %d out of range [1,512]", c.BoundaryPlaybackRTPReorderWindowPackets))
	}
	if c.BoundaryPlaybackRTPLossTolerancePackets < 0 || c.BoundaryPlaybackRTPLossTolerancePackets > 256 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_playback_rtp_loss_tolerance_packets %d out of range [0,256]", c.BoundaryPlaybackRTPLossTolerancePackets))
	}
	if c.BoundaryPlaybackRTPGapTimeoutMS < 100 || c.BoundaryPlaybackRTPGapTimeoutMS > 30000 {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_playback_rtp_gap_timeout_ms %d out of range [100,30000]", c.BoundaryPlaybackRTPGapTimeoutMS))
	}
	if c.BoundaryPlaybackRTPFECGroupPackets != 0 && (c.BoundaryPlaybackRTPFECGroupPackets < 2 || c.BoundaryPlaybackRTPFECGroupPackets > 32) {
		errs = append(errs, fmt.Errorf("transport_tuning.boundary_playback_rtp_fec_group_packets %d out of range {0}∪[2,32]", c.BoundaryPlaybackRTPFECGroupPackets))
	}
	if c.BoundaryFixedWindowBytes <= 0 || c.BoundaryFixedWindowThreshold <= 0 {
		errs = append(errs, errors.New("transport_tuning.boundary fixed window bytes/threshold must be > 0"))
	}
	if c.BoundaryHTTPWindowBytes <= 0 || c.BoundaryHTTPWindowThreshold <= 0 {
		errs = append(errs, errors.New("transport_tuning.boundary http window bytes/threshold must be > 0"))
	}
	if c.StandardWindowBytes <= 0 || c.StandardWindowThreshold <= 0 {
		errs = append(errs, errors.New("transport_tuning.standard window bytes/threshold must be > 0"))
	}
	if c.BoundarySegmentConcurrency < 1 || c.BoundarySegmentConcurrency > 16 || c.BoundaryHTTPSegmentConcurrency < 1 || c.BoundaryHTTPSegmentConcurrency > 16 || c.StandardSegmentConcurrency < 1 || c.StandardSegmentConcurrency > 16 {
		errs = append(errs, errors.New("transport_tuning.segment_concurrency out of range [1,16]"))
	}
	if c.BoundarySegmentRetries < 0 || c.BoundarySegmentRetries > 4 || c.BoundaryHTTPSegmentRetries < 0 || c.BoundaryHTTPSegmentRetries > 4 || c.StandardSegmentRetries < 0 || c.StandardSegmentRetries > 4 {
		errs = append(errs, errors.New("transport_tuning.segment_retries out of range [0,4]"))
	}
	if c.BoundaryResumeMaxAttempts < 1 || c.BoundaryResumeMaxAttempts > 8 || c.BoundaryResumePerRangeRetries < 0 || c.BoundaryResumePerRangeRetries > 4 {
		errs = append(errs, errors.New("transport_tuning.boundary resume attempts/retries out of range"))
	}
	if c.BoundaryResponseStartWaitMS < 1000 || c.BoundaryResponseStartWaitMS > 120000 || c.BoundaryRangeResponseWaitMS < 1000 || c.BoundaryRangeResponseWaitMS > 120000 {
		errs = append(errs, errors.New("transport_tuning.boundary response wait ms out of range [1000,120000]"))
	}
	if bodyBudget := RecommendedInlineBodyBudgetBytes(c); bodyBudget < 128 {
		errs = append(errs, fmt.Errorf("transport_tuning inline body budget too small after reserve/headroom calculation: %d bytes", bodyBudget))
	}
	if reorderBudget := RecommendedRTPReorderBufferBytes(c); reorderBudget <= 0 {
		errs = append(errs, fmt.Errorf("transport_tuning rtp reorder buffer budget invalid: %d", reorderBudget))
	}
	return errors.Join(errs...)
}
