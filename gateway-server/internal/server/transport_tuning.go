package server

import (
	"sync/atomic"
	"time"

	"siptunnel/internal/config"
)

var activeTransportTuning atomic.Value

func init() {
	activeTransportTuning.Store(config.DefaultTransportTuningConfig())
}

func ApplyTransportTuning(cfg config.TransportTuningConfig) {
	defaults := config.DefaultTransportTuningConfig()
	cfg.ApplyDefaultsForRuntime(defaults)
	activeTransportTuning.Store(cfg)
}

func currentTransportTuning() config.TransportTuningConfig {
	if v := activeTransportTuning.Load(); v != nil {
		if cfg, ok := v.(config.TransportTuningConfig); ok {
			return cfg
		}
	}
	return config.DefaultTransportTuningConfig()
}

func udpControlMaxBytes() int {
	return currentTransportTuning().UDPControlMaxBytes
}

func udpCatalogMaxBytes() int {
	cfg := currentTransportTuning()
	if cfg.UDPCatalogMaxBytes > 0 {
		return cfg.UDPCatalogMaxBytes
	}
	return cfg.UDPControlMaxBytes
}

func boundaryRTPPayloadBytes() int {
	return currentTransportTuning().BoundaryRTPPayloadBytes
}

func boundaryRTPBitrate() int64 {
	return int64(currentTransportTuning().BoundaryRTPBitrateBps)
}

func boundaryRTPMinSpacing() time.Duration {
	return time.Duration(currentTransportTuning().BoundaryRTPMinSpacingUS) * time.Microsecond
}

func boundaryRTPSocketBufferBytes() int {
	return currentTransportTuning().BoundaryRTPSocketBufferBytes
}

func genericSegmentedPrimaryThresholdBytes() int64 {
	return currentTransportTuning().GenericSegmentedPrimaryThresholdBytes
}

func genericPrefetchSegments() int {
	return currentTransportTuning().GenericPrefetchSegments
}

func genericDownloadWindowBytes() int64 {
	return currentTransportTuning().GenericDownloadWindowBytes
}

func genericDownloadOpenEndedWindowBytes() int64 {
	return currentTransportTuning().GenericDownloadOpenEndedWindowBytes
}

func genericDownloadSegmentConcurrency() int {
	return currentTransportTuning().GenericDownloadSegmentConcurrency
}

func genericDownloadSameTransferSplitEnabled() bool {
	return currentTransportTuning().GenericDownloadSameTransferSplitEnabled
}

func genericDownloadSourceConstrainedAutoSingleflightEnabled() bool {
	return currentTransportTuning().GenericDownloadSourceConstrainedAutoSingleflightEnabled
}
func genericDownloadResumeMaxAttempts() int {
	return currentTransportTuning().GenericDownloadResumeMaxAttempts
}

func genericDownloadResumePerRangeRetries() int {
	return currentTransportTuning().GenericDownloadResumePerRangeRetries
}

func genericDownloadPenaltyWait() time.Duration {
	return time.Duration(currentTransportTuning().GenericDownloadPenaltyWaitMS) * time.Millisecond
}

func genericDownloadTotalBitrate() int64 {
	return int64(currentTransportTuning().GenericDownloadTotalBitrateBps)
}

func genericDownloadMinPerTransferBitrate() int64 {
	return int64(currentTransportTuning().GenericDownloadMinPerTransferBitrateBps)
}

func genericDownloadCircuitFailureThreshold() int {
	return currentTransportTuning().GenericDownloadCircuitFailureThreshold
}

func genericDownloadCircuitOpen() time.Duration {
	return time.Duration(currentTransportTuning().GenericDownloadCircuitOpenMS) * time.Millisecond
}

func effectiveGenericDownloadConvergence() config.GenericDownloadConvergenceProfile {
	return config.ConvergedGenericDownloadProfile(currentTransportTuning())
}

func genericDownloadRTPPayloadBytes() int {
	return effectiveGenericDownloadConvergence().PayloadBytes
}

func genericDownloadRTPBitrate() int64 {
	return int64(effectiveGenericDownloadConvergence().BitrateBps)
}

func genericDownloadRTPMinSpacing() time.Duration {
	return time.Duration(currentTransportTuning().GenericDownloadRTPMinSpacingUS) * time.Microsecond
}

