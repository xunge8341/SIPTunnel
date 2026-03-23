package config

import "fmt"

const (
	genericDownloadStablePayloadCapBytes   = 1200
	genericDownloadStableBitrateCapBps     = 16 * 1024 * 1024
	genericDownloadStableMinBitrateCapBps  = 4 * 1024 * 1024
	genericDownloadStableSocketBufferBytes = 32 << 20
	genericDownloadStableReorderWindowMax  = 768
	genericDownloadStableLossToleranceMax  = 256
	genericDownloadStableGapTimeoutMSMax   = 1200
	genericDownloadStableFECGroupPackets   = 8
)

type GenericDownloadConvergenceProfile struct {
	PayloadBytes           int
	BitrateBps             int
	SocketBufferBytes      int
	ReorderWindowPackets   int
	LossTolerancePackets   int
	GapTimeoutMS           int
	FECEnabled             bool
	FECGroupPackets        int
	AggressiveConfig       bool
	PayloadCapped          bool
	BitrateCapped          bool
	SocketBufferCapped     bool
	ReorderWindowCapped    bool
	LossToleranceCapped    bool
	GapTimeoutCapped       bool
	FECForced              bool
	EffectiveBitrateCapBps int
}

func stableGenericDownloadBitrateCap(totalBitrateBps int) int {
	capBps := genericDownloadStableBitrateCapBps
	if totalBitrateBps > 0 {
		half := totalBitrateBps / 2
		if half > 0 && half < capBps {
			capBps = half
		}
	}
	if capBps < genericDownloadStableMinBitrateCapBps {
		capBps = genericDownloadStableMinBitrateCapBps
	}
	return capBps
}

func ConvergedGenericDownloadProfile(t TransportTuningConfig) GenericDownloadConvergenceProfile {
	out := GenericDownloadConvergenceProfile{
		PayloadBytes:           t.BoundaryRTPPayloadBytes,
		BitrateBps:             t.GenericDownloadRTPBitrateBps,
		SocketBufferBytes:      t.GenericDownloadRTPSocketBufferBytes,
		ReorderWindowPackets:   t.GenericDownloadRTPReorderWindowPackets,
		LossTolerancePackets:   t.GenericDownloadRTPLossTolerancePackets,
		GapTimeoutMS:           t.GenericDownloadRTPGapTimeoutMS,
		FECEnabled:             t.GenericDownloadRTPFECEnabled || t.GenericDownloadRTPFECGroupPackets > 1,
		FECGroupPackets:        t.GenericDownloadRTPFECGroupPackets,
		EffectiveBitrateCapBps: stableGenericDownloadBitrateCap(t.GenericDownloadTotalBitrateBps),
	}
	if out.PayloadBytes <= 0 {
		out.PayloadBytes = genericDownloadStablePayloadCapBytes
	}
	if out.BitrateBps <= 0 {
		out.BitrateBps = 8 * 1024 * 1024
	}
	if out.SocketBufferBytes <= 0 {
		out.SocketBufferBytes = genericDownloadStableSocketBufferBytes
	}
	if out.ReorderWindowPackets <= 0 {
		out.ReorderWindowPackets = 512
	}
	if out.LossTolerancePackets <= 0 {
		out.LossTolerancePackets = 192
	}
	if out.GapTimeoutMS <= 0 {
		out.GapTimeoutMS = 900
	}
	if out.PayloadBytes > genericDownloadStablePayloadCapBytes {
		out.PayloadBytes = genericDownloadStablePayloadCapBytes
		out.PayloadCapped = true
	}
	if out.BitrateBps > out.EffectiveBitrateCapBps {
		out.BitrateBps = out.EffectiveBitrateCapBps
		out.BitrateCapped = true
	}
	if out.SocketBufferBytes > genericDownloadStableSocketBufferBytes {
		out.SocketBufferBytes = genericDownloadStableSocketBufferBytes
		out.SocketBufferCapped = true
	}
	if out.ReorderWindowPackets > genericDownloadStableReorderWindowMax {
		out.ReorderWindowPackets = genericDownloadStableReorderWindowMax
		out.ReorderWindowCapped = true
	}
	if out.LossTolerancePackets > genericDownloadStableLossToleranceMax {
		out.LossTolerancePackets = genericDownloadStableLossToleranceMax
		out.LossToleranceCapped = true
	}
	if out.GapTimeoutMS > genericDownloadStableGapTimeoutMSMax {
		out.GapTimeoutMS = genericDownloadStableGapTimeoutMSMax
		out.GapTimeoutCapped = true
	}
	out.AggressiveConfig = out.PayloadCapped || out.BitrateCapped || out.SocketBufferCapped || out.ReorderWindowCapped || out.LossToleranceCapped || out.GapTimeoutCapped
	if out.FECEnabled {
		if out.FECGroupPackets <= 1 || out.FECGroupPackets > genericDownloadStableFECGroupPackets {
			out.FECGroupPackets = genericDownloadStableFECGroupPackets
		}
	} else if out.AggressiveConfig {
		out.FECEnabled = true
		out.FECForced = true
		out.FECGroupPackets = genericDownloadStableFECGroupPackets
	}
	return out
}

func GenericDownloadConvergenceSummary(t TransportTuningConfig) string {
	p := ConvergedGenericDownloadProfile(t)
	status := "aligned"
	if p.AggressiveConfig || p.FECForced {
		status = "clamped"
	}
	return fmt.Sprintf("generic_download_profile_guard=%s payload<=%d bitrate<=%d socket_buffer<=%d reorder<=%d loss<=%d gap_timeout_ms<=%d fec=%t/%d", status, p.PayloadBytes, p.BitrateBps, p.SocketBufferBytes, p.ReorderWindowPackets, p.LossTolerancePackets, p.GapTimeoutMS, p.FECEnabled, p.FECGroupPackets)
}