func genericDownloadRTPSocketBufferBytes() int {
	return effectiveGenericDownloadConvergence().SocketBufferBytes
}

func genericDownloadRTPReorderWindowPackets() int {
	return effectiveGenericDownloadConvergence().ReorderWindowPackets
}

func genericDownloadRTPLossTolerancePackets() int {
	return effectiveGenericDownloadConvergence().LossTolerancePackets
}

func genericDownloadRTPGapTimeout() time.Duration {
	return time.Duration(effectiveGenericDownloadConvergence().GapTimeoutMS) * time.Millisecond
}

func genericDownloadRTPFECEnabled() bool {
	return effectiveGenericDownloadConvergence().FECEnabled
}

func genericDownloadRTPFECGroupPackets() int {
	return effectiveGenericDownloadConvergence().FECGroupPackets
}

func boundaryRTPReorderWindowPackets() int {
	return currentTransportTuning().BoundaryRTPReorderWindowPackets
}

func boundaryRTPLossTolerancePackets() int {
	return currentTransportTuning().BoundaryRTPLossTolerancePackets
}

func boundaryRTPGapTimeout() time.Duration {
	return time.Duration(currentTransportTuning().BoundaryRTPGapTimeoutMS) * time.Millisecond
}

func boundaryRTPFECEnabled() bool {
	cfg := currentTransportTuning()
	return cfg.BoundaryRTPFECGroupPackets > 1 || cfg.BoundaryRTPFECEnabled
}

func boundaryRTPFECGroupPackets() int {
	return currentTransportTuning().BoundaryRTPFECGroupPackets
}

func genericRTPSegmentedDownloadProfile() segmentedDownloadProfile {
	cfg := currentTransportTuning()
	return segmentedDownloadProfile{
		name:           "generic-rtp",
		windowBytes:    cfg.GenericDownloadWindowBytes,
		threshold:      cfg.GenericSegmentedPrimaryThresholdBytes,
		concurrency:    cfg.GenericDownloadSegmentConcurrency,
		segmentRetries: cfg.GenericDownloadSegmentRetries,
	}
}

func boundarySegmentedDownloadProfile() segmentedDownloadProfile {
	cfg := currentTransportTuning()
	return segmentedDownloadProfile{
		name:           "boundary-rtp",
		windowBytes:    cfg.BoundaryFixedWindowBytes,
		threshold:      cfg.BoundaryFixedWindowThreshold,
		concurrency:    cfg.BoundarySegmentConcurrency,
		segmentRetries: cfg.BoundarySegmentRetries,
	}
}

func boundaryHTTPSegmentedDownloadProfile() segmentedDownloadProfile {
	cfg := currentTransportTuning()
	return segmentedDownloadProfile{
		name:           "boundary-http",
		windowBytes:    cfg.BoundaryHTTPWindowBytes,
		threshold:      cfg.BoundaryHTTPWindowThreshold,
		concurrency:    cfg.BoundaryHTTPSegmentConcurrency,
		segmentRetries: cfg.BoundaryHTTPSegmentRetries,
	}
}

func standardSegmentedDownloadProfile() segmentedDownloadProfile {
	cfg := currentTransportTuning()
	return segmentedDownloadProfile{
		name:           "standard-http",
		windowBytes:    cfg.StandardWindowBytes,
		threshold:      cfg.StandardWindowThreshold,
		concurrency:    cfg.StandardSegmentConcurrency,
		segmentRetries: cfg.StandardSegmentRetries,
	}
}

func boundaryResumeMaxAttempts() int {
	return currentTransportTuning().BoundaryResumeMaxAttempts
}

func boundaryResumePerRangeRetries() int {
	return currentTransportTuning().BoundaryResumePerRangeRetries
}

func boundaryResponseStartWait() time.Duration {
	return time.Duration(currentTransportTuning().BoundaryResponseStartWaitMS) * time.Millisecond
}

func boundaryRangeResponseStartWait() time.Duration {
	return time.Duration(currentTransportTuning().BoundaryRangeResponseWaitMS) * time.Millisecond
}

func boundaryFixedWindowThreshold() int64 {
	return currentTransportTuning().BoundaryFixedWindowThreshold
}

func boundaryPlaybackRTPReorderWindowPackets() int {
	cfg := currentTransportTuning()
	if cfg.BoundaryPlaybackRTPReorderWindowPackets > 0 {
		return cfg.BoundaryPlaybackRTPReorderWindowPackets
	}
	return cfg.BoundaryRTPReorderWindowPackets
}

func boundaryPlaybackRTPLossTolerancePackets() int {
	cfg := currentTransportTuning()
	if cfg.BoundaryPlaybackRTPLossTolerancePackets >= 0 {
		return cfg.BoundaryPlaybackRTPLossTolerancePackets
	}
	return cfg.BoundaryRTPLossTolerancePackets
}

func boundaryPlaybackRTPGapTimeout() time.Duration {
	cfg := currentTransportTuning()
	if cfg.BoundaryPlaybackRTPGapTimeoutMS > 0 {
		return time.Duration(cfg.BoundaryPlaybackRTPGapTimeoutMS) * time.Millisecond
	}
	return time.Duration(cfg.BoundaryRTPGapTimeoutMS) * time.Millisecond
}

func boundaryPlaybackRTPFECEnabled() bool {
	cfg := currentTransportTuning()
	return cfg.BoundaryPlaybackRTPFECGroupPackets > 1 || cfg.BoundaryPlaybackRTPFECEnabled
}

func boundaryPlaybackRTPFECGroupPackets() int {
	cfg := currentTransportTuning()
	if cfg.BoundaryPlaybackRTPFECGroupPackets > 0 {
		return cfg.BoundaryPlaybackRTPFECGroupPackets
	}
	return cfg.BoundaryRTPFECGroupPackets
}

func inlineResponseUDPBudgetBytes() int {
	cfg := currentTransportTuning()
	if cfg.InlineResponseUDPBudgetBytes > 0 {
		return cfg.InlineResponseUDPBudgetBytes
	}
	return cfg.UDPControlMaxBytes
}

func inlineResponseSafetyReserveBytes() int {
	return currentTransportTuning().InlineResponseSafetyReserveBytes
}

func inlineResponseEnvelopeOverheadBytes() int {
	return currentTransportTuning().InlineResponseEnvelopeOverheadBytes
}

func inlineResponseHeadroomRatio() float64 {
	cfg := currentTransportTuning()
	if cfg.InlineResponseHeadroomRatio > 0 {
		return cfg.InlineResponseHeadroomRatio
	}
	if cfg.InlineResponseHeadroomPercent > 0 {
		return float64(cfg.InlineResponseHeadroomPercent) / 100
	}
	return 0
}
func udpSmallRequestMaxWait() time.Duration {
	return time.Duration(currentTransportTuning().UDPSmallRequestMaxWaitMS) * time.Millisecond
}

func udpSegmentParallelismPerDevice() int {
	return currentTransportTuning().UDPSegmentParallelismPerDevice
}

func adaptivePlaybackHotWindowBytes() int64 {
	return currentTransportTuning().AdaptivePlaybackHotWindowBytes
}

func adaptivePlaybackSegmentCacheBytes() int64 {
	return currentTransportTuning().AdaptivePlaybackSegmentCacheBytes
}

func adaptivePlaybackSegmentCacheTTL() time.Duration {
	return time.Duration(currentTransportTuning().AdaptivePlaybackSegmentCacheTTLMS) * time.Millisecond
}

func adaptivePlaybackPrefetchSegments() int {
	return currentTransportTuning().AdaptivePlaybackPrefetchSegments
}

func adaptivePrimarySegmentAfterFailures() int {
	return currentTransportTuning().AdaptivePrimarySegmentAfterFailures
}

func adaptiveLoopbackPlaybackSegmentConcurrency() int {
	return currentTransportTuning().AdaptiveLoopbackPlaybackSegmentConcurrency
}

func adaptiveOpenEndedRangeInitialWindowBytes() int64 {
	return currentTransportTuning().AdaptiveOpenEndedRangeInitialWindowBytes
}

func udpRequestParallelismPerDevice() int {
	return currentTransportTuning().UDPRequestParallelismPerDevice
}

func udpCallbackParallelismPerPeer() int {
	return currentTransportTuning().UDPCallbackParallelismPerPeer
}

func udpBulkParallelismPerDevice() int {
	return currentTransportTuning().UDPBulkParallelismPerDevice
}
